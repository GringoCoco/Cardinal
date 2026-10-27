#!/bin/zsh
# Установка LaunchAgent: cardinald стартует при входе и перезапускается при падении.
set -euo pipefail

BIN="${CARDINALD:-$HOME/.local/bin/cardinald}"
PLIST_SRC="$(cd "$(dirname "$0")" && pwd)/com.cardinal.daemon.plist"
PLIST_DST="$HOME/Library/LaunchAgents/com.cardinal.daemon.plist"

[[ -x "$BIN" ]] || { echo "✗ нет $BIN — собери: go build -o ~/.local/bin/cardinald ./cmd/cardinald"; exit 1; }

mkdir -p "$HOME/Library/LaunchAgents"
# Подставляем реальный $HOME вместо REPLACE_ME (launchd не раскрывает ~).
sed "s|/Users/REPLACE_ME|$HOME|g" "$PLIST_SRC" > "$PLIST_DST"

launchctl unload "$PLIST_DST" 2>/dev/null || true
launchctl load "$PLIST_DST"
sleep 1
label=$(launchctl list | grep com.cardinal.daemon || true)
echo "Установлен и запущен: $label"
echo "Лог: /tmp/cardinald.log"
echo ""
echo "Убрать: launchctl unload $PLIST_DST && rm $PLIST_DST"
