#!/usr/bin/env bash
# install.sh - Native Installer for Antigravity CLI Window Title (Linux / macOS Native Engine)
set -euo pipefail

ANIMATE=false
ANIMATE_SET=false
SHOW_PREFIX=true
USE_TEXT=false
CUSTOM_PREFIX="agy:"
MAX_LEN=35
TASK_TIMEOUT=60
READY_SYMBOL=""
WORKING_SYMBOL=""
WAITING_SYMBOL=""
DONE_SYMBOL=""

SHOW_PREFIX_SET=false
USE_TEXT_SET=false
CUSTOM_PREFIX_SET=false
MAX_LEN_SET=false
TASK_TIMEOUT_SET=false
READY_SYMBOL_SET=false
WORKING_SYMBOL_SET=false
WAITING_SYMBOL_SET=false
DONE_SYMBOL_SET=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --animate|--animate=true) ANIMATE=true; ANIMATE_SET=true; shift ;;
        --no-animate|--animate=false) ANIMATE=false; ANIMATE_SET=true; shift ;;
        --no-prefix) SHOW_PREFIX=false; SHOW_PREFIX_SET=true; shift ;;
        --no-prefix=true) SHOW_PREFIX=false; SHOW_PREFIX_SET=true; shift ;;
        --no-prefix=false|--show-prefix|--with-prefix) SHOW_PREFIX=true; SHOW_PREFIX_SET=true; shift ;;
        --text|--classic) USE_TEXT=true; USE_TEXT_SET=true; shift ;;
        --text=true|--classic=true) USE_TEXT=true; USE_TEXT_SET=true; shift ;;
        --no-text|--emoji|--text=false|--classic=false) USE_TEXT=false; USE_TEXT_SET=true; shift ;;
        --max-length=*) MAX_LEN="${1#*=}"; MAX_LEN_SET=true; shift ;;
        --max-length)
            if [[ $# -lt 2 ]]; then
                echo "[-] Error: $1 requires a value" >&2
                exit 1
            fi
            MAX_LEN="$2"; MAX_LEN_SET=true; shift 2 ;;
        --prefix=*) CUSTOM_PREFIX="${1#*=}"; CUSTOM_PREFIX_SET=true; shift ;;
        --prefix)
            if [[ $# -lt 2 ]]; then
                echo "[-] Error: $1 requires a value" >&2
                exit 1
            fi
            CUSTOM_PREFIX="$2"; CUSTOM_PREFIX_SET=true; shift 2 ;;
        --task-timeout=*) TASK_TIMEOUT="${1#*=}"; TASK_TIMEOUT_SET=true; shift ;;
        --task-timeout)
            if [[ $# -lt 2 ]]; then
                echo "[-] Error: $1 requires a value" >&2
                exit 1
            fi
            TASK_TIMEOUT="$2"; TASK_TIMEOUT_SET=true; shift 2 ;;
        --ready=*|--ready-symbol=*) READY_SYMBOL="${1#*=}"; READY_SYMBOL_SET=true; shift ;;
        --ready|--ready-symbol)
            if [[ $# -lt 2 ]]; then
                echo "[-] Error: $1 requires a value" >&2
                exit 1
            fi
            READY_SYMBOL="$2"; READY_SYMBOL_SET=true; shift 2 ;;
        --working=*|--working-symbol=*) WORKING_SYMBOL="${1#*=}"; WORKING_SYMBOL_SET=true; shift ;;
        --working|--working-symbol)
            if [[ $# -lt 2 ]]; then
                echo "[-] Error: $1 requires a value" >&2
                exit 1
            fi
            WORKING_SYMBOL="$2"; WORKING_SYMBOL_SET=true; shift 2 ;;
        --waiting=*|--waiting-symbol=*) WAITING_SYMBOL="${1#*=}"; WAITING_SYMBOL_SET=true; shift ;;
        --waiting|--waiting-symbol)
            if [[ $# -lt 2 ]]; then
                echo "[-] Error: $1 requires a value" >&2
                exit 1
            fi
            WAITING_SYMBOL="$2"; WAITING_SYMBOL_SET=true; shift 2 ;;
        --done=*|--done-symbol=*) DONE_SYMBOL="${1#*=}"; DONE_SYMBOL_SET=true; shift ;;
        --done|--done-symbol)
            if [[ $# -lt 2 ]]; then
                echo "[-] Error: $1 requires a value" >&2
                exit 1
            fi
            DONE_SYMBOL="$2"; DONE_SYMBOL_SET=true; shift 2 ;;
        -h|--help) echo "Usage: install.sh [--animate|--no-animate] [--no-prefix|--show-prefix] [--text] [--prefix VALUE] [--max-length N] [--task-timeout N] [--ready VALUE] [--working VALUE] [--waiting VALUE] [--done VALUE]"; exit 0 ;;
        *) echo "Unknown option: $1" >&2; exit 1 ;;
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
IS_REMOTE_PIPELINE=false
if [[ -z "${BASH_SOURCE[0]:-}" || ! -f "${BASH_SOURCE[0]:-}" ]]; then
    IS_REMOTE_PIPELINE=true
    SCRIPT_DIR=""
else
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" 2>/dev/null && pwd || echo "")"
    if [[ -z "$SCRIPT_DIR" ]]; then
        IS_REMOTE_PIPELINE=true
    fi
fi

DEST_EXE="${TARGET_DIR}/title"
DEST_UNINSTALL="${TARGET_DIR}/uninstall.sh"

# 1. Install uninstaller
TMP_UNINSTALL="${DEST_UNINSTALL}.tmp.$$"
if [[ "$IS_REMOTE_PIPELINE" == false && -n "$SCRIPT_DIR" && -f "${SCRIPT_DIR}/uninstall.sh" ]]; then
    cp "${SCRIPT_DIR}/uninstall.sh" "$TMP_UNINSTALL"
    chmod +x "$TMP_UNINSTALL" 2>/dev/null || true
    mv -f "$TMP_UNINSTALL" "$DEST_UNINSTALL"
    echo "[+] Installed uninstaller to: $DEST_UNINSTALL"
else
    echo "[*] Fetching uninstaller from GitHub..."
    if curl -fsSL "${REPO_BASE_URL}/uninstall.sh" -o "$TMP_UNINSTALL" 2>/dev/null && [[ -s "$TMP_UNINSTALL" ]]; then
        chmod +x "$TMP_UNINSTALL" 2>/dev/null || true
        mv -f "$TMP_UNINSTALL" "$DEST_UNINSTALL"
        echo "[+] Downloaded uninstaller to: $DEST_UNINSTALL"
    else
        rm -f "$TMP_UNINSTALL"
        echo "[-] Warning: Failed to download uninstaller from GitHub."
    fi
fi
chmod +x "$DEST_UNINSTALL" 2>/dev/null || true

# 2. Install Native Fast Execution Engine (title)
HAS_NATIVE=false
TMP_EXE="${DEST_EXE}.tmp.$$"

# Check local precompiled binary in repo
if [[ "$IS_REMOTE_PIPELINE" == false && -n "$SCRIPT_DIR" ]]; then
    OS_NAME=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH_NAME=$(uname -m)
    case "$ARCH_NAME" in
        x86_64) GO_ARCH="amd64" ;;
        aarch64|arm64) GO_ARCH="arm64" ;;
        *) echo "Unsupported architecture: $ARCH_NAME" >&2; exit 1 ;;
    esac

    # Try local go build first to ensure matching current architecture and fresh code
    if command -v go >/dev/null 2>&1 && [[ -d "${SCRIPT_DIR}/cmd/title" ]]; then
        echo "[*] Compiling native execution engine (title) via local Go..."
        if (cd "${SCRIPT_DIR}" && go build -ldflags="-s -w" -o "$TMP_EXE" ./cmd/title); then
            chmod +x "$TMP_EXE"
            mv -f "$TMP_EXE" "$DEST_EXE"
            echo "[+] Compiled native execution engine: $DEST_EXE"
            HAS_NATIVE=true
        else
            rm -f "$TMP_EXE"
        fi
    fi

    # Fallback to local prebuilt binary (architecture-specific preferred)
    if [[ "$HAS_NATIVE" == false ]]; then
        CANDIDATES=(
            "${SCRIPT_DIR}/title_${OS_NAME}_${GO_ARCH}"
            "${SCRIPT_DIR}/title_${OS_NAME}"
            "${SCRIPT_DIR}/title"
        )
        for c in "${CANDIDATES[@]}"; do
            if [[ -f "$c" ]]; then
                cp "$c" "$TMP_EXE"
                chmod +x "$TMP_EXE"
                mv -f "$TMP_EXE" "$DEST_EXE"
                echo "[+] Installed native fast execution engine from local binary: $DEST_EXE"
                HAS_NATIVE=true
                break
            fi
        done
        rm -f "$TMP_EXE"
    fi
fi

# Try downloading prebuilt binary from GitHub Releases
if [[ "$HAS_NATIVE" == false ]]; then
    OS_NAME=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH_NAME=$(uname -m)
    case "$ARCH_NAME" in
        x86_64) GO_ARCH="amd64" ;;
        aarch64|arm64) GO_ARCH="arm64" ;;
        *) echo "Unsupported architecture: $ARCH_NAME" >&2; exit 1 ;;
    esac

    BINARY_NAME="title_${OS_NAME}_${GO_ARCH}"
    DOWNLOAD_URL="${RELEASE_BASE_URL}/${BINARY_NAME}"
    echo "[*] Downloading prebuilt native execution engine (${BINARY_NAME})..."

    if curl -fsSL "$DOWNLOAD_URL" -o "$TMP_EXE" 2>/dev/null && [[ -s "$TMP_EXE" ]]; then
        chmod +x "$TMP_EXE"
        mv -f "$TMP_EXE" "$DEST_EXE"
        echo "[+] Downloaded native execution engine: $DEST_EXE"
        HAS_NATIVE=true
    else
        rm -f "$TMP_EXE"
        # Fallback to generic release name if architecture specific isn't found
        if curl -fsSL "${RELEASE_BASE_URL}/title_${OS_NAME}" -o "$TMP_EXE" 2>/dev/null && [[ -s "$TMP_EXE" ]]; then
            chmod +x "$TMP_EXE"
            mv -f "$TMP_EXE" "$DEST_EXE"
            echo "[+] Downloaded native execution engine: $DEST_EXE"
            HAS_NATIVE=true
        else
            rm -f "$TMP_EXE"
        fi
    fi
fi

if [[ "$HAS_NATIVE" == false ]]; then
    echo "[-] Failed to obtain native binary 'title'."
    echo "    Please install Go (https://go.dev) or compile manually: go build -o title ./cmd/title"
    exit 1
fi

clean_symbol() {
    local val="$1"
    val="${val#\"}"
    val="${val%\"}"
    val="${val#\'}"
    val="${val%\'}"
    echo "$val"
}

# Tokenize without eval: quoted values can contain text that looks like a flag.
command_has_flag() {
    local command="$1" wanted="$2" token="" quote="" char next has_token=false enabled=false
    local -a tokens=()
    local i
    for ((i=0; i<${#command}; i++)); do
        char="${command:$i:1}"
        if [[ -n "$quote" ]]; then
            next="${command:$((i+1)):1}"
            if [[ "$char" == '\' && ( "$next" == "$quote" || "$next" == '\' ) ]]; then
                token+="$next"; i=$((i+1))
            elif [[ "$char" == "$quote" ]]; then quote=""
            else token+="$char"
            fi
        elif [[ "$char" == '"' || "$char" == "'" ]]; then
            quote="$char"; has_token=true
        elif [[ "$char" == ' ' || "$char" == $'\t' || "$char" == $'\n' || "$char" == $'\r' ]]; then
            if [[ "$has_token" == true ]]; then tokens+=("$token"); token=""; has_token=false; fi
        else
            token+="$char"; has_token=true
        fi
    done
    if [[ "$has_token" == true ]]; then tokens+=("$token"); fi
    # Start after the executable; values must never be interpreted as flags.
    for ((i=1; i<${#tokens[@]}; i++)); do
        case "${tokens[$i]}" in
            --prefix|--max-length|--task-timeout|--ready|--ready-symbol|--working|--working-symbol|--waiting|--waiting-symbol|--done|--done-symbol) i=$((i+1)) ;;
            --animate|--animate=true) if [[ "$wanted" == animate ]]; then enabled=true; fi ;;
            --no-animate|--animate=false) if [[ "$wanted" == animate ]]; then enabled=false; fi ;;
            --no-prefix) if [[ "$wanted" == no-prefix ]]; then enabled=true; fi ;;
            --text|--classic) if [[ "$wanted" == text ]]; then enabled=true; fi ;;
        esac
    done
    [[ "$enabled" == true ]]
}

command_has_animate() { command_has_flag "$1" animate; }

SETTINGS_DIR="${HOME_DIR}/.gemini/antigravity-cli"
SETTINGS_PATH="${SETTINGS_DIR}/settings.json"
BAK_PATH="${SETTINGS_DIR}/settings.json.pre-title.bak"
mkdir -p "$SETTINGS_DIR"

EXISTING_CMD=""
if [[ -f "$SETTINGS_PATH" ]]; then
    if python3 -c 'import json' >/dev/null 2>&1; then
        EXISTING_CMD=$(python3 -c "import sys, json; data=json.load(open(sys.argv[1], encoding='utf-8-sig')); print(data.get('title',{}).get('command',''))" "$SETTINGS_PATH" 2>/dev/null || true)
    elif python -c 'import json' >/dev/null 2>&1; then
        EXISTING_CMD=$(python -c "import sys, json; data=json.load(open(sys.argv[1], encoding='utf-8-sig')); print(data.get('title',{}).get('command',''))" "$SETTINGS_PATH" 2>/dev/null || true)
    elif command -v jq >/dev/null 2>&1; then
        EXISTING_CMD=$(jq -r '.title.command // empty' "$SETTINGS_PATH" 2>/dev/null || true)
    fi
fi

if [[ -n "$EXISTING_CMD" && "$EXISTING_CMD" == *"antigravity-cli-title"* ]]; then
    if [[ "$SHOW_PREFIX_SET" == false ]] && command_has_flag "$EXISTING_CMD" no-prefix; then
        SHOW_PREFIX=false
    fi
    if [[ "$USE_TEXT_SET" == false ]] && command_has_flag "$EXISTING_CMD" text; then
        USE_TEXT=true
    fi
    if [[ "$ANIMATE_SET" == false ]] && command_has_animate "$EXISTING_CMD"; then ANIMATE=true; fi
    PY_BIN=""
    if python3 -c 'import json' >/dev/null 2>&1; then
        PY_BIN="python3"
    elif python -c 'import json' >/dev/null 2>&1; then
        PY_BIN="python"
    fi

    EXISTING_PREFIX=""
    EXISTING_MAX=""
    EXISTING_TIMEOUT=""
    EXISTING_READY=""
    EXISTING_WORKING=""
    EXISTING_WAITING=""
    EXISTING_DONE=""

    if [[ -n "$PY_BIN" ]]; then
        eval "$($PY_BIN - "$EXISTING_CMD" 2>/dev/null << 'EOF' || true
import sys, shlex
cmd = sys.argv[1]
try:
    tokens = shlex.split(cmd)
except Exception:
    tokens = cmd.split()
i = 0
flags = {
    "--prefix": "EXISTING_PREFIX",
    "--max-length": "EXISTING_MAX",
    "--task-timeout": "EXISTING_TIMEOUT",
    "--ready": "EXISTING_READY",
    "--ready-symbol": "EXISTING_READY",
    "--working": "EXISTING_WORKING",
    "--working-symbol": "EXISTING_WORKING",
    "--waiting": "EXISTING_WAITING",
    "--waiting-symbol": "EXISTING_WAITING",
    "--done": "EXISTING_DONE",
    "--done-symbol": "EXISTING_DONE"
}
while i < len(tokens):
    tok = tokens[i]
    matched = False
    for flag, varname in flags.items():
        if tok == flag and i + 1 < len(tokens):
            val = tokens[i+1]
            print(f"{varname}={shlex.quote(val)}")
            i += 1
            matched = True
            break
        elif tok.startswith(flag + "="):
            val = tok[len(flag)+1:]
            print(f"{varname}={shlex.quote(val)}")
            matched = True
            break
    i += 1
EOF
)"
    else
        if echo "$EXISTING_CMD" | grep -q -- '--prefix'; then
            EXISTING_PREFIX=$(echo "$EXISTING_CMD" | sed -E -n 's/.*--prefix[ =]+("([^"]*)"|'\''([^'\'']*)'\''|([^ ]+)).*/\2\3\4/p')
        fi
        if echo "$EXISTING_CMD" | grep -q -- '--max-length'; then
            EXISTING_MAX=$(echo "$EXISTING_CMD" | sed -E -n 's/.*--max-length[ =]+([0-9]+).*/\1/p')
        fi
        if echo "$EXISTING_CMD" | grep -q -- '--task-timeout'; then
            EXISTING_TIMEOUT=$(echo "$EXISTING_CMD" | sed -E -n 's/.*--task-timeout[ =]+([0-9]+).*/\1/p')
        fi
        if echo "$EXISTING_CMD" | grep -qE -- '(--ready|--ready-symbol)'; then
            EXISTING_READY=$(echo "$EXISTING_CMD" | sed -E -n 's/.*--ready(-symbol)?[ =]+("([^"]*)"|'\''([^'\'']*)'\''|([^ ]+)).*/\3\4\5/p')
        fi
        if echo "$EXISTING_CMD" | grep -qE -- '(--working|--working-symbol)'; then
            EXISTING_WORKING=$(echo "$EXISTING_CMD" | sed -E -n 's/.*--working(-symbol)?[ =]+("([^"]*)"|'\''([^'\'']*)'\''|([^ ]+)).*/\3\4\5/p')
        fi
        if echo "$EXISTING_CMD" | grep -qE -- '(--waiting|--waiting-symbol)'; then
            EXISTING_WAITING=$(echo "$EXISTING_CMD" | sed -E -n 's/.*--waiting(-symbol)?[ =]+("([^"]*)"|'\''([^'\'']*)'\''|([^ ]+)).*/\3\4\5/p')
        fi
        if echo "$EXISTING_CMD" | grep -qE -- '(--done|--done-symbol)'; then
            EXISTING_DONE=$(echo "$EXISTING_CMD" | sed -E -n 's/.*--done(-symbol)?[ =]+("([^"]*)"|'\''([^'\'']*)'\''|([^ ]+)).*/\3\4\5/p')
        fi
    fi

    [[ "$CUSTOM_PREFIX_SET" == false && -n "$EXISTING_PREFIX" ]] && CUSTOM_PREFIX="$EXISTING_PREFIX"
    [[ "$MAX_LEN_SET" == false && -n "$EXISTING_MAX" ]] && MAX_LEN="$EXISTING_MAX"
    [[ "$TASK_TIMEOUT_SET" == false && -n "$EXISTING_TIMEOUT" ]] && TASK_TIMEOUT="$EXISTING_TIMEOUT"
    [[ "$READY_SYMBOL_SET" == false && -n "$EXISTING_READY" ]] && READY_SYMBOL="$EXISTING_READY"
    [[ "$WORKING_SYMBOL_SET" == false && -n "$EXISTING_WORKING" ]] && WORKING_SYMBOL="$EXISTING_WORKING"
    [[ "$WAITING_SYMBOL_SET" == false && -n "$EXISTING_WAITING" ]] && WAITING_SYMBOL="$EXISTING_WAITING"
    [[ "$DONE_SYMBOL_SET" == false && -n "$EXISTING_DONE" ]] && DONE_SYMBOL="$EXISTING_DONE"
fi

READY_SYMBOL=$(clean_symbol "$READY_SYMBOL")
WORKING_SYMBOL=$(clean_symbol "$WORKING_SYMBOL")
WAITING_SYMBOL=$(clean_symbol "$WAITING_SYMBOL")
DONE_SYMBOL=$(clean_symbol "$DONE_SYMBOL")

# 3. Build arguments
ARGS=""
[[ "$ANIMATE" == true ]] && ARGS="${ARGS} --animate"
[[ "$SHOW_PREFIX" == false ]] && ARGS="${ARGS} --no-prefix"
[[ "$USE_TEXT" == true ]] && ARGS="${ARGS} --text"
if [[ "$CUSTOM_PREFIX" != "agy:" ]]; then
    CLEAN_PREFIX=$(clean_symbol "$CUSTOM_PREFIX")
    CLEAN_PREFIX="${CLEAN_PREFIX//\\/\\\\}"
    CLEAN_PREFIX="${CLEAN_PREFIX//\"/\\\"}"
    ARGS="${ARGS} --prefix \"$CLEAN_PREFIX\""
fi
[[ "$MAX_LEN" != 35 ]] && ARGS="${ARGS} --max-length $MAX_LEN"
[[ "$TASK_TIMEOUT" != 60 ]] && ARGS="${ARGS} --task-timeout $TASK_TIMEOUT"

append_symbol_arg() {
    local flag="$1"
    local val="$2"
    [[ -z "$val" ]] && return
    val="${val//\\/\\\\}"
    val="${val//\"/\\\"}"
    ARGS="${ARGS} ${flag} \"${val}\""
}

append_symbol_arg "--ready" "$READY_SYMBOL"
append_symbol_arg "--working" "$WORKING_SYMBOL"
append_symbol_arg "--waiting" "$WAITING_SYMBOL"
append_symbol_arg "--done" "$DONE_SYMBOL"

if [[ "${DEST_EXE}" =~ [[:space:]] ]]; then
    TITLE_CMD="\"${DEST_EXE}\"${ARGS}"
else
    TITLE_CMD="${DEST_EXE}${ARGS}"
fi

PYTHON_BIN=""
if python3 -c 'import json' >/dev/null 2>&1; then
    PYTHON_BIN="python3"
elif python -c 'import json' >/dev/null 2>&1; then
    PYTHON_BIN="python"
fi

# Pre-validate settings.json before backup or modification
if [[ -f "$SETTINGS_PATH" && -s "$SETTINGS_PATH" ]]; then
    if [[ -n "$PYTHON_BIN" ]]; then
        $PYTHON_BIN - "$SETTINGS_PATH" << 'EOF' || exit 1
import sys, json
path = sys.argv[1]
try:
    with open(path, 'r', encoding='utf-8-sig') as f:
        c = f.read().strip()
        if c:
            data = json.loads(c)
            if not isinstance(data, dict):
                print(f"[-] Root element in {path} is not an object", file=sys.stderr)
                sys.exit(1)
except Exception as e:
    print(f"[-] Failed to parse existing {path}: {e}", file=sys.stderr)
    sys.exit(1)
EOF
    elif command -v jq >/dev/null 2>&1; then
        if ! jq -e 'if type == "object" then true else false end' "$SETTINGS_PATH" >/dev/null 2>&1; then
            echo "[-] Error: Root element in $SETTINGS_PATH is not a valid JSON object" >&2
            exit 1
        fi
    fi
fi

# 4. Update settings.json
if [[ -f "$SETTINGS_PATH" ]] && [[ ! -f "$BAK_PATH" ]]; then
    if ! grep -q 'antigravity-cli-title' "$SETTINGS_PATH" 2>/dev/null; then
        cp "$SETTINGS_PATH" "$BAK_PATH"
        echo "[+] Initial backup saved to: $BAK_PATH"
    fi
fi

if [[ -f "$SETTINGS_PATH" ]]; then
    TS=$(date +%Y%m%d%H%M%S)
    INC_BAK="${SETTINGS_DIR}/settings.json.${TS}.bak"
    cp "$SETTINGS_PATH" "$INC_BAK"
    echo "[+] Incremental backup saved to: $INC_BAK"
    # Keep at most 5 latest timestamped backups (portable across Linux and macOS)
    BAK_FILES=()
    for backup in "${SETTINGS_DIR}"/settings.json.*.bak; do
        if [[ "${backup##*/}" =~ ^settings\.json\.[0-9]{14}\.bak$ ]]; then BAK_FILES+=("$backup"); fi
    done
    NUM_BAKS=${#BAK_FILES[@]}
    if [[ $NUM_BAKS -gt 5 ]]; then
        EXCESS=$((NUM_BAKS - 5))
        for ((i=0; i<EXCESS; i++)); do
            rm -f "${BAK_FILES[$i]}"
        done
    fi
fi

ESCAPED_TITLE_CMD=$(printf '%s' "$TITLE_CMD" | sed 's/\\/\\\\/g; s/"/\\"/g')

if [[ -n "$PYTHON_BIN" ]]; then
    $PYTHON_BIN - "$SETTINGS_PATH" "$TITLE_CMD" << 'EOF'
import sys, json, os

settings_path = sys.argv[1]
title_cmd = sys.argv[2]

data = {}
if os.path.exists(settings_path):
    try:
        with open(settings_path, 'r', encoding='utf-8-sig') as f:
            content = f.read().strip()
            if content:
                data = json.loads(content)
            else:
                data = {}
    except Exception as e:
        print(f"[-] Failed to parse existing {settings_path}: {e}", file=sys.stderr)
        sys.exit(1)

    if not isinstance(data, dict):
        print(f"[-] Root element in {settings_path} is not an object", file=sys.stderr)
        sys.exit(1)

data['title'] = {
    'type': 'command',
    'command': title_cmd,
    'enabled': True
}

import tempfile
fd, temp_path = tempfile.mkstemp(prefix='settings.', suffix='.tmp', dir=os.path.dirname(settings_path))
try:
    with os.fdopen(fd, 'w', encoding='utf-8') as f:
        json.dump(data, f, indent=2, ensure_ascii=False)
    os.replace(temp_path, settings_path)
finally:
    if os.path.exists(temp_path):
        os.unlink(temp_path)
EOF
elif command -v jq >/dev/null 2>&1; then
    has_content=false
    if [[ -f "$SETTINGS_PATH" && -s "$SETTINGS_PATH" ]]; then
        if grep -q '[^[:space:]]' "$SETTINGS_PATH" 2>/dev/null; then
            has_content=true
        fi
    fi

    if [[ "$has_content" == true ]]; then
        if jq -e 'if type == "object" then .title = {"type": "command", "command": $cmd, "enabled": true} else error("Root element is not an object") end' --arg cmd "$TITLE_CMD" "$SETTINGS_PATH" > "${SETTINGS_PATH}.tmp" 2>/dev/null; then
            mv "${SETTINGS_PATH}.tmp" "$SETTINGS_PATH"
        else
            rm -f "${SETTINGS_PATH}.tmp"
            echo "[-] Failed to safely update ${SETTINGS_PATH} with jq. Please ensure it is valid JSON with an object root." >&2
            exit 1
        fi
    else
        cat <<EOF > "$SETTINGS_PATH"
{
  "title": {
    "type": "command",
    "command": "$ESCAPED_TITLE_CMD",
    "enabled": true
  }
}
EOF
    fi
else
    # Neither Python nor jq is available
    if [[ -f "$SETTINGS_PATH" && -s "$SETTINGS_PATH" ]]; then
        echo "[-] Neither Python nor jq is available to safely merge configuration into '${SETTINGS_PATH}'."
        echo "    To prevent overwriting your existing settings, please manually add the 'title' block to '${SETTINGS_PATH}':"
        echo "    \"title\": {"
        echo "      \"type\": \"command\","
        echo "      \"command\": \"${ESCAPED_TITLE_CMD}\","
        echo "      \"enabled\": true"
        echo "    }"
        exit 1
    else
        cat <<EOF > "$SETTINGS_PATH"
{
  "title": {
    "type": "command",
    "command": "$ESCAPED_TITLE_CMD",
    "enabled": true
  }
}
EOF
    fi
fi

echo "[+] Successfully configured 'title' in: $SETTINGS_PATH"
echo ""
echo "✨ Installation complete!"
echo "   Command configured: $TITLE_CMD"
echo "   Installed directory: $TARGET_DIR"
echo "   Engine: Native Compiled Executable"
echo "   To uninstall later, run:"
echo "     ${DEST_UNINSTALL}"
echo ""
echo "   ⚪ Ready     -> Initial state / re-entering history"
echo "   ⚙️  Working   -> Thinking / background task execution"
echo "   ⚠️  Waiting   -> Prompting user (ask_question) or permission confirmation"
echo "   ✅ Done      -> Turn completed; remains done until new input"
echo "=========================================================="
