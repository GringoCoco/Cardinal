// Package events — канонические события агентов и их нормализация.
//
// Контракт скопирован с MauMau (coucou): любой агент присылает hook-JSON,
// имена событий которого нормализуются к каноническим. Сам Claude Code —
// эталон, остальные маппятся по таблице из windows/hook/src/normalize.rs.
package events

// Канонические события (Claude Code naming).
const (
	SessionStart     = "SessionStart"
	UserPromptSubmit = "UserPromptSubmit"
	PreToolUse       = "PreToolUse"
	PostToolUse      = "PostToolUse"
	Notification     = "Notification"
	PermissionRequ   = "PermissionRequest"
	Stop             = "Stop"
	StopFailure      = "StopFailure"
	SessionEnd       = "SessionEnd"
	SubagentStart    = "SubagentStart"
	SubagentStop     = "SubagentStop"
	StopInterrupted  = "StopInterrupted"
)

// normalizeEvents — чужие имена событий → канонические. Неизвестные проходят.
var normalizeEvents = map[string]string{
	// Gemini CLI / Antigravity
	"BeforeTool":            PreToolUse,
	"BeforeToolSelection":   PreToolUse,
	"AfterTool":             PostToolUse,
	"AfterModel":            PostToolUse,
	"BeforeAgent":           UserPromptSubmit,
	"AfterAgent":            Stop,
	"startup":               SessionStart,
	"exit":                  SessionEnd,
	"agent_start":           SubagentStart,
	"agent_end":             SubagentStop,
	"agent_end_ack_success": SubagentStop,
	// OpenCode (если шлёт свои имена)
	"permission.updated": PermissionRequ,
	"session.idle":       Stop,
	"session.error":      StopFailure,
}

// Normalize возвращает каноническое имя события.
func Normalize(name string) string {
	if canonical, ok := normalizeEvents[name]; ok {
		return canonical
	}
	return name
}

// Hook — полезная нагрузка от хука агента (POST /hook).
// Поля сверх канона Claude Code добавляются обёрткой хука (coucou_agent).
type Hook struct {
	Event        string         `json:"hook_event_name"`
	SessionID    string         `json:"session_id"`
	Agent        string         `json:"coucou_agent,omitempty"` // пусто = Claude Code
	Tool         string         `json:"tool_name,omitempty"`
	ToolInput    map[string]any `json:"tool_input,omitempty"`
	Prompt       string         `json:"prompt,omitempty"`
	Message      string         `json:"message,omitempty"`
	CWD          string         `json:"cwd,omitempty"`
	ToolResponse any            `json:"tool_response,omitempty"` // принимаем, не шлём
}

// Canonical возвращает нормализованное имя события.
func (h *Hook) Canonical() string { return Normalize(h.Event) }

// NamedAgent — имя агента для показа. Пустое = Claude Code.
func (h *Hook) NamedAgent() string {
	if h.Agent == "" {
		return "Claude Code"
	}
	return h.Agent
}

// Project — короткое имя проекта из cwd.
func (h *Hook) Project() string {
	if h.CWD == "" {
		return ""
	}
	for i := len(h.CWD) - 1; i >= 0; i-- {
		if h.CWD[i] == '/' {
			return h.CWD[i+1:]
		}
	}
	return h.CWD
}

// Summary — человекочитаемая суть действия для карточки разрешения.
func (h *Hook) Summary() string {
	if len(h.ToolInput) == 0 {
		return h.Tool
	}
	// Команда — исключение из privacy-default: без неё решение Allow/Deny
	// небезопасно. Не обрезаем её до размеров push-превью.
	if command, ok := h.ToolInput["command"].(string); ok && command != "" {
		return h.Tool + ": " + command
	}
	for _, key := range []string{"file_path", "pattern", "url", "path"} {
		if v, ok := h.ToolInput[key].(string); ok && v != "" {
			if len(v) > 80 {
				v = v[:80] + "…"
			}
			return h.Tool + ": " + v
		}
	}
	return h.Tool
}

// Decision — решение пользователя по PermissionRequest.
type Decision int

const (
	DecisionNone Decision = iota
	DecisionAllow
	DecisionDeny
)
