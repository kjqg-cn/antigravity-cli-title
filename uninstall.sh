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
SETTINGS_DIR="${HOME_DIR}/.gemini/antigravity-cli"
SETTINGS_PATH="${SETTINGS_DIR}/settings.json"
BAK_PATH="${SETTINGS_DIR}/settings.json.pre-title.bak"

PYTHON_BIN=""
if python3 -c 'import json' >/dev/null 2>&1; then
    PYTHON_BIN="python3"
elif python -c 'import json' >/dev/null 2>&1; then
    PYTHON_BIN="python"
fi

SETTINGS_UPDATE_FAILED=false
if [[ -f "$SETTINGS_PATH" ]]; then
    if [[ -n "$PYTHON_BIN" ]]; then
        if ! $PYTHON_BIN - "$SETTINGS_PATH" "$BAK_PATH" << 'EOF'
import sys, json, os

settings_path = sys.argv[1]
bak_path = sys.argv[2]

if os.path.exists(settings_path):
    try:
        with open(settings_path, 'r', encoding='utf-8-sig') as f:
            content = f.read().strip()
            if content:
                data = json.loads(content)
            else:
                data = {}
        if not isinstance(data, dict):
            raise ValueError("settings.json root must be an object; file preserved")

        current_title = data.get('title')
        current_cmd = str(current_title.get('command', '')) if isinstance(current_title, dict) else str(current_title or '')
        if current_title is None or 'antigravity-cli-title' not in current_cmd:
            print('[i] Current title belongs to another configuration; settings and backup preserved.')
            sys.exit(0)

        eff_bak = bak_path if os.path.exists(bak_path) else None
        restored = False
        if eff_bak:
            try:
                with open(eff_bak, 'r', encoding='utf-8-sig') as bf:
                    bak_data = json.load(bf)
                if isinstance(bak_data, dict) and 'title' in bak_data and bak_data['title']:
                    bak_title = bak_data['title']
                    bak_cmd = str(bak_title.get('command', '')) if isinstance(bak_title, dict) else str(bak_title)
                    if bak_cmd and 'antigravity-cli-title' not in bak_cmd:
                        data['title'] = bak_data['title']
                        with open(settings_path, 'w', encoding='utf-8') as f:
                            json.dump(data, f, indent=2, ensure_ascii=False)
                        print(f'[+] Restored previous title configuration from backup: {eff_bak}')
                        restored = True
                    else:
                        print(f'[i] Backup title points to antigravity-cli-title, skipping restore.')
            except Exception:
                pass

        if not restored:
            if 'title' in data:
                del data['title']
                with open(settings_path, 'w', encoding='utf-8') as f:
                    json.dump(data, f, indent=2, ensure_ascii=False)
                print('[+] Removed title configuration from settings.json')
            else:
                print('[i] No title configuration found in settings.json')

        if os.path.exists(bak_path):
            os.remove(bak_path)
    except Exception as e:
        print(f'[-] Error updating settings.json: {e}')
        sys.exit(1)
EOF
        then
            SETTINGS_UPDATE_FAILED=true
        fi
    elif command -v jq >/dev/null 2>&1; then
        if ! jq -e 'type == "object"' "$SETTINGS_PATH" >/dev/null 2>&1; then
            echo "[-] settings.json root is not a valid object; preserved."
            exit 1
        fi
        CURRENT_CMD=$(jq -r '.title | if type == "object" then (.command // "") else (. // "" | tostring) end' "$SETTINGS_PATH")
        if [[ "$CURRENT_CMD" != *"antigravity-cli-title"* ]]; then
            echo "[i] Current title belongs to another configuration; settings and backup preserved."
        else
            RESTORED=false
            UPDATED=false
            if [[ -f "$BAK_PATH" ]]; then
                OLD_TITLE=$(jq '.title' "$BAK_PATH" 2>/dev/null || echo "null")
                if [[ "$OLD_TITLE" != "null" && "$OLD_TITLE" != "" ]]; then
                    OLD_CMD=$(echo "$OLD_TITLE" | jq -r 'if type == "object" then (.command // "") else tostring end' 2>/dev/null || echo "")
                    if [[ -n "$OLD_CMD" && "$OLD_CMD" != *"antigravity-cli-title"* ]]; then
                        if jq -e 'type == "object"' "$SETTINGS_PATH" >/dev/null 2>&1 && jq --argjson old "$OLD_TITLE" '.title = $old' "$SETTINGS_PATH" > "${SETTINGS_PATH}.tmp" 2>/dev/null; then
                            mv "${SETTINGS_PATH}.tmp" "$SETTINGS_PATH"
                            echo "[+] Restored previous title configuration from backup via jq: $BAK_PATH"
                            RESTORED=true
                            UPDATED=true
                        else
                            rm -f "${SETTINGS_PATH}.tmp"
                        fi
                    else
                        echo "[i] Backup title points to antigravity-cli-title, skipping restore."
                    fi
                fi
            fi

            if [[ "$RESTORED" == false ]]; then
                if jq -e 'if type == "object" then del(.title) else error("Root element is not an object") end' "$SETTINGS_PATH" > "${SETTINGS_PATH}.tmp" 2>/dev/null; then
                    mv "${SETTINGS_PATH}.tmp" "$SETTINGS_PATH"
                    echo "[+] Removed title configuration from settings.json via jq"
                    UPDATED=true
                else
                    rm -f "${SETTINGS_PATH}.tmp"
                    echo "[-] Failed to update settings.json via jq (file may be invalid JSON)"
                fi
            fi
            if [[ "$UPDATED" == true ]]; then
                rm -f "$BAK_PATH"
            else
                SETTINGS_UPDATE_FAILED=true
            fi
        fi
    else
        if grep -q '"title"' "$SETTINGS_PATH" 2>/dev/null; then
            echo "[-] Neither Python nor jq is available to safely update '${SETTINGS_PATH}'."
            echo "    Please manually edit '${SETTINGS_PATH}' and remove or restore the 'title' block."
            exit 1
        fi
    fi
fi

TARGET_DIR="${HOME_DIR}/.gemini/antigravity-cli-title"

if [[ "$PURGE" == true && "$SETTINGS_UPDATE_FAILED" != true ]]; then
    if [[ -d "$TARGET_DIR" ]]; then
        rm -rf "$TARGET_DIR"
        echo "[+] Purged installation directory: $TARGET_DIR"
    fi
    for backup in "${SETTINGS_DIR}"/settings.json.*.bak; do
        if [[ "${backup##*/}" =~ ^settings\.json\.[0-9]{14}\.bak$ ]]; then rm -f "$backup"; fi
    done
else
    if [[ -d "$TARGET_DIR" ]]; then
        echo "[i] Files preserved at: $TARGET_DIR (Use --purge to completely remove directory)"
    fi
fi

if [[ "$SETTINGS_UPDATE_FAILED" == true ]]; then
    echo ""
    echo "[-] Warning: settings.json could not be updated. Please check manually."
    echo "=========================================================="
    exit 1
fi

echo ""
echo "✨ Uninstallation completed."
echo "=========================================================="
