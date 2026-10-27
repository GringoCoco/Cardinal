// relayd — сервер-ретранслятор Cardinal: daemon (Mac) ⇄ Mini App (телефон).
//
// Endpoints:
//
//	POST /api/register      — daemon: Bearer $RELAY_TOKEN → {device_token}
//	POST /api/pair-code     — daemon: Bearer <device_token> → {code}
//	POST /api/pair          — Mini App: {initData, code} → привязка chatId
//	GET  /api/me            — Mini App: Bearer initData → {device, online}
//	GET  /ws/device         — daemon WS: Bearer <device_token>
//	GET  /ws/app            — Mini App WS: Bearer initData
//	POST /api/send          — Mini App: Bearer initData {type, body} → device
//	GET  /app/*             — статика Mini App (./web/dist, фаза 2)
//	GET  /health
//
// Env: RELAY_TOKEN (обязателен), TELEGRAM_BOT_TOKEN (для initData),
// CARDINAL_RELAY_LISTEN (по умолчанию :8080).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/coder/websocket"

	"github.com/GringoCoco/cardinal/internal/relay"
)

const initDataMaxAge = 24 * time.Hour

type Server struct {
	hub      *relay.Hub
	relayTok string
	botToken string
	static   string
}

func main() {
	log.SetFlags(log.Ltime)
	listen := flag.String("listen", envOr("CARDINAL_RELAY_LISTEN", ":8080"), "адрес HTTP")
	static := flag.String("static", envOr("CARDINAL_RELAY_STATIC", "./web/dist"), "каталог статики Mini App")
	flag.Parse()

	relayTok := os.Getenv("RELAY_TOKEN")
	if relayTok == "" {
		log.Fatalf("RELAY_TOKEN не задан — это общий секрет для регистрации daemon'ов")
	}
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		log.Printf("TELEGRAM_BOT_TOKEN не задан — /api/pair и /ws/app не будут работать")
	}

	s := &Server{
		hub:      relay.NewHub(),
		relayTok: relayTok,
		botToken: botToken,
		static:   *static,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "relayd")
	})
	mux.HandleFunc("POST /api/register", s.handleRegister)
	mux.HandleFunc("POST /api/pair-code", s.handlePairCode)
	mux.HandleFunc("POST /api/pair", s.handlePair)
	mux.HandleFunc("GET /api/me", s.handleMe)
	mux.HandleFunc("POST /api/send", s.handleSend)
	mux.HandleFunc("GET /ws/device", s.handleDeviceWS)
	mux.HandleFunc("GET /ws/app", s.handleAppWS)
	mux.Handle("/app/", http.StripPrefix("/app/", s.staticHandler()))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	srv := &http.Server{Addr: *listen, Handler: mux}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()

	log.Printf("relayd: слушаю %s", *listen)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("relayd: %v", err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ── daemon: регистрация и код пары ─────────────────────────────────────

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+s.relayTok {
		http.Error(w, "неавторизован", http.StatusUnauthorized)
		return
	}
	var req struct {
		DeviceID string `json:"device_id"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		http.Error(w, "кривой JSON", http.StatusBadRequest)
		return
	}
	d, err := s.hub.Register(req.DeviceID, req.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]string{"device_token": d.Token})
}

func (s *Server) handlePairCode(w http.ResponseWriter, r *http.Request) {
	d := s.deviceByBearer(r)
	if d == nil {
		http.Error(w, "неавторизован", http.StatusUnauthorized)
		return
	}
	code, err := s.hub.SetPairCode(d.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]string{"code": code})
}

// ── Mini App: авторизация initData ────────────────────────────────────

// authInitData — initData из заголовка Authorization или query (?init_data=).
func (s *Server) authInitData(w http.ResponseWriter, r *http.Request) *relay.InitData {
	if s.botToken == "" {
		http.Error(w, "relay не знает токен бота", http.StatusServiceUnavailable)
		return nil
	}
	raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if raw == "" {
		raw = r.URL.Query().Get("init_data")
	}
	if raw == "" {
		http.Error(w, "нет initData", http.StatusUnauthorized)
		return nil
	}
	id, err := relay.ValidateInitData(s.botToken, raw, initDataMaxAge)
	if err != nil {
		http.Error(w, "initData: "+err.Error(), http.StatusUnauthorized)
		return nil
	}
	return id
}

func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	id := s.authInitData(w, r)
	if id == nil {
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		http.Error(w, "кривой JSON", http.StatusBadRequest)
		return
	}
	d, ok := s.hub.PairByCode(req.Code, id.ChatID)
	if !ok {
		http.Error(w, "код неверный или истёк", http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]string{"device": d.ID, "name": d.Name})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	id := s.authInitData(w, r)
	if id == nil {
		return
	}
	d := s.hub.DeviceByChat(id.ChatID)
	if d == nil {
		writeJSON(w, map[string]any{"paired": false})
		return
	}
	writeJSON(w, map[string]any{
		"paired":     true,
		"device":     d.ID,
		"name":       d.Name,
		"online":     s.hub.DeviceOnline(d),
		"snapshot_n": len(d.Snapshot),
	})
}

// handleSend — Mini App шлёт конверт своему daemon'у (не через WS).
func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	id := s.authInitData(w, r)
	if id == nil {
		return
	}
	d := s.hub.DeviceByChat(id.ChatID)
	if d == nil {
		http.Error(w, "нет привязки устройства", http.StatusForbidden)
		return
	}
	var env relay.Envelope
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<18)).Decode(&env); err != nil || env.Type == "" {
		http.Error(w, "кривой конверт", http.StatusBadRequest)
		return
	}
	env.V = 1
	env.To = "device"
	if !s.deliverToDevice(d, env) {
		http.Error(w, "daemon не на связи", http.StatusServiceUnavailable)
	}
	w.WriteHeader(http.StatusAccepted)
}

// ── WebSocket ──────────────────────────────────────────────────────────

func (s *Server) handleDeviceWS(w http.ResponseWriter, r *http.Request) {
	d := s.deviceByBearer(r)
	if d == nil {
		http.Error(w, "неавторизован", http.StatusUnauthorized)
		return
	}
	wsOpts := &websocket.AcceptOptions{OriginPatterns: []string{"*"}}
	c, err := websocket.Accept(w, r, wsOpts)
	if err != nil {
		return
	}
	ctx := r.Context()
	s.hub.AttachDeviceWS(d, c, ctx)
}

func (s *Server) handleAppWS(w http.ResponseWriter, r *http.Request) {
	// Браузерный WebSocket не умеет заголовки — initData также в query.
	id := s.authInitData(w, r)
	if id == nil {
		return
	}
	d := s.hub.DeviceByChat(id.ChatID)
	if d == nil {
		http.Error(w, "нет привязки устройства", http.StatusForbidden)
		return
	}
	wsOpts := &websocket.AcceptOptions{OriginPatterns: []string{"*"}}
	c, err := websocket.Accept(w, r, wsOpts)
	if err != nil {
		return
	}
	ctx := r.Context()
	s.hub.AttachAppWS(d, c, ctx)
}

// deliverToDevice — push конверта в WS daemon'а (используется /api/send).
func (s *Server) deliverToDevice(d *relay.Device, env relay.Envelope) bool {
	return s.hub.WriteToDevice(d, env)
}

// ── helpers ────────────────────────────────────────────────────────────

func (s *Server) deviceByBearer(r *http.Request) *relay.Device {
	return s.hub.DeviceByToken(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
}

func (s *Server) staticHandler() http.Handler {
	fs := http.FileServer(http.Dir(s.static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(s.static); err != nil {
			http.Error(w, "Mini App ещё не собран (фаза 2)", http.StatusNotFound)
			return
		}
		fs.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
