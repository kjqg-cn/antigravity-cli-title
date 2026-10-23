#!/usr/bin/env bash
# title.sh - Antigravity CLI Window Title Renderer for Linux / macOS
set -euo pipefail

SHOW_PREFIX=true
CUSTOM_PREFIX="agy:"
USE_TEXT_STATUS=false
MAX_TITLE_LEN=35

while [[ $# -gt 0 ]]; do
    case "$1" in
        --no-prefix) SHOW_PREFIX=false; shift ;;
        --text|--classic) USE_TEXT_STATUS=true; shift ;;
        --max-length) MAX_TITLE_LEN="$2"; shift 2 ;;
        --prefix) CUSTOM_PREFIX="$2"; SHOW_PREFIX=true; shift 2 ;;
        *) shift ;;
    esac
done

INPUT_JSON=$(cat)
if [[ -z "${INPUT_JSON// /}" ]]; then
    CWD_NAME=$(basename "$PWD")
    READY_IND="⚪"
    [[ "$USE_TEXT_STATUS" == true ]] && READY_IND="[READY]"
    [[ "$SHOW_PREFIX" == true ]] && echo "${CUSTOM_PREFIX} ${READY_IND} ${CWD_NAME}" || echo "${READY_IND} ${CWD_NAME}"
    exit 0
fi

# Extract JSON fields via python3 or jq
eval $(echo "$INPUT_JSON" | python3 -c '
import json, sys, os, sqlite3
try:
    d = json.loads(sys.stdin.read())
    state = d.get("agent_state", "idle")
    cid = d.get("conversation_id", "")
    title = d.get("conversation_title", "")
    cwd = d.get("cwd", "")

    if not title and cid:
        for p in [
            os.path.expanduser("~/.gemini/antigravity-cli/conversation_summaries.db"),
            os.path.expanduser("~/.gemini/antigravity/conversation_summaries.db")
        ]:
            if os.path.exists(p):
                try:
                    con = sqlite3.connect(f"file:{p}?mode=ro", uri=True)
                    cur = con.cursor()
                    cur.execute("SELECT title FROM conversation_summaries WHERE conversation_id=? LIMIT 1", (cid,))
                    row = cur.fetchone()
                    if row and row[0]:
                        title = row[0]
                    con.close()
                    if title:
                        break
                except Exception:
                    pass

    subagents = d.get("subagents", [])
    active_sub = sum(1 for s in subagents if isinstance(s, dict) and s.get("status") not in ["completed", "failed", "killed", "error", "cancelled"])
    task_count = int(d.get("task_count", 0) or 0)
    has_bg = "true" if (active_sub > 0 or task_count > 0) else "false"

    safe_title = title.replace("\"", "\\\"").replace("$", "\\$").replace("`", "\\`")
    safe_cwd = cwd.replace("\"", "\\\"").replace("$", "\\$").replace("`", "\\`")
    print(f"STATE=\"{state}\"")
    print(f"CONV_ID=\"{cid}\"")
    print(f"CONV_TITLE=\"{safe_title}\"")
    print(f"CWD_DIR=\"{safe_cwd}\"")
    print(f"HAS_BG=\"{has_bg}\"")
except Exception:
    print("STATE=\"idle\" CONV_ID=\"\" CONV_TITLE=\"\" CWD_DIR=\"\" HAS_BG=\"false\"")
' 2>/dev/null || echo 'STATE="idle" CONV_ID="" CONV_TITLE="" CWD_DIR="" HAS_BG="false"')

HOME_DIR="${HOME:-/root}"
TRANSCRIPT_PATH="${HOME_DIR}/.gemini/antigravity-cli/brain/${CONV_ID}/.system_generated/logs/transcript.jsonl"
if [[ ! -f "$TRANSCRIPT_PATH" ]]; then
    ALT_PATH="${HOME_DIR}/.gemini/antigravity/brain/${CONV_ID}/.system_generated/logs/transcript.jsonl"
    [[ -f "$ALT_PATH" ]] && TRANSCRIPT_PATH="$ALT_PATH"
fi
TARGET_DIR="${HOME_DIR}/.gemini/antigravity-cli-title"
mkdir -p "$TARGET_DIR"
MARKER_FILE=""
[[ -n "$CONV_ID" ]] && MARKER_FILE="${TARGET_DIR}/marker_${CONV_ID}.txt"

# Title extraction
HAS_HISTORY=false
TITLE="${CONV_TITLE:-}"
if [[ -n "$TITLE" ]]; then
    HAS_HISTORY=true
fi

if [[ -f "$TRANSCRIPT_PATH" ]]; then
    FIRST_REQ=$(grep -m 1 '"type"[[:space:]]*:[[:space:]]*"USER_INPUT"' "$TRANSCRIPT_PATH" 2>/dev/null | python3 -c '
import json, sys, re
try:
    line = sys.stdin.read().strip()
    d = json.loads(line)
    c = d.get("content", "")
    m = re.search(r"<USER_REQUEST>(.*?)</USER_REQUEST>", c, re.DOTALL)
    text = m.group(1).strip() if m else c.strip()
    first = text.splitlines()[0].strip() if text else ""
    print(first)
except Exception:
    pass
' 2>/dev/null || echo "")

    if [[ -n "$FIRST_REQ" ]]; then
        HAS_HISTORY=true
        if [[ -z "$TITLE" ]]; then
            if [[ ${#FIRST_REQ} -gt $MAX_TITLE_LEN ]]; then
                TITLE="${FIRST_REQ:0:$((MAX_TITLE_LEN - 3))}..."
            else
                TITLE="$FIRST_REQ"
            fi
        fi
    fi
fi

if [[ -z "$TITLE" ]]; then
    if [[ -n "$CWD_DIR" ]]; then
        TITLE=$(basename "$CWD_DIR")
    else
        TITLE=$(basename "$PWD")
    fi
fi

# Locate parent agy PID
AGY_PID=""
CUR_PID=$$
for ((i=0; i<4; i++)); do
    P_NAME=$(ps -p "$CUR_PID" -o comm= 2>/dev/null || echo "")
    if [[ "$P_NAME" =~ agy ]]; then
        AGY_PID="$CUR_PID"
        break
    fi
    CUR_PID=$(ps -p "$CUR_PID" -o ppid= 2>/dev/null | tr -d ' ' || echo "")
    [[ -z "$CUR_PID" || "$CUR_PID" -eq 1 ]] && break
done
if [[ -z "$AGY_PID" ]]; then
    AGY_PID=$(pgrep -n agy 2>/dev/null || echo "$$")
fi

# Determine emoji
EMOJI="⚪"
[[ "$USE_TEXT_STATUS" == true ]] && EMOJI="[READY]"

case "$STATE" in
    thinking|working)
        EMOJI="⚙️"
        [[ "$USE_TEXT_STATUS" == true ]] && EMOJI="[WORKING]"
        [[ -n "$MARKER_FILE" ]] && echo "${AGY_PID}:RUNNING" > "$MARKER_FILE" 2>/dev/null || true
        ;;
    waiting|paused)
        EMOJI="⚠️"
        [[ "$USE_TEXT_STATUS" == true ]] && EMOJI="[WAITING]"
        [[ -n "$MARKER_FILE" ]] && echo "${AGY_PID}:RUNNING" > "$MARKER_FILE" 2>/dev/null || true
        ;;
    tool_use)
        EMOJI="⚙️"
        [[ "$USE_TEXT_STATUS" == true ]] && EMOJI="[WORKING]"
        [[ -n "$MARKER_FILE" ]] && echo "${AGY_PID}:RUNNING" > "$MARKER_FILE" 2>/dev/null || true

        # Check for ask_question or permissions.ask
        if [[ -f "$TRANSCRIPT_PATH" ]]; then
            IS_ASK=$(tail -n 200 "$TRANSCRIPT_PATH" | python3 -c '
import json, sys
lines = [l.strip() for l in sys.stdin if l.strip()]
last_resp = None
for l in reversed(lines):
    try:
        obj = json.loads(l)
        if obj.get("type") == "PLANNER_RESPONSE" and obj.get("tool_calls"):
            last_resp = obj
            break
    except Exception:
        pass
if last_resp:
    for tc in last_resp.get("tool_calls", []):
        name = tc.get("name", "")
        if name in ["ask_question", "ask", "prompt_user", "get_confirmation", "request_feedback"]:
            print("YES")
            sys.exit(0)
print("NO")
' 2>/dev/null || echo "NO")

            if [[ "$HAS_BG" == "true" && "$IS_ASK" != "YES" ]]; then
                EMOJI="⚙️"
                [[ "$USE_TEXT_STATUS" == true ]] && EMOJI="[WORKING]"
            elif [[ "$IS_ASK" == "YES" ]]; then
                HAS_WORKER=false
                if [[ -n "$AGY_PID" ]]; then
                    for cpid in $(pgrep -P "$AGY_PID" 2>/dev/null || true); do
                        [[ "$cpid" -eq "$$" ]] && continue
                        cmdline=$(ps -p "$cpid" -o args= 2>/dev/null || true)
                        if [[ "$cmdline" =~ statusline || "$cmdline" =~ title\.sh ]]; then
                            continue
                        fi
                        HAS_WORKER=true
                        break
                    done
                fi
                if [[ "$HAS_WORKER" == true ]]; then
                    EMOJI="⚙️"
                    [[ "$USE_TEXT_STATUS" == true ]] && EMOJI="[WORKING]"
                else
                    EMOJI="⚠️"
                    [[ "$USE_TEXT_STATUS" == true ]] && EMOJI="[WAITING]"
                fi
            fi
        fi
        ;;
    idle|*)
        if [[ "$HAS_BG" == "true" ]]; then
            EMOJI="⚙️"
            [[ "$USE_TEXT_STATUS" == true ]] && EMOJI="[WORKING]"
            [[ -n "$MARKER_FILE" ]] && echo "${AGY_PID}:RUNNING" > "$MARKER_FILE" 2>/dev/null || true
        elif [[ -n "$MARKER_FILE" && "$HAS_HISTORY" == true && -f "$MARKER_FILE" ]]; then
            SAVED_CONTENT=$(cat "$MARKER_FILE" 2>/dev/null || echo "")
            SAVED_PID="${SAVED_CONTENT%%:*}"
            if [[ -n "$SAVED_PID" && "$SAVED_PID" == "$AGY_PID" ]]; then
                EMOJI="✅"
                [[ "$USE_TEXT_STATUS" == true ]] && EMOJI="[DONE]"
                echo "${AGY_PID}:COMPLETED" > "$MARKER_FILE" 2>/dev/null || true
            fi
        fi
        ;;
esac

PREFIX_STR=""
[[ "$SHOW_PREFIX" == true ]] && PREFIX_STR="${CUSTOM_PREFIX} "

CLEAN_TITLE=$(echo "$TITLE" | tr '\r\n\t' ' ' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
if [[ ${#CLEAN_TITLE} -gt $MAX_TITLE_LEN ]]; then
    CLEAN_TITLE="${CLEAN_TITLE:0:$((MAX_TITLE_LEN - 3))}..."
fi
echo "${PREFIX_STR}${EMOJI} ${CLEAN_TITLE}"
