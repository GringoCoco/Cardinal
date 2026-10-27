// Package daemon — HTTP-сервер хуков на localhost и доставка событий в Telegram.
package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/GringoCoco/cardinal/internal/config"
	"github.com/GringoCoco/cardinal/internal/events"
	"github.com/GringoCoco/cardinal/internal/relay"
	"github.com/GringoCoco/cardinal/internal/tg"
)

// DecisionWait — сколько хук PermissionRequest ждёт решения
// (как в MauMau: 110 с). Дальше пустой ответ — агент спросит в терминале.
const DecisionWait = 110 * time.Second

// DecisionWaitOf — бюджет ожидания из конфига.
func DecisionWaitOf(cfg *config.Config) time.Duration {
	if cfg != nil && cfg.DecisionWaitSec > 0 {
		return time.Duration(cfg.DecisionWaitSec) * time.Second
	}
	return DecisionWait
}

// PendingPerm — запрос разрешения, ждущий решения из Telegram.
type PendingPerm struct {
	ID        string
	SessionID string
	Agent     string
	Project   string
	Summary   string
	ChatID    int64
	MsgID     int64
	Decision  chan events.Decision
}

type Daemon struct {
	cfg         *config.Config
	tg          *tg.Client
	relay       *RelayClient
	mu          sync.Mutex
	pending     map[string]*PendingPerm
	lastByAgent map[string]lastInfo
	sessions    map[string]*sessionState
	processes   map[string]*managedProcess
	httpSrv     *http.Server
}

func New(cfg *config.Config) *Daemon {
	return &Daemon{
		cfg: cfg, tg: tg.New(cfg.BotToken),
		pending: map[string]*PendingPerm{}, sessions: map[string]*sessionState{},
		processes: map[string]*managedProcess{},
	}
}

func (d *Daemon) notifyPermissions() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.cfg.NotifyPermissions()
}

func (d *Daemon) notifyCompleted() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.cfg.NotifyCompleted()
}

func (d *Daemon) notifyErrors() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.cfg.NotifyErrors()
}

// Run — поднять HTTP-сервер хуков и цикл опроса Telegram (если chat_id задан).
func (d *Daemon) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/hook", d.handleHook)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "cardinal")
	})

	addr := fmt.Sprintf("127.0.0.1:%d", d.cfg.Port)
	d.httpSrv = &http.Server{Addr: addr, Handler: mux}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("порт %d занят: %w", d.cfg.Port, err)
	}

	errCh := make(chan error, 1)
	go func() {
		if err := d.httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	if d.cfg.ChatID != 0 {
		go d.pollLoop(ctx)
	}
	if rc := newRelayClient(d); rc != nil {
		d.relay = rc
		go rc.Run(ctx)
		log.Printf("relay: запускаю WS-клиент → %s", d.cfg.RelayURL)
	}

	log.Printf("cardinald: хуки по http://%s/hook", addr)
	if d.cfg.ChatID == 0 {
		log.Printf("cardinald: chat_id не задан — пуши выключены. Сначала «cardinald pair».")
	}

	select {
	case <-ctx.Done():
		// всем ожидающим — пустой ответ (агент переспросит в терминале)
		d.mu.Lock()
		for _, p := range d.pending {
			select {
			case p.Decision <- events.DecisionNone:
			default:
			}
		}
		d.mu.Unlock()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return d.httpSrv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// handleHook — единая точка входа от агентов.
func (d *Daemon) handleHook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var hook events.Hook
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&hook); err != nil {
		log.Printf("hook: кривой JSON: %v", err)
		writeJSON(w, []byte("{}"))
		return
	}
	log.Printf("hook: %s %s · %s", hook.NamedAgent(), hook.Canonical(), hook.Project())

	d.remember(agentKeyOf(&hook), hook.CWD, hook.SessionID)

	// Правило MauMau. Ожидание — только для PermissionRequest,
	// остальное отвечаем пустым JSON немедленно.
	if hook.Canonical() == events.PermissionRequ {
		d.handlePermission(w, &hook)
		return
	}

	writeJSON(w, []byte("{}"))
	d.dispatch(&hook)
}

// dispatch — какой пуш слать для события. По умолчанию в пуше нет
// ни текста промпта, ни команды — только агент и состояние
// (приватность как у MauMau: push сообщает, детали смотришь на опции).
func (d *Daemon) dispatch(h *events.Hook) {
	d.trackHook(h)
	state := ""
	switch h.Canonical() {
	case events.Stop:
		state = "ok"
	case events.StopFailure:
		state = "error"
	case events.StopInterrupted:
		state = "interrupted"
	}
	// Завершение сессии должно закрыть ожидающие карточки даже в relay-only
	// режиме. Telegram ниже остаётся необязательным каналом уведомлений.
	if state != "" {
		reason := "сессия завершилась"
		if state == "error" {
			reason = "сессия завершилась с ошибкой"
		} else if state == "interrupted" {
			reason = "сессия остановлена"
		}
		d.cancelPending(h.SessionID, reason)
	}

	if d.cfg.ChatID == 0 {
		return
	}
	switch h.Canonical() {
	case events.Stop:
		if d.notifyCompleted() {
			d.send(h.NamedAgent()+" 🟢 готово", h, false, nil, nil)
		}
	case events.StopFailure:
		if d.notifyErrors() {
			d.send(h.NamedAgent()+" 🔴 ошибка", h, false, nil, nil)
		}
	case events.StopInterrupted:
		if d.notifyErrors() {
			d.send(h.NamedAgent()+" ⏹ остановлено", h, false, nil, nil)
		}
	case events.Notification:
		msg := h.Message
		if msg == "" {
			msg = "требует внимания"
		}
		if len(msg) > 200 {
			msg = msg[:200] + "…"
		}
		d.send("💬 "+h.NamedAgent()+": "+msg, h, false, nil, nil)
	}
}

// cancelPending — сессия завершилась: гасим её живые карточки разрешений,
// снимаем кнопки и объясняем, почему решение больше не нужно.
func (d *Daemon) cancelPending(sessionID, reason string) {
	if sessionID == "" {
		return
	}
	d.mu.Lock()
	var cancelled []*PendingPerm
	for _, p := range d.pending {
		if p.SessionID == sessionID {
			cancelled = append(cancelled, p)
			delete(d.pending, p.ID)
		}
	}
	d.mu.Unlock()
	for _, p := range cancelled {
		select {
		case p.Decision <- events.DecisionNone:
		default:
		}
		if p.ChatID != 0 && p.MsgID != 0 {
			_ = d.tg.EditText(p.ChatID, p.MsgID,
				"⌛ "+p.Agent+" — "+reason+"\n"+p.Summary)
		}
		d.relay.SendStatus(relay.TypePermResolved, map[string]string{
			"id": p.ID, "decision": "expired", "agent": p.Agent,
			"summary": p.Summary, "session_id": p.SessionID,
		})
	}
}

// handlePermission — речь о безопасном решении: ждём ответ ≤ DecisionWait.
func (d *Daemon) handlePermission(w http.ResponseWriter, h *events.Hook) {
	p := &PendingPerm{
		ID:        newID(),
		SessionID: h.SessionID,
		Agent:     h.NamedAgent(),
		Project:   h.Project(),
		Summary:   h.Summary(),
		Decision:  make(chan events.Decision, 1),
	}
	d.mu.Lock()
	d.pending[p.ID] = p
	d.mu.Unlock()

	title := "⚠️ " + p.Agent + " ждёт разрешения"
	if p.Project != "" {
		title += " · " + p.Project
	}
	text := title + "\n" + p.Summary
	markup := &tg.ReplyMarkup{InlineKeyboard: [][]tg.InlineButton{{
		{Text: "✅ Разрешить", Data: "perm:" + p.ID + ":1"},
		{Text: "⛔ Запретить", Data: "perm:" + p.ID + ":0"},
	}}}

	// Сначала relay: Mini App может принять решение даже если Telegram-пуш
	// временно недоступен. Карточка всё равно содержит команду полностью.
	d.relay.SendStatus(relay.TypePermRequest, map[string]string{
		"id": p.ID, "agent": p.Agent, "summary": p.Summary,
		"project": p.Project, "session_id": p.SessionID, "cwd": h.CWD,
	})
	d.markSessionWaiting(p.SessionID)

	msgID := int64(0)
	var err error
	if d.cfg.ChatID != 0 && d.notifyPermissions() {
		// Синхронно (кнопки сгенерированы до ожидания), но занимает ≤1.5 с.
		msgID, err = d.send(text, h, false, markup, p)
	}
	if !d.relay.IsConnected() && msgID == 0 {
		log.Printf("permission: ни relay, ни Telegram недоступны (%v) — отвечаю пустым", err)
		d.mu.Lock()
		delete(d.pending, p.ID)
		d.mu.Unlock()
		// Если request уже попал в очередь relay, следом обязательно отправляем
		// tombstone: после reconnect не должна всплыть неактуальная карточка.
		d.relay.SendStatus(relay.TypePermResolved, map[string]string{
			"id": p.ID, "decision": "expired", "agent": p.Agent,
			"summary": p.Summary, "session_id": p.SessionID,
		})
		writeJSON(w, []byte("{}"))
		return
	}

	select {
	case dec := <-p.Decision:
		writeJSON(w, decisionJSON(dec))
	case <-time.After(DecisionWaitOf(d.cfg)):
		if msgID > 0 {
			_ = d.tg.EditMarkup(d.cfg.ChatID, msgID, nil)
		}
		d.relay.SendStatus(relay.TypePermResolved, map[string]string{
			"id": p.ID, "decision": "expired", "agent": p.Agent,
			"summary": p.Summary, "session_id": p.SessionID,
		})
		writeJSON(w, []byte("{}"))
	}

	d.mu.Lock()
	delete(d.pending, p.ID)
	d.mu.Unlock()
}

// decisionJSON — контракт ответа хуку (как в MauMau reply.rs).
func decisionJSON(dec events.Decision) []byte {
	if dec == events.DecisionNone {
		return []byte("{}")
	}
	behavior := "deny"
	if dec == events.DecisionAllow {
		behavior = "allow"
	}
	return []byte(fmt.Sprintf(
		`{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":%q}}}`,
		behavior))
}

// send — пуш в чат владельца. owner-perm: заполняет ChatID/MsgID пермийца.
func (d *Daemon) send(text string, h *events.Hook, silent bool, markup *tg.ReplyMarkup, p *PendingPerm) (int64, error) {
	if d.cfg.ShowText && h.Prompt != "" {
		prompt := h.Prompt
		if len(prompt) > 120 {
			prompt = prompt[:120] + "…"
		}
		text += "\n» " + prompt
	}
	id, err := d.tg.Send(d.cfg.ChatID, text, silent, markup)
	if err == nil && p != nil {
		p.ChatID, p.MsgID = d.cfg.ChatID, id
	}
	return id, err
}

// pollLoop — единственный потребитель getUpdates: кнопки Allow/Deny.
func (d *Daemon) pollLoop(ctx context.Context) {
	offset := 0
	for {
		updates, err := d.tg.GetUpdates(offset, 25)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			time.Sleep(3 * time.Second)
			continue
		}
		for _, u := range updates {
			offset = u.UpdateID + 1
			d.handleUpdate(u)
		}
		if ctx.Err() != nil {
			return
		}
	}
}

func (d *Daemon) allowedChat(chatID int64) bool {
	if chatID == d.cfg.ChatID {
		return true
	}
	for _, id := range d.cfg.ExtraChats {
		if id == chatID {
			return true
		}
	}
	return false
}

func (d *Daemon) handleUpdate(u tg.Update) {
	cb := u.CallbackQuery
	if cb == nil || !strings.HasPrefix(cb.Data, "perm:") {
		return
	}
	parts := strings.SplitN(cb.Data, ":", 3)
	if len(parts) != 3 || !d.allowedChat(cb.From.ID) {
		_ = d.tg.AnswerCallback(cb.ID, "Недоступно в этом чате")
		return
	}
	dec := events.DecisionDeny
	if parts[2] == "1" {
		dec = events.DecisionAllow
	}
	word := "Запрещено"
	if dec == events.DecisionAllow {
		word = "Разрешено"
	}

	d.mu.Lock()
	p, ok := d.pending[parts[1]]
	if ok {
		delete(d.pending, parts[1])
	}
	d.mu.Unlock()

	if !ok {
		// карточка уже обработана/истекла — снимаем кнопки, чтобы не висела
		if cb.Message != nil && cb.Message.MessageID != 0 && d.allowedChat(cb.From.ID) {
			_ = d.tg.EditText(cb.From.ID, cb.Message.MessageID, "⌛ Карточка уже обработана или истекла")
		}
		_ = d.tg.AnswerCallback(cb.ID, "Уже решено или истекло")
		return
	}
	if p.ChatID != 0 && p.MsgID != 0 {
		_ = d.tg.EditMarkup(p.ChatID, p.MsgID, nil)
	}
	select {
	case p.Decision <- dec:
	default:
	}
	_ = d.tg.AnswerCallback(cb.ID, word+" · "+p.Agent)
	state := "deny"
	if dec == events.DecisionAllow {
		state = "allow"
	}
	// Mini App тоже должен увидеть, что решение принято из Telegram
	d.relay.SendStatus(relay.TypePermResolved, map[string]string{
		"id": p.ID, "decision": state, "agent": p.Agent, "summary": p.Summary,
		"session_id": p.SessionID,
	})
	d.markSessionRunning(p.SessionID, "Решение принято — агент продолжает работу")
}

func writeJSON(w http.ResponseWriter, b []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// NewCode — 6-значный одноразовый код для привязки Telegram-чата.
func NewCode() (string, error) {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	n := int(b[0])<<16 | int(b[1])<<8 | int(b[2])
	return fmt.Sprintf("%06d", n%1000000), nil
}
