#!/usr/bin/env bash
# install.sh - Native Installer for Antigravity CLI Window Title (Linux / macOS Native Engine)
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
echo "   🚀 Antigravity CLI Window Title Installer (Native Go)  "
echo "=========================================================="

HOME_DIR="${HOME:-/root}"
TARGET_DIR="${HOME_DIR}/.gemini/antigravity-cli-title"
mkdir -p "$TARGET_DIR"

REPO_BASE_URL="https://raw.githubusercontent.com/kjqg-cn/antigravity-cli-title/main"
RELEASE_BASE_URL="https://github.com/kjqg-cn/antigravity-cli-title/releases/latest/download"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" 2>/dev/null && pwd || echo "")"

DEST_EXE="${TARGET_DIR}/title"
DEST_UNINSTALL="${TARGET_DIR}/uninstall.sh"

# 1. Install uninstaller
SOURCE_UNINSTALL="${SCRIPT_DIR}/uninstall.sh"
if [[ -n "$SCRIPT_DIR" && -f "$SOURCE_UNINSTALL" ]]; then
    cp "$SOURCE_UNINSTALL" "$DEST_UNINSTALL"
    echo "[+] Installed uninstaller to: $DEST_UNINSTALL"
else
    echo "[*] Fetching uninstaller from GitHub..."
    curl -fsSL "${REPO_BASE_URL}/uninstall.sh" -o "$DEST_UNINSTALL" 2>/dev/null || true
    echo "[+] Downloaded uninstaller to: $DEST_UNINSTALL"
fi
chmod +x "$DEST_UNINSTALL" 2>/dev/null || true

# 2. Install Native Fast Execution Engine (title)
HAS_NATIVE=false

# Check local precompiled binary in repo
if [[ -n "$SCRIPT_DIR" ]]; then
    OS_NAME=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH_NAME=$(uname -m)
    case "$ARCH_NAME" in
        x86_64) GO_ARCH="amd64" ;;
        aarch64|arm64) GO_ARCH="arm64" ;;
        *) GO_ARCH="amd64" ;;
    esac

    CANDIDATES=(
        "${SCRIPT_DIR}/title"
        "${SCRIPT_DIR}/title_${OS_NAME}"
        "${SCRIPT_DIR}/title_${OS_NAME}_${GO_ARCH}"
    )
    for c in "${CANDIDATES[@]}"; do
        if [[ -f "$c" ]]; then
            cp "$c" "$DEST_EXE"
            chmod +x "$DEST_EXE"
            echo "[+] Installed native fast execution engine from local binary: $DEST_EXE"
            HAS_NATIVE=true
            break
        fi
    done

    # Try local go build
    if [[ "$HAS_NATIVE" == false ]] && command -v go >/dev/null 2>&1 && [[ -d "${SCRIPT_DIR}/cmd/title" ]]; then
        echo "[*] Compiling native execution engine (title) via local Go..."
        if (cd "${SCRIPT_DIR}" && go build -ldflags="-s -w" -o "$DEST_EXE" ./cmd/title); then
            chmod +x "$DEST_EXE"
            echo "[+] Compiled native execution engine: $DEST_EXE"
            HAS_NATIVE=true
        fi
    fi
fi

# Try downloading prebuilt binary from GitHub Releases
if [[ "$HAS_NATIVE" == false ]]; then
    OS_NAME=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH_NAME=$(uname -m)
    case "$ARCH_NAME" in
        x86_64) GO_ARCH="amd64" ;;
        aarch64|arm64) GO_ARCH="arm64" ;;
        *) GO_ARCH="amd64" ;;
    esac

    BINARY_NAME="title_${OS_NAME}_${GO_ARCH}"
    DOWNLOAD_URL="${RELEASE_BASE_URL}/${BINARY_NAME}"
    echo "[*] Downloading prebuilt native execution engine (${BINARY_NAME})..."

    if curl -fsSL "$DOWNLOAD_URL" -o "$DEST_EXE" 2>/dev/null && [[ -s "$DEST_EXE" ]]; then
        chmod +x "$DEST_EXE"
        echo "[+] Downloaded native execution engine: $DEST_EXE"
        HAS_NATIVE=true
    else
        # Fallback to generic release name if architecture specific isn't found
        if curl -fsSL "${RELEASE_BASE_URL}/title_${OS_NAME}" -o "$DEST_EXE" 2>/dev/null && [[ -s "$DEST_EXE" ]]; then
            chmod +x "$DEST_EXE"
            echo "[+] Downloaded native execution engine: $DEST_EXE"
            HAS_NATIVE=true
        fi
    fi
fi

if [[ "$HAS_NATIVE" == false ]]; then
    echo "[-] Failed to obtain native binary 'title'."
    echo "    Please install Go (https://go.dev) or compile manually: go build -o title ./cmd/title"
    exit 1
fi

# 3. Build arguments
ARGS=""
[[ "$SHOW_PREFIX" == false ]] && ARGS="${ARGS} --no-prefix"
[[ "$USE_TEXT" == true ]] && ARGS="${ARGS} --text"
[[ "$CUSTOM_PREFIX" != "agy:" ]] && ARGS="${ARGS} --prefix \"$CUSTOM_PREFIX\""
[[ "$MAX_LEN" != 35 ]] && ARGS="${ARGS} --max-length $MAX_LEN"

TITLE_CMD="${DEST_EXE}${ARGS}"

# 4. Update settings.json
SETTINGS_DIR="${HOME_DIR}/.gemini/antigravity-cli"
SETTINGS_PATH="${SETTINGS_DIR}/settings.json"
mkdir -p "$SETTINGS_DIR"

if [[ -f "$SETTINGS_PATH" ]]; then
    cp "$SETTINGS_PATH" "${SETTINGS_PATH}.bak"
    echo "[+] Backup saved to: ${SETTINGS_PATH}.bak"
fi

if command -v python3 >/dev/null 2>&1; then
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
else
    # Minimal fallback without python3
    cat <<EOF > "$SETTINGS_PATH"
{
  "title": {
    "type": "command",
    "command": "$TITLE_CMD",
    "enabled": true
  }
}
EOF
fi

echo "[+] Successfully configured 'title' in: $SETTINGS_PATH"
echo ""
echo "✨ Installation complete!"
echo "   Command configured: $TITLE_CMD"
echo "   Installed directory: $TARGET_DIR"
echo "   Engine: Native Compiled Executable (< 3ms latency)"
echo "   To uninstall later, run:"
echo "     ${DEST_UNINSTALL}"
echo ""
echo "   ⚪ Ready     -> Initial state / re-entering history"
echo "   ⚙️  Working   -> Thinking / background task execution"
echo "   ⚠️  Waiting   -> Prompting user (ask_question) or permission confirmation"
echo "   ✅ Done      -> Turn completed; remains done until new input"
echo "=========================================================="
