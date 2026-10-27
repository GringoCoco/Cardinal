#!/usr/bin/env python3
"""Установка хуков Cardinal в ~/.claude/settings.json.

Безопасность (правило MauMau):
  * ни-ког-да не перезаписываем файл молча: датированный бэкап + merge
  * перед записью показываем diff и спрашиваем подтверждение
  * уже существующие хуки не трогаем, только дописываем Cardinal

Запуск:  python3 scripts/install_claude_hooks.py            # установить
         python3 scripts/install_claude_hooks.py --uninstall # убрать
"""
import json
import os
import shutil
import subprocess
import sys
import time

CARDINAL_HOOK = os.path.expanduser("~/.local/bin/cardinal-hook")
SETTINGS = os.path.expanduser("~/.claude/settings.json")

# События, нужные Cardinal фазы 0:
#   Stop             — «агент закончил генерацию»
#   Notification     — «ждёт твоего внимания / задал вопрос»
#   PermissionRequest — карточка с Allow/Deny
#   SessionStart/SessionEnd — тихие статусы
EVENTS = ["Stop", "Notification", "PermissionRequest", "SessionStart", "SessionEnd"]
# SessionStart/SessionEnd придут тихими пуши; это настраивается в daemon.

HOOK_CMD = {
    "command": CARDINAL_HOOK,
    "timeout": 120,  # PermissionRequest висит до 110 с на стороне daemon
}


def load_settings():
    if not os.path.exists(SETTINGS):
        return {}
    with open(SETTINGS) as fh:
        return json.load(fh)


def save_settings(data, path):
    tmp = path + ".tmp"
    with open(tmp, "w") as fh:
        json.dump(data, fh, indent=2, ensure_ascii=False)
        fh.write("\n")
    os.replace(tmp, path)


def hook_entries():
    return [dict(HOOK_CMD)]


def cardinal_hook_marker(entry):
    return isinstance(entry, dict) and entry.get("command") == CARDINAL_HOOK


def merge(data):
    hooks = data.setdefault("hooks", {})
    added, skipped = [], []
    for event in EVENTS:
        groups = hooks.setdefault(event, [])
        for group in groups:
            if any(cardinal_hook_marker(h) for h in group.get("hooks", [])):
                skipped.append(event)
                break
        else:
            groups.append({"hooks": hook_entries()})
            added.append(event)
    return added, skipped


def unmerge(data):
    removed = []
    hooks = data.get("hooks", {})
    for event in list(hooks):
        groups = hooks[event]
        fresh = []
        for group in groups:
            hs = [h for h in group.get("hooks", []) if not cardinal_hook_marker(h)]
            if hs:
                fresh.append({**group, "hooks": hs})
            else:
                removed.append(event)
        if fresh:
            hooks[event] = fresh
        else:
            del hooks[event]
            removed.append(event)
    return sorted(set(removed))


def show_diff(before, after):
    """diff через утилиту diff -u (есть на любой macOS)."""
    tmp_old = "/tmp/.cardinal_settings_before.json"
    tmp_new = "/tmp/.cardinal_settings_after.json"
    with open(tmp_old, "w") as fh:
        json.dump(before, fh, indent=2, ensure_ascii=False)
        fh.write("\n")
    with open(tmp_new, "w") as fh:
        json.dump(after, fh, indent=2, ensure_ascii=False)
        fh.write("\n")
    p = subprocess.run(["diff", "-u", tmp_old, tmp_new], capture_output=True, text=True)
    print(p.stdout)
    os.remove(tmp_old)
    os.remove(tmp_new)


def main():
    uninstall = "--uninstall" in sys.argv
    before = load_settings()
    data = json.loads(json.dumps(before))  # deep copy

    if uninstall:
        removed = unmerge(data)
        if not removed:
            print("Хуков Cardinal в settings.json нет — нечего убирать.")
            return 0
        show_diff(before, data)
        if input(f"Убрать хуки {', '.join(removed)}? [y/N] ").lower() != "y":
            print("Отмена, файл не изменён.")
            return 1
        save_settings(data, SETTINGS)
        print("Убрано.")
        return 0

    if not os.path.exists(CARDINAL_HOOK):
        print(f"✗ {CARDINAL_HOOK} не найден.")
        print("  Сначала: cp hooks/cardinal-hook ~/.local/bin/ && chmod +x ~/.local/bin/cardinal-hook")
        return 1

    added, skipped = merge(data)
    if not added:
        print("Хуки Cardinal уже установлены — всё есть, ничего не делаю.")
        return 0

    show_diff(before, data)
    backup = SETTINGS + "." + time.strftime("%Y%m%d-%H%M%S") + ".bak"
    if input("Установить эти хуки в ~/.claude/settings.json? [y/N] ").lower() != "y":
        print("Отмена, файл не изменён.")
        return 1
    if os.path.exists(SETTINGS):
        shutil.copy2(SETTINGS, backup)
        print(f"Бэкап старого settings.json → {backup}")
    save_settings(data, SETTINGS)
    if skipped:
        print(f"Уже установлены и не тронуты: {', '.join(skipped)}")
    print(f"Установлены события: {', '.join(added)}")
    print("\nПерезапусти сессию Claude Code, чтобы хуки подхватились.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
