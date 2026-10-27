#!/usr/bin/env python3
"""Установка плагина Cardinal в глобальный ~/.config/opencode/opencode.json.

Безопасность — как у install_claude_hooks.py:
  * датированный бэкап, diff перед записью, подтверждение
  * существующий конфиг не переписываем молча, только добавляем запись plugins
  * повторный запуск — no-op

Запуск:  python3 scripts/install_opencode_plugin.py            # установить
         python3 scripts/install_opencode_plugin.py --uninstall # убрать
"""
import json
import os
import shutil
import subprocess
import sys
import time

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PLUGIN_DIR = os.path.join(REPO, "plugins", "opencode", "cardinal")

for name in ("opencode.json", "opencode.jsonc"):
    candidate = os.path.expanduser(f"~/.config/opencode/{name}")
    if os.path.exists(candidate):
        SETTINGS = candidate
        break
else:
    SETTINGS = os.path.expanduser("~/.config/opencode/opencode.json")

CARDINAL_MARK = "plugins/opencode/cardinal"


def load_settings():
    if not os.path.exists(SETTINGS):
        return {}
    raw = open(SETTINGS).read()
    try:
        return json.loads(raw)
    except json.JSONDecodeError:
        # jsonc с комментариями — просим добавить руками
        print(f"✗ {SETTINGS} — не чистый JSON (JSONC с комментариями?).")
        print(f"  Добавь вручную: \"plugins\": [\"{PLUGIN_DIR}\"]")
        sys.exit(1)


def save_settings(data, path):
    tmp = path + ".tmp"
    with open(tmp, "w") as fh:
        json.dump(data, fh, indent=2, ensure_ascii=False)
        fh.write("\n")
    os.replace(tmp, path)


def show_diff(before, after):
    tmp_old, tmp_new = "/tmp/.cardinal_oc_before.json", "/tmp/.cardinal_oc_after.json"
    for path, data in ((tmp_old, before), (tmp_new, after)):
        with open(path, "w") as fh:
            json.dump(data, fh, indent=2, ensure_ascii=False)
            fh.write("\n")
    p = subprocess.run(["diff", "-u", tmp_old, tmp_new], capture_output=True, text=True)
    print(p.stdout)
    os.remove(tmp_old)
    os.remove(tmp_new)


def main():
    uninstall = "--uninstall" in sys.argv
    before = load_settings()
    data = json.loads(json.dumps(before))
    plugins = data.get("plugins", [])

    if uninstall:
        fresh = [p for p in plugins if CARDINAL_MARK not in json.dumps(p)]
        if len(fresh) == len(plugins):
            print("Плагин Cardinal в opencode.json нет — нечего убирать.")
            return 0
        data["plugins"] = fresh
        if not data["plugins"]:
            del data["plugins"]
        show_diff(before, data)
        if input("Убрать плагин Cardinal из opencode.json? [y/N] ").lower() != "y":
            print("Отмена, файл не изменён.")
            return 1
        save_settings(data, SETTINGS)
        print("Убрано. Перезапусти opencode.")
        return 0

    if not os.path.isdir(PLUGIN_DIR):
        print(f"✗ нет каталога плагина: {PLUGIN_DIR}")
        return 1

    if any(CARDINAL_MARK in json.dumps(p) for p in plugins):
        print("Плагин Cardinal уже установлен — ничего не делаю.")
        return 0

    data["plugins"] = [*plugins, PLUGIN_DIR]
    show_diff(before, data)
    backup = SETTINGS + "." + time.strftime("%Y%m%d-%H%M%S") + ".bak"
    if input("Установить плагин Cardinal в opencode.json? [y/N] ").lower() != "y":
        print("Отмена, файл не изменён.")
        return 1
    if os.path.exists(SETTINGS):
        shutil.copy2(SETTINGS, backup)
        print(f"Бэкап → {backup}")
    save_settings(data, SETTINGS)
    print("Готово. Плагин подключится при следующем запуске opencode.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
