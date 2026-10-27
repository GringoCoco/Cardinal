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
	case "version":
		fmt.Println("cardinald 0.1.0 (фаза 0: хуки → Telegram)")
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
	if cfg.BotToken == "" {
		return fmt.Errorf("нет bot_token — сначала запусти «cardinald pair»")
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
	payload := fmt.Sprintf(`{"hook_event_name":"PermissionRequest","session_id":%q,`+
		`"coucou_agent":"Cardinal Test","tool_name":"shell",`+
		`"tool_input":{"command":"echo cardinal-testperm"},"cwd":"%s"}`, sid, config.Path())

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
