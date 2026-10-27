package daemon

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/GringoCoco/cardinal/internal/events"
	"github.com/GringoCoco/cardinal/internal/relay"
)

// sessionState — компактная модель экрана сессии. Она намеренно не хранит
// prompt/tool_response: Cardinal не превращает relay в историю переписки.
type sessionState struct {
	ID        string
	Agent     string
	Provider  string
	Project   string
	CWD       string
	Title     string
	Activity  string
	State     string
	Tool      string
	StartedAt time.Time
	UpdatedAt time.Time
	LogPath   string
}

type managedProcess struct {
	Cmd     *exec.Cmd
	LogPath string
}

func shortText(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func stateAndActivity(h *events.Hook) (string, string) {
	switch h.Canonical() {
	case events.SessionStart:
		return "running", "Сессия запущена"
	case events.UserPromptSubmit:
		return "running", "Получена новая задача"
	case events.PreToolUse:
		if h.Tool != "" {
			return "running", h.Tool + " — выполняется"
		}
		return "running", "Выполняется действие"
	case events.PostToolUse:
		if h.Tool != "" {
			return "running", h.Tool + " — выполнено"
		}
		return "running", "Действие выполнено"
	case events.Notification:
		return "waiting", shortText(h.Message, 120)
	case events.Stop:
		return "completed", "Задача завершена"
	case events.StopFailure:
		return "error", "Сессия завершилась с ошибкой"
	case events.StopInterrupted, events.SessionEnd:
		return "stopped", "Сессия остановлена"
	default:
		return "running", "Агент работает"
	}
}

func (s *sessionState) body() map[string]any {
	return map[string]any{
		"session_id": s.ID,
		"agent":      s.Agent,
		"provider":   s.Provider,
		"project":    s.Project,
		"cwd":        s.CWD,
		"title":      s.Title,
		"activity":   s.Activity,
		"state":      s.State,
		"tool":       s.Tool,
		"started_at": s.StartedAt.UnixMilli(),
		"updated_at": s.UpdatedAt.UnixMilli(),
		"has_output": s.LogPath != "",
	}
}

// trackHook обновляет одну карточку сессии. Сколько бы событий ни прислал
// агент, relay upsert'ит их по session_id — лента не спамится.
func (d *Daemon) trackHook(h *events.Hook) {
	if h.SessionID == "" {
		return
	}
	now := time.Now()
	state, activity := stateAndActivity(h)
	d.mu.Lock()
	s := d.sessions[h.SessionID]
	if s == nil {
		s = &sessionState{ID: h.SessionID, StartedAt: now}
		d.sessions[h.SessionID] = s
	}
	s.Agent = h.NamedAgent()
	s.Provider = agentKeyOf(h)
	if h.Project() != "" {
		s.Project = h.Project()
	}
	if h.CWD != "" {
		s.CWD = h.CWD
	}
	if s.Title == "" {
		s.Title = s.Project
		if s.Title == "" {
			s.Title = h.NamedAgent()
		}
	}
	if d.cfg.ShowText && h.Prompt != "" {
		s.Title = shortText(h.Prompt, 90)
	}
	s.State, s.Activity, s.Tool, s.UpdatedAt = state, activity, h.Tool, now
	if p := d.processes[h.SessionID]; p != nil {
		s.LogPath = p.LogPath
	}
	body := s.body()
	d.mu.Unlock()

	typ := relay.TypeSessionStatus
	if state == "completed" || state == "error" || state == "stopped" {
		typ = relay.TypeSessionFinished
	}
	d.relay.SendStatus(typ, body)
}

func (d *Daemon) markSessionWaiting(sessionID string) {
	if sessionID == "" {
		return
	}
	d.mu.Lock()
	s := d.sessions[sessionID]
	if s != nil {
		s.State = "waiting"
		s.Activity = "Ждёт вашего решения"
		s.UpdatedAt = time.Now()
	}
	var body map[string]any
	if s != nil {
		body = s.body()
	}
	d.mu.Unlock()
	if body != nil {
		d.relay.SendStatus(relay.TypeSessionStatus, body)
	}
}

func (d *Daemon) markSessionRunning(sessionID, activity string) {
	if sessionID == "" {
		return
	}
	d.mu.Lock()
	s := d.sessions[sessionID]
	if s != nil {
		s.State = "running"
		s.Activity = activity
		s.UpdatedAt = time.Now()
	}
	var body map[string]any
	if s != nil {
		body = s.body()
	}
	d.mu.Unlock()
	if body != nil {
		d.relay.SendStatus(relay.TypeSessionStatus, body)
	}
}

func (d *Daemon) syncSessions() {
	d.mu.Lock()
	bodies := make([]map[string]any, 0, len(d.sessions))
	for _, s := range d.sessions {
		bodies = append(bodies, s.body())
	}
	d.mu.Unlock()
	for _, body := range bodies {
		d.relay.SendStatus(relay.TypeSessionStatus, body)
	}
}

func (d *Daemon) handleSessionCommand(id, action string) {
	d.mu.Lock()
	p := d.processes[id]
	s := d.sessions[id]
	d.mu.Unlock()
	if action == "output" {
		if p == nil || p.LogPath == "" {
			d.relay.SendStatus(relay.TypeSessionOutput, map[string]any{
				"session_id": id, "error": "Вывод недоступен: сессия запущена вне Cardinal",
			})
			return
		}
		data, err := os.ReadFile(p.LogPath)
		if err != nil {
			d.relay.SendStatus(relay.TypeSessionOutput, map[string]any{"session_id": id, "error": err.Error()})
			return
		}
		if len(data) > 24<<10 {
			data = data[len(data)-(24<<10):]
		}
		d.relay.SendStatus(relay.TypeSessionOutput, map[string]any{"session_id": id, "output": string(data)})
		return
	}

	if !canControlSession(p) {
		d.relay.SendStatus(relay.TypeSessionAction, map[string]any{
			"session_id": id, "action": action, "ok": false,
			"error": "Этой сессией управляет внешний терминал",
		})
		return
	}
	var sig syscall.Signal
	state, activity := "running", "Агент работает"
	switch action {
	case "pause":
		sig, state, activity = syscall.SIGSTOP, "paused", "Сессия приостановлена"
	case "resume":
		sig, state, activity = syscall.SIGCONT, "running", "Работа возобновлена"
	case "stop":
		sig, state, activity = syscall.SIGTERM, "stopped", "Сессия остановлена"
	default:
		d.relay.SendStatus(relay.TypeSessionAction, map[string]any{"session_id": id, "action": action, "ok": false, "error": "Неизвестная команда"})
		return
	}
	err := p.Cmd.Process.Signal(sig)
	if err == nil && s != nil {
		d.mu.Lock()
		s.State, s.Activity, s.UpdatedAt = state, activity, time.Now()
		body := s.body()
		d.mu.Unlock()
		typ := relay.TypeSessionStatus
		if state == "stopped" {
			typ = relay.TypeSessionFinished
		}
		d.relay.SendStatus(typ, body)
	}
	d.relay.SendStatus(relay.TypeSessionAction, map[string]any{
		"session_id": id, "action": action, "ok": err == nil, "error": errorText(err),
	})
}

func canControlSession(p *managedProcess) bool {
	return p != nil && p.Cmd != nil && p.Cmd.Process != nil
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
