#!/usr/bin/env bash
# install.sh - Installer for Antigravity CLI Window Title (Linux / macOS)
set -euo pipefail

SHOW_PREFIX=true
USE_TEXT=false
CUSTOM_PREFIX="agy:"
MAX_LEN=35

while [[ $# -gt 0 ]]; do
    case "$1" in
        --no-prefix) SHOW_PREFIX=false; shift ;;
        --text|--classic) USE_TEXT=true; shift ;;
        --max-length) MAX_LEN="$2"; shift 2 ;;
        --prefix) CUSTOM_PREFIX="$2"; shift 2 ;;
        *) shift ;;
    esac
done

echo "=========================================================="
echo "   🚀 Antigravity CLI Window Title Installer (POSIX)      "
echo "=========================================================="

HOME_DIR="${HOME:-/root}"
TARGET_DIR="${HOME_DIR}/.gemini/antigravity-cli-title"
mkdir -p "$TARGET_DIR"

REPO_BASE_URL="https://raw.githubusercontent.com/kjqg-cn/antigravity-cli-title/main"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" 2>/dev/null && pwd || echo "")"

SOURCE_TITLE="${SCRIPT_DIR}/scripts/title.sh"
SOURCE_UNINSTALL="${SCRIPT_DIR}/uninstall.sh"

DEST_FILE="${TARGET_DIR}/title.sh"
DEST_UNINSTALL="${TARGET_DIR}/uninstall.sh"

if [[ -n "$SCRIPT_DIR" && -f "$SOURCE_TITLE" ]]; then
    cp "$SOURCE_TITLE" "$DEST_FILE"
    echo "[+] Installed renderer to: $DEST_FILE"
else
    echo "[*] Fetching renderer from GitHub..."
    curl -fsSL "${REPO_BASE_URL}/scripts/title.sh" -o "$DEST_FILE"
    echo "[+] Downloaded renderer to: $DEST_FILE"
fi
chmod +x "$DEST_FILE"

if [[ -n "$SCRIPT_DIR" && -f "$SOURCE_UNINSTALL" ]]; then
    cp "$SOURCE_UNINSTALL" "$DEST_UNINSTALL"
    echo "[+] Installed uninstaller to: $DEST_UNINSTALL"
else
    echo "[*] Fetching uninstaller from GitHub..."
    curl -fsSL "${REPO_BASE_URL}/uninstall.sh" -o "$DEST_UNINSTALL" 2>/dev/null || true
    echo "[+] Downloaded uninstaller to: $DEST_UNINSTALL"
fi
chmod +x "$DEST_UNINSTALL" 2>/dev/null || true

# Build arguments
ARGS=""
[[ "$SHOW_PREFIX" == false ]] && ARGS="${ARGS} --no-prefix"
[[ "$USE_TEXT" == true ]] && ARGS="${ARGS} --text"
[[ "$CUSTOM_PREFIX" != "agy:" ]] && ARGS="${ARGS} --prefix \"$CUSTOM_PREFIX\""
[[ "$MAX_LEN" != 35 ]] && ARGS="${ARGS} --max-length $MAX_LEN"

TITLE_CMD="${DEST_FILE}${ARGS}"

SETTINGS_DIR="${HOME_DIR}/.gemini/antigravity-cli"
SETTINGS_PATH="${SETTINGS_DIR}/settings.json"
mkdir -p "$SETTINGS_DIR"

if [[ -f "$SETTINGS_PATH" ]]; then
    cp "$SETTINGS_PATH" "${SETTINGS_PATH}.bak"
    echo "[+] Backup saved to: ${SETTINGS_PATH}.bak"
fi

python3 -c "
import json, os

settings_path = '$SETTINGS_PATH'
title_cmd = '$TITLE_CMD'

data = {}
if os.path.exists(settings_path):
    try:
        with open(settings_path, 'r', encoding='utf-8') as f:
            data = json.load(f)
    except Exception:
        data = {}

data['title'] = {
    'type': 'command',
    'command': title_cmd,
    'enabled': True
}

with open(settings_path, 'w', encoding='utf-8') as f:
    json.dump(data, f, indent=2, ensure_ascii=False)
"

echo "[+] Successfully configured 'title' in: $SETTINGS_PATH"
echo ""
echo "✨ Installation complete!"
echo "   Command configured: $TITLE_CMD"
echo "   Installed directory: $TARGET_DIR"
echo "   To uninstall later, run:"
echo "     ${DEST_UNINSTALL}"
echo ""
echo "   ⚪ Ready     -> Initial state / re-entering history"
echo "   ⚙️  Working   -> Thinking / background task execution"
echo "   ⚠️  Waiting   -> Prompting user (ask_question) or permission confirmation"
echo "   ✅ Done      -> Turn completed; remains done until new input"
echo "=========================================================="
