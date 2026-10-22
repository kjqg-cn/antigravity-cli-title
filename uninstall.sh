#!/usr/bin/env bash
# uninstall.sh - Uninstaller for Antigravity CLI Window Title (Linux / macOS)
set -euo pipefail

PURGE=false
while [[ $# -gt 0 ]]; do
    case "$1" in
        --purge) PURGE=true; shift ;;
        *) shift ;;
    esac
done

echo "=========================================================="
echo "  🗑️ Uninstalling Antigravity CLI Window Title (POSIX)   "
echo "=========================================================="

HOME_DIR="${HOME:-/root}"
SETTINGS_PATH="${HOME_DIR}/.gemini/antigravity-cli/settings.json"

if [[ -f "$SETTINGS_PATH" ]]; then
    python3 -c "
import json, os

settings_path = '$SETTINGS_PATH'
if os.path.exists(settings_path):
    try:
        with open(settings_path, 'r', encoding='utf-8') as f:
            data = json.load(f)
        if 'title' in data:
            del data['title']
            with open(settings_path, 'w', encoding='utf-8') as f:
                json.dump(data, f, indent=2, ensure_ascii=False)
            print('[+] Removed title configuration from settings.json')
    except Exception as e:
        print(f'[-] Error updating settings.json: {e}')
"
fi

TARGET_DIR="${HOME_DIR}/.gemini/antigravity-cli-title"
LEGACY_DIR="${HOME_DIR}/.antigravity"

if [[ "$PURGE" == true ]]; then
    if [[ -d "$TARGET_DIR" ]]; then
        rm -rf "$TARGET_DIR"
        echo "[+] Purged installation directory: $TARGET_DIR"
    fi
    if [[ -f "${LEGACY_DIR}/title.sh" ]]; then
        rm -f "${LEGACY_DIR}/title.sh"
        echo "[+] Purged legacy file: ${LEGACY_DIR}/title.sh"
    fi
else
    if [[ -d "$TARGET_DIR" ]]; then
        echo "[i] Scripts preserved at: $TARGET_DIR (Use --purge to completely remove directory)"
    fi
fi

echo ""
echo "✨ Uninstallation completed."
echo "=========================================================="
