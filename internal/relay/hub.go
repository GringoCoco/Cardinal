// Package relay — WebSocket-hub между Mac-daemon'ом и Telegram Mini App.
//
// По духу stateless: relay не хранит историю переписки, только
//   - реестр устройств (id → device token → активный WS),
//   - привязку chatId ↔ deviceId (через одноразовый код пары),
//   - маленький «снапшот» последних статусов, чтобы Mini App увидел
//     состояние сразу после переподключения.
//
// Транспорт: оба конца подключаются сами (daemon и телефон за любым NAT),
// relay ничего не открывает наружу.
package relay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Envelope — единица обмена. to выбирает адресата: "app" или "device".
type Envelope struct {
	V    int             `json:"v"`
	To   string          `json:"to"` // "app" | "device"
	Type string          `json:"type"`
	Body json.RawMessage `json:"body,omitempty"`
}

const snapshotLimit = 20 // сколько последних статусов видит Mini App на старте

type connWrap struct {
	c   *websocket.Conn
	ctx context.Context // cancel при закрытии
}

type Device struct {
	ID         string
	Name       string
	Token      string // секрет для подключения WS (Bearer)
	ChatID     int64  // владелец после привязки (0 = не привязано)
	deviceConn *connWrap
	appConn    *connWrap
	Snapshot   []Envelope // последние статусы для Mini App
}

type Hub struct {
	mu        sync.Mutex
	devices   map[string]*Device // по ID
	byToken   map[string]*Device // по device token
	chats     map[int64]string   // chatID → deviceID
	pairCodes map[string]pairCode
}

type pairCode struct {
	deviceID  string
	expiresAt time.Time
}

func NewHub() *Hub {
	return &Hub{
		devices:   map[string]*Device{},
		byToken:   map[string]*Device{},
		chats:     map[int64]string{},
		pairCodes: map[string]pairCode{},
	}
}

func NewToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Register — daemon сообщил о себе (Bearer = общий RELAY_TOKEN).
// Повторная регистрация сохраняет прежний token, чтобы daemon не терял привязку.
func (h *Hub) Register(deviceID, name string) (*Device, error) {
	if deviceID == "" || len(deviceID) > 64 {
		return nil, fmt.Errorf("некорректный deviceId")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if d, ok := h.devices[deviceID]; ok {
		if name != "" {
			d.Name = name
		}
		return d, nil
	}
	d := &Device{ID: deviceID, Name: name, Token: NewToken(24)}
	h.devices[deviceID] = d
	h.byToken[d.Token] = d
	return d, nil
}

// SetPairCode — daemon выпустил одноразовый код (показывает в терминале).
func (h *Hub) SetPairCode(deviceID string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.devices[deviceID]; !ok {
		return "", fmt.Errorf("устройство %s не зарегистрировано", deviceID)
	}
	code := fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	h.pairCodes[code] = pairCode{deviceID: deviceID, expiresAt: time.Now().Add(10 * time.Minute)}
	return code, nil
}

// PairByCode — Mini App привязывает свой chatId к устройству.
func (h *Hub) PairByCode(code string, chatID int64) (*Device, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	pc, ok := h.pairCodes[code]
	if !ok || time.Now().After(pc.expiresAt) {
		return nil, false
	}
	delete(h.pairCodes, code)
	d := h.devices[pc.deviceID]
	if d == nil {
		return nil, false
	}
	d.ChatID = chatID
	h.chats[chatID] = d.ID
	return d, true
}

// DeviceByChat — устройство, привязанное к чату Mini App.
func (h *Hub) DeviceByChat(chatID int64) *Device {
	h.mu.Lock()
	defer h.mu.Unlock()
	id, ok := h.chats[chatID]
	if !ok {
		return nil
	}
	return h.devices[id]
}

// DeviceByToken — устройство по device token (WS-аутентификация daemon'а).
func (h *Hub) DeviceByToken(token string) *Device {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.byToken[token]
}

func snapshotType(t string) bool {
	switch t {
	case "session.status", "session.finished", "permission.request":
		return true
	}
	return false
}

// AttachDeviceWS — daemon подключился по WS. Возвращает writer для исходящих.
func (h *Hub) AttachDeviceWS(d *Device, c *websocket.Conn, ctx context.Context) {
	h.mu.Lock()
	if old := d.deviceConn; old != nil {
		_ = old.c.Close(websocket.StatusNormalClosure, "replaced")
	}
	d.deviceConn = &connWrap{c: c, ctx: ctx}
	h.mu.Unlock()

	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			h.mu.Lock()
			if d.deviceConn != nil && d.deviceConn.c == c {
				d.deviceConn = nil
			}
			h.mu.Unlock()
			return
		}
		var env Envelope
		if err := json.Unmarshal(data, &env); err != nil || env.To != "app" {
			continue
		}
		h.mu.Lock()
		if snapshotType(env.Type) {
			d.Snapshot = append(d.Snapshot, env)
			if len(d.Snapshot) > snapshotLimit {
				d.Snapshot = d.Snapshot[len(d.Snapshot)-snapshotLimit:]
			}
		}
		app := d.appConn
		h.mu.Unlock()
		if app != nil {
			_ = app.c.Write(app.ctx, websocket.MessageText, data)
		}
	}
}

// AttachAppWS — Mini App подключился (чат уже привязан).
func (h *Hub) AttachAppWS(d *Device, c *websocket.Conn, ctx context.Context) {
	h.mu.Lock()
	if old := d.appConn; old != nil {
		_ = old.c.Close(websocket.StatusNormalClosure, "replaced")
	}
	d.appConn = &connWrap{c: c, ctx: ctx}
	snap := append([]Envelope(nil), d.Snapshot...)
	dev := d.deviceConn
	h.mu.Unlock()

	for _, env := range snap { // снапшот сразу после подключения
		data, _ := json.Marshal(env)
		if err := c.Write(ctx, websocket.MessageText, data); err != nil {
			return
		}
	}

	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			h.mu.Lock()
			if d.appConn != nil && d.appConn.c == c {
				d.appConn = nil
			}
			h.mu.Unlock()
			return
		}
		var env Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			continue
		}
		env.To = "device"
		forward, _ := json.Marshal(env)
		if dev != nil {
			_ = dev.c.Write(dev.ctx, websocket.MessageText, forward)
		}
	}
}

// DeviceOnline — есть ли активный WS у устройства.
func (h *Hub) DeviceOnline(d *Device) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return d.deviceConn != nil
}

// WriteToDevice — доставить конверт на подключённый daemon (например,
// промпт из Mini App через POST /api/send). false = daemon не на связи.
func (h *Hub) WriteToDevice(d *Device, env Envelope) bool {
	data, err := json.Marshal(env)
	if err != nil {
		return false
	}
	h.mu.Lock()
	dev := d.deviceConn
	h.mu.Unlock()
	if dev == nil {
		return false
	}
	return dev.c.Write(dev.ctx, websocket.MessageText, data) == nil
}
