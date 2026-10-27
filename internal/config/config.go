// Package config — конфиг демона.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultPort — порт HTTP-сервера хуков на localhost.
const DefaultPort = 9723

// HookPath — где живёт скрипт-хук (сюда его кладёт установщик).
const HookPath = "~/.local/bin/cardinal-hook"

type Config struct {
	BotToken string `json:"bot_token"` // токен от @BotFather
	ChatID   int64  `json:"chat_id"`   // чат владельца (заполняет cardinald pair)

	Port     int  `json:"port"`      // HTTP-порт хуков (0 = DefaultPort)
	ShowText bool `json:"show_text"` // показывать текст промптов/команд в пуше (по умолчанию нет — приватность)

	// Дополнительные чаты, которым разрешено отвечать (Allow/Deny). Пусто = только владелец.
	ExtraChats []int64 `json:"extra_chats,omitempty"`
}

// Dir — каталог конфига. Переопределяется CARDINAL_HOME.
func Dir() string {
	if home := os.Getenv("CARDINAL_HOME"); home != "" {
		return home
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "cardinal")
}

func Path() string { return filepath.Join(Dir(), "config.json") }

// Load читает конфиг; отсутствие файла — не ошибка, вернёт дефолтный_ports.
func Load() (*Config, error) {
	data, err := os.ReadFile(Path())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, err
	}
	cfg := &Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("конфиг %s повреждён: %w", Path(), err)
	}
	if cfg.Port == 0 {
		cfg.Port = DefaultPort
	}
	return cfg, nil
}

// Save атомарно пишет конфиг (tmp + rename), права 0600 — в токене секрет.
func (c *Config) Save() error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := Path() + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, Path())
}

// Expand разворачивает "~/..." в абсолютный путь.
func Expand(path string) string {
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[1:])
	}
	return path
}

// TelegramLink — t.me-ссылка на бота по токену.
func TelegramLink(token string) string {
	parts := strings.SplitN(token, ":", 2)
	if len(parts) == 2 {
		return "https://t.me/" + parts[0]
	}
	return ""
}
