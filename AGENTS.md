# AGENTS.md - Antigravity CLI Window Title (`antigravity-cli-title`)

> **Note for AI Agents**: This document provides essential architecture context, design decisions, hard constraints, and development guidelines for this repository. Read this file before proposing or making changes.

---

## 1. Project Mission & Background

### 1.1 The Core Problem
When developers collaborate with Google Antigravity CLI (`agy`) across multiple terminal tabs or windows:
1. **Foreground-Only Statuslines**: Bottom statusbars (like `statusline.ps1`) are invisible when tabs are backgrounded, minimized, or occluded.
2. **False Completion in Vanilla CLI**: When a task completes, the CLI drops back to `idle`. In standard scripts, `idle` simply outputs a static coffee emoji (`☕`) or `idle`, confusing "finished this turn" with "reopened an old session".
3. **Approval Blindness**: When `agy` pauses for `ask_question` or a sensitive permission confirmation (`permissions.ask`), the payload's `agent_state` is reported as `tool_use`. Naive scripts render this as `⚙️ Working`, so the developer has no idea the agent is halted waiting for their input.

### 1.2 Our Solution
A lightweight, low-latency 4-state lifecycle state machine mapped directly to the OS window title:
- **`⚪` [READY]**: Clean initial state / restored history session (waiting for user prompt).
- **`⚙️` [WORKING]**: Actively thinking, running tools, or executing commands.
- **`⚠️` [WAITING]**: Interactively blocked on `ask_question` or permission approval.
- **`✅` [DONE]**: Finished the turn; stays completed until the next prompt is submitted.

---

## 2. Architecture & Key Technical Decisions

### 2.1 File Map
```
antigravity-cli-title/
├── .github/workflows/ci.yml     # AST syntax & Go build/test CI
├── docs/
│   └── demo.gif                 # 4-state lifecycle demonstration animation
├── cmd/
│   └── title/                   # Universal native execution engine (Windows / Linux / macOS)
├── install.ps1 / install.sh     # Zero-config installers (injects into settings.json)
├── uninstall.ps1 / uninstall.sh # Clean uninstallers (preserves user settings)
├── .gitignore                   # Ignores runtime markers and PIDs
├── LICENSE                      # MIT License
├── README.md / README.zh-CN.md  # User-facing documentation (EN / zh-CN)
└── AGENTS.md                    # This developer & AI handoff guide
```

### 2.2 Lifecycle & Process Binding Decision
- **Why simple file timestamps fail**:
  The CLI invokes the title renderer via a transient subprocess wrapper. The wrapper's start time is *always* milliseconds newer than `transcript.jsonl`'s last write time, making naive timestamp comparisons (`fileWriteTime -ge proc.StartTime`) return false 100% of the time.
- **The Fast-Path Native Process Finder**:
  1. We use a microsecond P/Invoke call (`NtQueryInformationProcess`) to traverse ancestor process IDs until the true parent `agy.exe` is reached (~70 ms on first compile, < 1 ms subsequently).
  2. **Multi-Window Isolation**: Each terminal tab's subprocess strictly climbs its *own* ancestor chain. It never relies on shared `Select-Object -First 1` or global PID guessing.
  3. **The Clean Workspace & "No History" Rule**:
     A session can **ONLY** be marked as `✅ Completed` if `hasHistory == true` (i.e. at least one real `USER_INPUT` exists in `transcript.jsonl` or `conversation_title` is set). In a freshly opened workspace before the first prompt, state is **guaranteed to be `⚪ Ready`**.
  4. **The `default` ID Isolation Rule**:
     When `conversation_id` is empty (pre-flight/fresh launch), the renderer will **never** read or write a shared `marker_default.txt`, eliminating cross-session collision.
  5. **Background Subagents & Tasks Integration**:
     When the main agent finishes its turn, `agent_state` transitions to `idle`. However, if active subagents (`$data.subagents`) or background tasks (`$data.task_count > 0`) are running, the session state remains **`⚙️ Working`** and writes `RUNNING` to the marker file. For background tasks, a 60-second smart decay mechanism monitors `.system_generated/tasks/` log activity: if all task logs have been silent for more than 60 seconds (customizable via `--task-timeout`), the session automatically decays and transitions to **`✅ Completed`**, preventing orphaned or hung tasks from permanently jamming the title.
  6. When state transitions to `idle` without background jobs:
     - If `hasHistory == true`, `conversation_id` is valid, and the marker matches the currently running `agy.exe` and was active in this lifetime ➔ **`✅` Completed**.
     - If the marker belongs to a dead PID or doesn't match (user closed CLI and reopened) ➔ **`⚪` Ready**.
     - If `hasHistory == false` ➔ **`⚪` Ready**.

### 2.3 Title Resolution Hierarchy & Fast SQLite Query
Antigravity CLI does not send a `conversation_title` field in standard telemetry stdin payloads. To provide accurate and responsive titles:
1. **Tier 1 - Telemetry Payload**: Check `$data.conversation_title` (if future CLI versions supply it).
2. **Tier 2 - Global SQLite Summary (`conversation_summaries.db`)**:
   - `/resume` reads AI-generated conversation summaries from `~/.gemini/antigravity-cli/conversation_summaries.db`.
   - On Windows, `title.exe` binds directly to Windows 10/11 native `winsqlite3.dll` to query the title in read-only mode (< 1ms, zero third-party dependencies).
   - On Linux/macOS, `title` dynamically queries sqlite3 in read-only URI mode (`file:... ?mode=ro`) when available.
3. **Tier 3 - Immediate Transcript Prompt (`transcript.jsonl`)**:
   - In turn 1 before the model finishes and generates an AI summary, the title immediately mirrors the user's first prompt (`<USER_REQUEST>`).
   - Note: Pipeline results in PowerShell MUST be wrapped in `@(...)` to avoid single-line string unwrapping into `[char]` indexing.
4. **Tier 4 - Working Directory Fallback (`cwd`)**:
   - Used only in fresh, unprompted workspaces (`hasHistory == false`), ensuring clean `⚪ Ready <cwd>` display.

### 2.4 Sub-Event Inspection (`ask_question` & Permissions)
Under `agent_state == "tool_use"`, the script seeks the last 128KB of `transcript.jsonl` to locate the active `PLANNER_RESPONSE` (skipping intermediate completed tool output frames like `type: "GENERIC"`):
1. **Multi-Tool Batch Inspection**: Scans all tool calls in `tool_calls` array (not just index 0), ensuring any pending interactive approval blocks the turn into `⚠️ Waiting`.
2. If tool name is `ask_question`, `ask`, `prompt_user`, `get_confirmation`, `request_feedback` ➔ `⚠️ Waiting`.
3. If tool name is `run_command` matching `permissions.ask` (or not in `permissions.allow`):
   - Supports both `command(regex:...)` and standard glob `command(...)` rules.
   - Unescapes literal escape sequences (`\n`, `\r`, `\t`, `\"`) before regex matching.
   - Check if `agy.exe` already has an active worker child process executing this command (`FastProcFinder::HasActiveWorker` with target command fingerprinting):
     - If matching worker child exists (user confirmed, command executing) ➔ `⚙️ Working`.
     - If no matching worker child exists (blocked on `[y/n]` interactive prompt, ignoring unrelated background tasks/MCP servers) ➔ `⚠️ Waiting`.
4. If tool name is sensitive file operation (`read_file`, `write_file`, `edit_file`) matching `permissions.ask`:
   - Normalizes Windows `\` and Unix `/` separators across path arguments and permission rules.
   - Matches relative paths outside current workspace against parent rules (e.g. `../*`).
   - ➔ `⚠️ Waiting`.
5. If tool name is an MCP tool or custom extension matching `permissions.ask` ➔ `⚠️ Waiting`.
6. Background tasks/subagents do NOT override foreground permission prompts: even with active background jobs, pending user approvals are faithfully presented as `⚠️ Waiting`.
7. Otherwise ➔ `⚙️ Working`.

---

## 3. Hard Constraints & Coding Rules

> [!CAUTION]
> **PowerShell Installer Encoding Rule**:
> - **`install.ps1` and `uninstall.ps1` MUST BE SAVED WITHOUT BOM (UTF-8 No-BOM)**:
>   These scripts are downloaded over HTTP and executed via `irm ... | iex`. When PowerShell's `Invoke-RestMethod` downloads a script with BOM, it decodes the BOM into a leading Unicode char `\uFEFF` in the string. When passed to `iex`, `\uFEFF#` is not recognized as a comment, invalidating the subsequent `param(...)` block and throwing `Invoke-Expression: 赋值表达式无效...` (`InvalidLeftHandSide`).

> [!WARNING]
> **No Blocking Calls in Title Renderer**:
> `title.exe` and `title` are invoked frequently on telemetry events.
> - **NEVER** add network requests or unbounded file reads.
> - All file operations must use safe exception handling.

> [!IMPORTANT]
> **Prefer `transcript_full.jsonl` to Prevent Truncation Blindness**:
> When inspecting tools and arguments, always prioritize `transcript_full.jsonl` over `transcript.jsonl`. Antigravity CLI truncates commands longer than ~2KB in `transcript.jsonl` (inserting `<truncated ... bytes>`), which hides sensitive commands (e.g. `Remove-Item` inside scripts). `transcript_full.jsonl` retains 100% untruncated content with the same 0.01ms seek speed.

---

## 4. Development & Testing Commands

### 4.1 Validate Go Engine & Build
```powershell
go test ./...
go build -ldflags="-s -w" -o title.exe ./cmd/title
```

### 4.2 Test Native Engine with Simulated Payload
```powershell
# Test Working state
@{ cwd = (Get-Location).Path; conversation_title = "Test Task"; agent_state = "working" } | ConvertTo-Json | .\title.exe

# Test Idle state
@{ cwd = (Get-Location).Path; conversation_title = "Test Task"; agent_state = "idle" } | ConvertTo-Json | .\title.exe
```

### 4.3 Test Flags
```powershell
# Test --no-prefix
@{ cwd = (Get-Location).Path; agent_state = "working" } | ConvertTo-Json | .\title.exe --no-prefix

# Test --text mode
@{ cwd = (Get-Location).Path; agent_state = "working" } | ConvertTo-Json | .\title.exe --text
```

---

## 5. Continuous Iteration & Feature Roadmap

This project is actively maintained and evolved by the user. Future agents should treat the following features as ongoing engineering priorities:

### 5.1 Immediate Next Steps (v0.1.0 Release)
- [x] **Demo GIF / Screenshots**: Record an animated GIF or SVG terminal capture showing the 4-state lifecycle progression to embed in `README.md`.
- [x] **GitHub Initial Commit & Push**:
  - `git add .`
  - `git commit -m "feat: initial release of antigravity-cli-title v0.1.0"`
  - Link with remote GitHub repository and push to `main`.
- [x] **Release v0.1.0**: Tagged and pushed `v0.1.0` release tag to GitHub.
- [ ] **Ecosystem Integration**:
  - Open a PR or discussion in `weby-homelab/antigravity-cli-statusline` proposing `antigravity-cli-title` as the recommended companion title plugin.

### 5.2 Upcoming Feature Explorations (v0.2.0+)
- [x] **Fast Native Execution Engine (`title.exe`)**:
  - Implemented zero-dependency, single-binary executable in pure Go (`cmd/title`).
  - Uses Windows native system APIs (`NtQueryInformationProcess`, `winsqlite3.dll`, PEB command-line inspection).
  - Eliminates `powershell.exe` cold-start latency (~13x speedup).
  - Automatically compiled from source or downloaded from GitHub releases by `install.ps1`.
- [ ] **Windows Terminal Tab Progress & Visual Ring (`OSC 9;4`)**:
  - Implement ConEmu / Windows Terminal `OSC 9;4` escape sequence support.
  - When in `⚙️ Working`: Trigger an indeterminate spinning progress ring on the tab header and taskbar.
  - When in `⚠️ Waiting`: Trigger a yellow warning flash/ring.
  - When in `✅ Completed`: Clear/reset progress ring.
- [ ] **Custom State Emoji / Text Mapping Config**:
  - Allow users to override default emojis (`⚪`, `⚙️`, `⚠️`, `✅`) via command-line flags or a user configuration block in `settings.json`.
- [ ] **Custom Terminal Tab Logo / Profile Integration**:
  - Provide an option in `install.ps1` (e.g. `-SetTerminalIcon`) to automatically configure Windows Terminal profiles with an official Antigravity logo (`.ico`/`.png`).
- [ ] **Linux / macOS Terminal Enhancements**:
  - Verify escape sequence compatibility across iTerm2, Kitty, WezTerm, and Alacritty.
