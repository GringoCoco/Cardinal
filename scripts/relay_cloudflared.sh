#!/usr/bin/env bash
# relay_cloudflared.sh — relayd на Mac + Cloudflare quick-tunnel наружу.
#
# Быстрый туннель не требует аккаунта Cloudflare, но URL меняется при
# каждом запуске — скрипт сам прогоняет «cardinald connect» с новым URL.
# Стабильный адрес (named tunnel) — когда появится домен в Cloudflare.
#
# Использование: scripts/relay_cloudflared.sh [порт, по умолчанию 8080]
set -euo pipefail

PORT="${1:-8080}"
command -v cloudflared >/dev/null || { echo "нет cloudflared — brew install cloudflared"; exit 1; }
command -v relayd     >/dev/null || { echo "нет relayd в PATH (go build -o ~/.local/bin/relayd ./cmd/relayd)"; exit 1; }
command -v cardinald  >/dev/null || { echo "нет cardinald в PATH"; exit 1; }
CFG="$HOME/.config/cardinal/config.json"
[ -f "$CFG" ] || { echo "нет конфига — сначала «cardinald pair»"; exit 1; }

BOT_TOKEN="$(/usr/bin/sed -n 's/.*"bot_token" *: *"\([^"]*\)".*/\1/p' "$CFG")"

# RELAY_TOKEN: из env, иначе генерируем (только для этой машины — уходит в конфиг daemon'а).
RELAY_TOKEN="${CARDINAL_RELAY_TOKEN:-$(/usr/bin/openssl rand -hex 24)}"

echo "→ поднимаю relayd на 127.0.0.1:$PORT"
RELAY_TOKEN="$RELAY_TOKEN" TELEGRAM_BOT_TOKEN="$BOT_TOKEN" \
  relayd -listen "127.0.0.1:$PORT" &
relay_pid=$!
cf_pid=""
trap 'kill ${relay_pid:-} ${cf_pid:-} 2>/dev/null || true' EXIT
until curl -s "http://127.0.0.1:$PORT/health" >/dev/null 2>&1; do sleep 0.2; done

echo "→ поднимаю quick-туннель cloudflared"
cloudflared tunnel --url "http://127.0.0.1:$PORT" --no-autoupdate > /tmp/cardinal-cloudflared.log 2>&1 &
cf_pid=$!
url=""
for _ in $(seq 1 60); do
  url="$(/usr/bin/grep -oE 'https://[a-z0-9-]+\.trycloudflare\.com' /tmp/cardinal-cloudflared.log | head -1 || true)"
  [ -n "$url" ] && break
  sleep 1
done
[ -n "$url" ] || { echo "туннель не поднялся — смотри /tmp/cardinal-cloudflared.log"; exit 1; }
echo "→ туннель: $url"

echo "→ cardinald connect $url"
CARDINAL_RELAY_TOKEN="$RELAY_TOKEN" cardinald connect "$url"
echo
echo "Готово. «cardinald run» теперь работает через relay. Логи туннеля: /tmp/cardinal-cloudflared.log"
wait
