// cardinald — daemon Cardinal: хуки агентов (Mac) → Telegram (телефон).
//
// Кардинал из SAO — центральная система, которая управляет всем и будит
// человека, когда происходит важное.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/GringoCoco/cardinal/internal/config"
	"github.com/GringoCoco/cardinal/internal/daemon"
	"github.com/GringoCoco/cardinal/internal/tg"
)

const usage = `cardinald — daemon Cardinal (https://github.com/GringoCoco/Cardinal)

Команды:
  cardinald pair        привязать бота: спрашивает токен, потом код из твоего чата
  cardinald run         запустить daemon (хуки :9723 + пуши в Telegram)
  cardinald connect URL подключить daemon к relay (Mini App): регистрирует устройство
  cardinald pair-code   выпустить новый код пары Mini App (10 минут)
  cardinald testevent   отправить тестовое уведомление в Telegram
  cardinald testperm    тест карточки разрешения: жми кнопку — увидишь решение здесь
  cardinald version     версия
`

func main() {
	log.SetFlags(log.Ltime)
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}
	var err error
	switch os.Args[1] {
	case "pair":
		err = cmdPair()
	case "run":
		err = cmdRun()
	case "testevent":
		err = cmdTestEvent()
	case "testperm":
		err = cmdTestPerm()
	case "connect":
		err = cmdConnect(os.Args[2:])
	case "pair-code":
		err = cmdRelayPairCode()
	case "version":
		fmt.Println("cardinald 0.2.0 (сессии → Telegram Mini App)")
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}
	if err != nil {
		log.Fatalf("ошибка: %v", err)
	}
}

func cmdRelayPairCode() error {
	cfg, err := loadCfg()
	if err != nil {
		return err
	}
	if cfg.RelayURL == "" || cfg.DeviceToken == "" {
		return fmt.Errorf("relay не настроен — сначала «cardinald connect URL»")
	}
	base, err := url.Parse(strings.TrimRight(cfg.RelayURL, "/"))
	if err != nil || base.Host == "" {
		return fmt.Errorf("кривой relay_url в конфиге")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	code, err := pairCode(ctx, base, cfg.DeviceToken)
	if err != nil {
		return err
	}
	fmt.Printf("Код пары для Mini App: %s\n", code)
	fmt.Println("Код одноразовый и действует 10 минут.")
	return nil
}

func loadCfg() (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if cfg.Port == 0 {
		cfg.Port = config.DefaultPort
	}
	return cfg, nil
}

// cmdRun — основной режим.
func cmdRun() error {
	cfg, err := loadCfg()
	if err != nil {
		return err
	}
	if cfg.BotToken == "" && cfg.RelayURL == "" {
		return fmt.Errorf("ничего не настроено — сначала запусти «cardinald pair» (и, при желании, «cardinald connect URL»)")
	}
	if cfg.BotToken == "" {
		log.Printf("bot_token пуст — Telegram-пуши выключены, работаю только по relay")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return daemon.New(cfg).Run(ctx)
}

// cmdPair — привязка бота и чата владельца.
func cmdPair() error {
	cfg, err := loadCfg()
	if err != nil {
		return err
	}
	if cfg.BotToken == "" {
		fmt.Print("Сначала создай бота у @BotFather и вставь его токен (Enter): ")
		var token string
		if _, err := fmt.Scanln(&token); err != nil {
			return fmt.Errorf("не считал токен: %w", err)
		}
		cfg.BotToken = token
		_, uname, err := probeBot(cfg.BotToken)
		if err != nil {
			return err
		}
		fmt.Printf("Бот @%s отвечает ✓\n", uname)
	}

	client := tg.New(cfg.BotToken)
	secretCode, err := daemon.NewCode()
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Printf("Открой диалог со своим ботом и отправь ровно эту строку:\n\n")
	fmt.Printf("    /start %s\n\n", secretCode)
	fmt.Println("Код одноразовый и живёт 10 минут. Периодически проверяю сообщения…")

	deadline := time.Now().Add(10 * time.Minute)
	offset := 0
	for time.Now().Before(deadline) {
		updates, err := client.GetUpdates(offset, 25)
		if err != nil {
			return fmt.Errorf("getUpdates: %w", err)
		}
		for _, u := range updates {
			offset = u.UpdateID + 1
			if u.Message == nil || u.Message.Text != "/start "+secretCode {
				continue
			}
			cfg.ChatID = u.Message.Chat.ID
			if err := cfg.Save(); err != nil {
				return err
			}
			client.Send(cfg.ChatID, "✅ Cardinal привязан к этому Mac. Уведомления об агентских событиях будут приходить сюда.",
				false, nil)
			fmt.Printf("Готово: chat_id %d сохранён в %s\n", cfg.ChatID, config.Path())
			return nil
		}
	}
	return fmt.Errorf("код не введён за 10 минут")
}

func probeBot(token string) (string, string, error) {
	client := tg.New(token)
	uname, err := client.Me()
	if err != nil {
		return "", "", fmt.Errorf("токен не работает: %w", err)
	}
	return token, uname, nil
}

// cmdTestEvent — оповещения Telegram без агентов (для проверки).
func cmdTestEvent() error {
	cfg, err := loadCfg()
	if err != nil {
		return err
	}
	if cfg.BotToken == "" || cfg.ChatID == 0 {
		return fmt.Errorf("сначала «cardinald pair»")
	}
	client := tg.New(cfg.BotToken)
	client.Send(cfg.ChatID, "🧪 Тестовое уведомление Cardinal — если видишь это, пуш работает из любой сети.", false, nil)
	fmt.Println("Уведомление отправлено.")
	return nil
}

// cmdTestPerm — детерминированный тест цикла разрешения без участия модели:
// шлём PermissionRequest в локальный daemon, ждём решения из Telegram.
// Решение печатаем в терминал — так проверяются карточка и кнопки,
// даже когда тестируемая модель капризничает.
func cmdTestPerm() error {
	cfg, err := loadCfg()
	if err != nil {
		return err
	}
	if cfg.BotToken == "" || cfg.ChatID == 0 {
		return fmt.Errorf("сначала «cardinald pair»")
	}

	wait := cfg.DecisionWaitSec
	if wait <= 0 {
		wait = 110
	}
	sid := fmt.Sprintf("testperm-%d", time.Now().Unix())
	cwd, _ := os.Getwd()
	if cwd == "" {
		cwd = "/"
	}
	payload := fmt.Sprintf(`{"hook_event_name":"PermissionRequest","session_id":%q,`+
		`"coucou_agent":"Cardinal Test","tool_name":"shell",`+
		`"tool_input":{"command":"echo cardinal-testperm"},"cwd":%q}`, sid, cwd)

	url := fmt.Sprintf("http://127.0.0.1:%d/hook", cfg.Port)
	fmt.Printf("→ PermissionRequest (session %s) в %s\n", sid, url)
	fmt.Printf("  Карточка придёт в Telegram. Окно решения: %d с. Жми кнопку — решение появится здесь.\n", wait)

	client := &http.Client{Timeout: time.Duration(wait+20) * time.Second}
	resp, err := client.Post(url, "application/json", strings.NewReader(payload))
	if err != nil {
		return fmt.Errorf("daemon не отвечает (запущен ли «cardinald run»?): %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var out struct {
		HookSpecificOutput struct {
			Decision struct {
				Behavior string `json:"behavior"`
			} `json:"decision"`
		} `json:"hookSpecificOutput"`
	}
	_ = json.Unmarshal(raw, &out)
	switch out.HookSpecificOutput.Decision.Behavior {
	case "allow":
		fmt.Println("✅ Решение из Telegram: РАЗРЕШИТЬ")
	case "deny":
		fmt.Println("⛔ Решение из Telegram: ЗАПРЕТИТЬ")
	default:
		fmt.Printf("⌛ Решения не пришло (таймаут %d с). Ответ daemon: %s\n", wait, string(raw))
	}
	return nil
}

// cmdConnect — регистрация устройства в relay и настройка конфига.
// После неё «cardinald run» держит WS к relay: Mini App видит статусы,
// решает карточки разрешений и шлёт промпты (фаза 2).
func cmdConnect(args []string) error {
	cfg, err := loadCfg()
	if err != nil {
		return err
	}
	if len(args) < 1 || args[0] == "" {
		return fmt.Errorf("укажи адрес relay: cardinald connect https://cardinal-relay.fly.dev")
	}
	rawURL := args[0]
	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}
	base, err := url.Parse(strings.TrimRight(rawURL, "/"))
	if err != nil || base.Host == "" {
		return fmt.Errorf("кривой URL: %s", rawURL)
	}

	// RELAY_TOKEN: флаг -token, окружение или ввод с терминала.
	relayToken := ""
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		if rest[i] == "-token" || rest[i] == "--token" {
			if i+1 < len(rest) {
				relayToken = rest[i+1]
				i++
			}
		}
	}
	if relayToken == "" {
		relayToken = os.Getenv("CARDINAL_RELAY_TOKEN")
	}
	if relayToken == "" {
		fmt.Print("RELAY_TOKEN (общий секрет relay): ")
		if _, err := fmt.Scanln(&relayToken); err != nil {
			return fmt.Errorf("не считал RELAY_TOKEN: %w", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tok, did, err := registerRelayDevice(ctx, base, relayToken)
	if err != nil {
		return err
	}

	cfg.RelayURL = base.Scheme + "://" + base.Host
	cfg.RelayToken = relayToken
	cfg.DeviceToken = tok
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Printf("✅ Устройство %s зарегистрировано в %s\n", did, cfg.RelayURL)
	// Код пары для Mini App (годен 10 минут, но можно выпустить снова).
	if code, err := pairCode(ctx, base, tok); err == nil {
		fmt.Printf("   Код пары для Mini App: %s\n", code)
	}
	fmt.Println("   Перезапусти «cardinald run» — подключится к relay по WS.")
	return nil
}

// pairCode — POST /api/pair-code (Bearer device_token).
func pairCode(ctx context.Context, base *url.URL, deviceToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base.Scheme+"://"+base.Host+"/api/pair-code", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var out struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.Code == "" {
		return "", fmt.Errorf("кривой ответ")
	}
	return out.Code, nil
}

// registerRelayDevice — POST /api/register у relay.
func registerRelayDevice(ctx context.Context, base *url.URL, relayToken string) (string, string, error) {
	did := daemon.DeviceID()
	body, _ := json.Marshal(map[string]string{"device_id": did, "name": did})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base.Scheme+"://"+base.Host+"/api/register", strings.NewReader(string(body)))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+relayToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("relay недоступен: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return "", "", fmt.Errorf("RELAY_TOKEN отклонён (HTTP 401)")
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("register: HTTP %d", resp.StatusCode)
	}
	var out struct {
		DeviceToken string `json:"device_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.DeviceToken == "" {
		return "", "", fmt.Errorf("register: кривой ответ relay")
	}
	return out.DeviceToken, did, nil
}
