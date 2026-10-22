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
├── .github/workflows/ci.yml     # AST syntax verification CI
├── docs/
│   └── demo.gif                 # 4-state lifecycle demonstration animation
├── scripts/
│   ├── title.ps1                # Windows native renderer (PowerShell 5.1+ & 7+)
│   └── title.sh                 # Linux / macOS native renderer (POSIX Bash)
├── install.ps1 / install.sh     # Zero-config installers (injects into settings.json)
├── uninstall.ps1 / uninstall.sh # Clean uninstallers (preserves user settings)
├── .gitignore                   # Ignores runtime markers and PIDs
├── LICENSE                      # MIT License
├── README.md / README.zh-CN.md  # User-facing documentation (EN / zh-CN)
└── AGENTS.md                    # This developer & AI handoff guide
```

### 2.2 Lifecycle & Process Binding Decision
- **Why simple file timestamps fail**:
  The CLI invokes `title.ps1` via a transient subprocess wrapper. The wrapper's start time is *always* milliseconds newer than `transcript.jsonl`'s last write time, making naive timestamp comparisons (`fileWriteTime -ge proc.StartTime`) return false 100% of the time.
- **The Fast-Path Native Process Finder**:
  1. We use a microsecond P/Invoke call (`NtQueryInformationProcess`) to traverse ancestor process IDs until the true parent `agy.exe` is reached (~70 ms on first compile, < 1 ms subsequently).
  2. **Multi-Window Isolation**: Each terminal tab's subprocess strictly climbs its *own* ancestor chain. It never relies on shared `Select-Object -First 1` or global PID guessing.
  3. **The Clean Workspace & "No History" Rule**:
     A session can **ONLY** be marked as `✅ Completed` if `hasHistory == true` (i.e. at least one real `USER_INPUT` exists in `transcript.jsonl` or `conversation_title` is set). In a freshly opened workspace before the first prompt, state is **guaranteed to be `⚪ Ready`**.
  4. **The `default` ID Isolation Rule**:
     When `conversation_id` is empty (pre-flight/fresh launch), the renderer will **never** read or write a shared `marker_default.txt`, eliminating cross-session collision.
  5. When state transitions to `idle`:
     - If `hasHistory == true`, `conversation_id` is valid, and the marker matches the currently running `agy.exe` and was active in this lifetime ➔ **`✅` Completed**.
     - If the marker belongs to a dead PID or doesn't match (user closed CLI and reopened) ➔ **`⚪` Ready**.
     - If `hasHistory == false` ➔ **`⚪` Ready**.

### 2.3 Title Resolution Hierarchy & Fast SQLite Query
Antigravity CLI does not send a `conversation_title` field in standard telemetry stdin payloads. To provide accurate and responsive titles:
1. **Tier 1 - Telemetry Payload**: Check `$data.conversation_title` (if future CLI versions supply it).
2. **Tier 2 - Global SQLite Summary (`conversation_summaries.db`)**:
   - `/resume` reads AI-generated conversation summaries from `~/.gemini/antigravity-cli/conversation_summaries.db`.
   - On Windows, `title.ps1` binds directly to Windows 10/11 native `winsqlite3.dll` in `FastProcFinder` to query the title in read-only mode (< 1ms, zero third-party dependencies).
   - On Linux/macOS, `title.sh` uses standard Python `sqlite3` in URI read-only mode (`file:... ?mode=ro`).
3. **Tier 3 - Immediate Transcript Prompt (`transcript.jsonl`)**:
   - In turn 1 before the model finishes and generates an AI summary, the title immediately mirrors the user's first prompt (`<USER_REQUEST>`).
   - Note: Pipeline results in PowerShell MUST be wrapped in `@(...)` to avoid single-line string unwrapping into `[char]` indexing.
4. **Tier 4 - Working Directory Fallback (`cwd`)**:
   - Used only in fresh, unprompted workspaces (`hasHistory == false`), ensuring clean `⚪ Ready <cwd>` display.

### 2.4 Sub-Event Inspection (`ask_question` & Permissions)
Under `agent_state == "tool_use"`, the script seeks the last 16KB of `transcript.jsonl` to locate the final `PLANNER_RESPONSE`:
1. If tool name is `ask_question`, `ask`, `prompt_user`, etc. ➔ `⚠️ Waiting`.
2. If tool name is `run_command` / file edit and matches `settings.json`'s `permissions.ask` (or is not in `permissions.allow`) ➔ `⚠️ Waiting`.
3. Otherwise ➔ `⚙️ Working`.

---

## 3. Hard Constraints & Coding Rules

> [!CAUTION]
> **PowerShell Encoding Rule**:
> Every `.ps1` file in this repository **MUST BE SAVED WITH UTF-8 BOM (`utf-8-sig`)**.
> Windows PowerShell 5.1 parses non-BOM UTF-8 files using the system default code page (e.g. GBK/Windows-1252), which corrupts multibyte Unicode emojis and strings (e.g., turning `⚪` into broken double-byte tokens like `鈿?`), resulting in fatal syntax errors.

> [!WARNING]
> **No Blocking Calls in Title Renderer**:
> `title.ps1` and `title.sh` are invoked frequently on telemetry events.
> - **NEVER** add network requests or unbounded file reads.
> - **NEVER** run broad `Get-CimInstance` queries without PID-specific filtering and caching.
> - All file operations must use safe exception handling (`try {} catch {}`).

> [!IMPORTANT]
> **FileShare.ReadWrite on `transcript.jsonl`**:
> Never use `[System.IO.File]::OpenRead($path)` to inspect `transcript.jsonl`. When a user submits a prompt, `agy.exe` holds an active write lock on the file. Standard `OpenRead` lacks the `FileShare.ReadWrite` flag, causing an immediate `IOException` (file in use by another process), silently falling back to the directory name. Always use `[System.IO.File]::Open($path, [System.IO.FileMode]::Open, [System.IO.FileAccess]::Read, [System.IO.FileShare]::ReadWrite)`.

---

## 4. Development & Testing Commands

### 4.1 Validate PowerShell Syntax (AST)
```powershell
$scripts = Get-ChildItem -Recurse -Filter *.ps1
foreach ($s in $scripts) {
    $errors = $null
    [System.Management.Automation.Language.Parser]::ParseFile($s.FullName, [ref]$null, [ref]$errors) | Out-Null
    if ($errors) { Write-Error "AST Error in $($s.Name): $errors"; exit 1 }
}
Write-Host "All PowerShell scripts passed AST checks." -ForegroundColor Green
```

### 4.2 Test Local Renderer with Simulated Payload
```powershell
# Test Working state
@{ cwd = (Get-Location).Path; conversation_title = "Test Task"; agent_state = "working" } | ConvertTo-Json | powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts\title.ps1

# Test Idle state
@{ cwd = (Get-Location).Path; conversation_title = "Test Task"; agent_state = "idle" } | ConvertTo-Json | powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts\title.ps1
```

### 4.3 Test Flags
```powershell
# Test --no-prefix
@{ cwd = (Get-Location).Path; agent_state = "working" } | ConvertTo-Json | powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts\title.ps1 --no-prefix

# Test --text mode
@{ cwd = (Get-Location).Path; agent_state = "working" } | ConvertTo-Json | powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts\title.ps1 --text
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

### 5.2 Upcoming Feature Roadmap (Prioritized by Necessity)

Future agents and contributors should implement features in the following prioritized order:

1. **Priority 1 (Core Performance) - Fast Native Execution Engine (`title.exe`)**:
   - **Root cause**: The current ~1.0s - 1.5s delay is dominated by Windows spawning `powershell.exe` and loading the .NET CLR runtime.
   - **Solution**: Compile a zero-dependency, single-binary executable (written in Go or Rust, < 1MB) that performs stdin parsing, ancestor traversal, and SQLite query natively.
   - **Target**: Shrink end-to-end title render latency from ~1500ms down to **< 20ms** (true imperceptible instant updates).
   - **Dual-Track Compatibility**: Retain `title.ps1` as the zero-compilation default fallback; `install.ps1` auto-detects and configures the native binary if available.

2. **Priority 2 (Visual Impact) - Windows Terminal Tab Progress & Visual Ring (`OSC 9;4`)**:
   - Implement ConEmu / Windows Terminal `OSC 9;4` escape sequence support.
   - When in `⚙️ Working`: Trigger an indeterminate spinning progress ring on the tab header and Windows taskbar.
   - When in `⚠️ Waiting`: Trigger a yellow warning flash/ring.
   - When in `✅ Completed`: Clear/reset progress ring.

3. **Priority 3 (User Customization) - Custom State Emoji / Text Mapping Config**:
   - Allow users to override default state emojis (`⚪`, `⚙️`, `⚠️`, `✅`) or text modes via command-line flags or a user configuration block in `settings.json`.

4. **Priority 4 (Cross-Platform Polish) - Linux / macOS Terminal Enhancements**:
   - Verify and tune escape sequence compatibility across iTerm2, Kitty, WezTerm, and Alacritty.

5. **Priority 5 (Cosmetic) - Custom Terminal Tab Logo / Profile Integration**:
   - Provide an option in `install.ps1` (e.g. `-SetTerminalIcon`) to automatically configure Windows Terminal profiles with an official Antigravity logo (`.ico`/`.png`).
