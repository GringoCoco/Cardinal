// RelayClient — WS-подключение daemon'а к relay (фаза 1).
//
// Daemon сам подключается к relay (оба конца за любым NAT):
//   - шлёт статусы сессий и карточки разрешений → Mini App (+ снапшот);
//   - принимает конверты Mini App: permission.resolve (решение карточки
//     из Mini App, как из кнопок Telegram) и notify (текст в Telegram).
//
// Переподключение — с экспоненциальной задержкой; при 401
// перерегистрируется (relay хранит реестр в памяти и сбрасывается).
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/GringoCoco/cardinal/internal/events"
	"github.com/GringoCoco/cardinal/internal/relay"
)

// RelayClient — живое WS-соединение daemon → relay.
type RelayClient struct {
	d         *Daemon
	base      *url.URL
	relayTok  string
	deviceTok string
	mu        sync.Mutex
	ch        chan relay.Envelope // очередь исходящих (одна горутина-писатель)
}

// newRelayClient — nil, если relay не настроен (фаза 0 живёт без него).
func newRelayClient(d *Daemon) *RelayClient {
	if d.cfg.RelayURL == "" || d.cfg.DeviceToken == "" {
		return nil
	}
	base, err := url.Parse(strings.TrimRight(d.cfg.RelayURL, "/"))
	if err != nil || base.Host == "" {
		log.Printf("relay: кривой relay_url %q — релей выключен", d.cfg.RelayURL)
		return nil
	}
	return &RelayClient{
		d:         d,
		base:      base,
		relayTok:  d.cfg.RelayToken,
		deviceTok: d.cfg.DeviceToken,
		ch:        make(chan relay.Envelope, 64),
	}
}

// Run — цикл подключений до завершения контекста.
func (c *RelayClient) Run(ctx context.Context) {
	backoff := time.Second
	for {
		if err := c.once(ctx); err != nil && ctx.Err() == nil {
			log.Printf("relay: %v — переподключение через %v", err, backoff)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff += backoff / 2 // 1с → 1.5с → 2.25с … потолок 30с
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
}

// once — одно подключение: dial, писатель, пинг, чтение.
func (c *RelayClient) once(ctx context.Context) error {
	wsURL := websocketURL(c.base)
	conn, resp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + c.token()}},
	})
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			// relay перезапустился и забыл нас — регистрируемся заново
			if tok, rerr := c.reRegister(ctx); rerr != nil {
				return fmt.Errorf("перерегистрация: %w", rerr)
			} else if conn, _, err = websocket.Dial(ctx, wsURL, &websocket.DialOptions{
				HTTPHeader: http.Header{"Authorization": {"Bearer " + tok}},
			}); err != nil {
				return fmt.Errorf("dial после перерегистрации: %w", err)
			}
		} else {
			return fmt.Errorf("dial %s: %w", wsURL, err)
		}
	}
	defer conn.Close(websocket.StatusNormalClosure, "bye")

	log.Printf("relay: подключён к %s", c.base.Host)
	defer log.Printf("relay: соединение закрыто")

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go c.writer(runCtx, conn) // единственный писатель
	go c.pinger(runCtx, conn)

	for {
		_, data, err := conn.Read(runCtx)
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
		var env relay.Envelope
		if json.Unmarshal(data, &env) == nil {
			c.handleAppEnvelope(env)
		}
	}
}

// writer — единственный писатель WS: тянет конверты из очереди.
func (c *RelayClient) writer(ctx context.Context, conn *websocket.Conn) {
	for {
		select {
		case <-ctx.Done():
			return
		case env := <-c.ch:
			env.V = 1
			env.To = "app"
			data, err := json.Marshal(env)
			if err != nil {
				continue
			}
			c.mu.Lock()
			err = conn.Write(ctx, websocket.MessageText, data)
			c.mu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

// pinger — держит соединение живым сквозь NAT/Fly (pong = признак жизни).
func (c *RelayClient) pinger(ctx context.Context, conn *websocket.Conn) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.mu.Lock()
			err := conn.Ping(ctx)
			c.mu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

// SendStatus — поставить конверт статуса в очередь (не блокирует).
func (c *RelayClient) SendStatus(typ string, body any) {
	if c == nil {
		return
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return
	}
	select {
	case c.ch <- relay.Envelope{Type: typ, Body: raw}:
	default:
		log.Printf("relay: очередь переполнена — конверт %q отброшен", typ)
	}
}

// handleAppEnvelope — конверт от Mini App.
func (c *RelayClient) handleAppEnvelope(env relay.Envelope) {
	switch env.Type {
	case relay.TypePermResolve:
		var body struct {
			ID       string `json:"id"`
			Decision string `json:"decision"`
		}
		if json.Unmarshal(env.Body, &body) != nil || body.ID == "" {
			return
		}
		c.d.resolvePermByID(body.ID, body.Decision == "allow")

	case relay.TypeNotify:
		var body struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(env.Body, &body) != nil || body.Text == "" {
			return
		}
		if c.d.cfg.ChatID != 0 {
			c.d.tg.Send(c.d.cfg.ChatID, "📱 "+body.Text, false, nil)
		}
	}
}

// reRegister — relay потерял нас (перезапуск, всё в памяти): новый device_token.
func (c *RelayClient) reRegister(ctx context.Context) (string, error) {
	if c.relayTok == "" {
		return "", fmt.Errorf("нет relay_token в конфиге — запусти «cardinald connect %s» заново", c.base)
	}
	tok, _, err := registerDevice(ctx, c.base, c.relayTok, DeviceID())
	if err != nil {
		return "", err
	}
	c.d.cfg.DeviceToken = tok
	if err := c.d.cfg.Save(); err != nil {
		return "", fmt.Errorf("сохранил не смог: %w", err)
	}
	c.mu.Lock()
	c.deviceTok = tok
	c.mu.Unlock()
	log.Printf("relay: перерегистрирован (новый device_token сохранён)")
	return tok, nil
}

func (c *RelayClient) token() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.deviceTok
}

// registerDevice — POST /api/register → device_token (+ device_id, каким звались).
func registerDevice(ctx context.Context, base *url.URL, relayToken, deviceID string) (string, string, error) {
	body, _ := json.Marshal(map[string]string{"device_id": deviceID, "name": deviceID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base.Scheme+"://"+base.Host+"/api/register", strings.NewReader(string(body)))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+relayToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("register: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("register: HTTP %d", resp.StatusCode)
	}
	var out struct {
		DeviceToken string `json:"device_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.DeviceToken == "" {
		return "", "", fmt.Errorf("register: кривой ответ")
	}
	return out.DeviceToken, deviceID, nil
}

// DeviceID — стабильное имя устройства из hostname (для реестра relay).
func DeviceID() string {
	hn, _ := os.Hostname()
	if hn == "" {
		return "mac-unknown"
	}
	hn = strings.ToLower(strings.SplitN(hn, ".", 2)[0])
	hn = strings.Map(func(r rune) rune {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-'
		if ok {
			return r
		}
		return -1
	}, hn)
	if hn == "" {
		return "mac-unknown"
	}
	return hn
}

// websocketURL — https:// → wss://, http:// → ws://.
func websocketURL(base *url.URL) string {
	scheme := "wss"
	if base.Scheme == "http" {
		scheme = "ws"
	}
	return scheme + "://" + base.Host + "/ws/device"
}

// resolvePermByID — решение, пришедшее из Mini App: то же, что кнопка Telegram.
func (d *Daemon) resolvePermByID(id string, allow bool) {
	d.mu.Lock()
	p, ok := d.pending[id]
	if ok {
		delete(d.pending, id)
	}
	d.mu.Unlock()
	if !ok {
		return
	}
	dec := events.DecisionDeny
	if allow {
		dec = events.DecisionAllow
	}
	select {
	case p.Decision <- dec:
	default:
	}
	if p.ChatID != 0 && p.MsgID != 0 {
		_ = d.tg.EditMarkup(p.ChatID, p.MsgID, nil)
	}
	state := "deny"
	if allow {
		state = "allow"
	}
	d.relay.SendStatus(relay.TypePermResolved, map[string]string{
		"id": id, "decision": state,
	})
}
