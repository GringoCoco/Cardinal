# Cardinal

Центральная система твоего Mac: ловит события агентов (Claude Code, OpenCode,
Codex…) и доставляет их на телефон через Telegram — **из любой сети**, без
Apple Developer Program, без App Store.

Карди́нал из SAO — центральный компьютер, который управляет всем. Cardinal —
daemon, который следит за агентами и будит тебя, когда происходит важное.

## Что делает (фаза 0)

| Событие | Что происходит |
|---|---|
| Агент закончил (`Stop`) | Пуш «🟢 готово» в Telegram |
| Агент ошибся | Пуш «🔴 ошибка» |
| Агент задал вопрос / ждёт внимания (`Notification`) | Пуш с текстом |
| Агент просит разрешение (`PermissionRequest`) | Пуш с кнопками **✅ Разрешить / ⛔ Запретить** — ответ уходит агенту, агент продолжается |
| Старт/конец сессии | Тихий пуш-статус |

Разрешения работают даже когда Mac за NAT, а телефон в другой сети или на
LTE — потому что обе стороны ходят в Telegram.

**Приватность как в MauMau:** в пуше по умолчанию нет ни команд, ни путей,
ни промптов — только имя агента, состояние и счётчик. Опция `show_text`
включает показ промптов (для карточки разрешения текст команды показывается
всегда — иначе решение не принять).

## Установка

Нужен Telegram-бот: найди **@BotFather** → `/newbot` → назови → скопируй токен.

```sh
git clone https://github.com/GringoCoco/Cardinal ~/Projects/cardinal
cd ~/Projects/cardinal

# 1. собери daemon
go build -o ~/.local/bin/cardinald ./cmd/cardinald

# 2. привяжи бота (введёт токен, потом пришлёт /start <код> боту)
cardinald pair

# 3. поставь хуки Claude Code (бэкап + diff + подтверждение)
cp hooks/cardinal-hook ~/.local/bin/ && chmod +x ~/.local/bin/cardinal-hook
python3 scripts/install_claude_hooks.py

# 4. запуск
cardinald run
```

Проверка: `cardinald testevent` — придёт тестовое уведомление.

## Как это работает

```
Claude Code ──hook──► cardinal-hook ──POST──► cardinald (:9723, localhost)
                                                    │
                                              Telegram Bot API
                                                    ▼
                                          твой телефон (любая сеть)
                                                    │
                                     кнопка Allow/Deny (callback)
                                                    ▼
                                     cardinald отвечает хуку JSON-контрактом
```

- `cardinald` слушает только `127.0.0.1` — наружу ничего не торчит.
- Токен бота и `chat_id` лежат в `~/.config/cardinal/config.json` (права 0600).
- Если daemon выключен — хук мгновенно выходит, агент спрашивает разрешение
  в терминале как обычно (правило MauMau: **никогда не блокировать агента**).

## Дорожная карта

- [x] **Фаза 0** — хуки → Telegram-пуш, Allow/Deny из кнопок бота
- [ ] **Фаза 1** — свой relay (Go + WebSocket): Mini App с живым статусом сессий
- [ ] **Фаза 2** — Telegram Mini App (Preact): промпты агентам прямо из телефона
  (`claude -p --resume`, `codex resume`)
- [ ] **Фаза 3** — история буфера обмена Mac в Mini App (по запросу)
- [ ] **Фаза 4** — menu-bar/notch приложение на Swift: сгусток энергии в вырезе,
  статус агентов, быстрые действия

## Лицензия

MIT
