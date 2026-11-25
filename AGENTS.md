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
│   └── title/
│       ├── main.go              # Telemetry, permissions and lifecycle state resolver
│       ├── animation*.go        # Optional Windows animation worker, fallback and tests
│       ├── command_statements.go # Worker command parsing and matching helpers
│       ├── config_read.go       # Bounded settings reads and runtime flag parsing
│       ├── cwd_windows*.go      # Native/WoW64 process cwd inspection and tests
│       ├── proc_*.go            # Platform process trees and SQLite title lookup
│       ├── darwin_parser.go     # macOS process snapshot parsing
│       ├── main_test.go         # Core resolver and parser regression tests
│       ├── review_fixes_test.go # Review-specific boundary regressions
│       └── timeline_test.go     # Windows live worker detection tests
├── install.ps1 / install.sh     # Zero-config installers (injects into settings.json)
├── uninstall.ps1 / uninstall.sh # Clean uninstallers (preserves user settings)
├── test_router.ps1              # AST and router command generation test suite
├── test_review.ps1              # Isolated installer and data-preservation regressions
├── test_py_sh.sh                # POSIX parser and invalid-root regressions
├── .gitattributes               # Cross-platform source/script line endings
├── .gitignore                   # Ignores runtime markers, binaries and local review logs
├── assets/                      # Official Google Antigravity PNG / ICO / SVG icons
├── go.mod                       # Go module definition
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
Antigravity CLI (versions >= 1.4.10) includes `conversation_title` directly in telemetry stdin payloads. To provide accurate and responsive titles across all versions:
1. **Tier 1 - Telemetry Payload**: Check `$data.conversation_title` (provided natively in newer CLI versions).
2. **Tier 2 - Global SQLite Summary (`conversation_summaries.db`)**:
   - `/resume` reads AI-generated conversation summaries from `~/.gemini/antigravity-cli/conversation_summaries.db`.
   - On Windows, `title.exe` binds directly to Windows 10/11 native `winsqlite3.dll` to query the title in read-only mode (< 1ms, zero third-party dependencies).
   - On Linux/macOS, `title` dynamically queries sqlite3 with `-readonly` CLI flag when available.
3. **Tier 3 - Immediate Transcript Prompt (`transcript.jsonl`)**:
   - In turn 1 before the model finishes and generates an AI summary, the title immediately mirrors the user's first prompt (`<USER_REQUEST>`).
   - Note: Pipeline results in PowerShell MUST be wrapped in `@(...)` to avoid single-line string unwrapping into `[char]` indexing.
4. **Tier 4 - Working Directory Fallback (`cwd`)**:
   - Used only in fresh, unprompted workspaces (`hasHistory == false`), ensuring clean `⚪ Ready <cwd>` display.

### 2.4 Sub-Event Inspection (`ask_question` & Permissions)
Under `agent_state == "tool_use"`, the engine inspects the tail of `transcript.jsonl` (or `transcript_full.jsonl`) to locate the active `PLANNER_RESPONSE` and dispatches through the pure `resolveState` resolver:
1. **Multi-Tool Batch Inspection**: Scans all tool calls in `tool_calls` array (not just index 0), ensuring any pending interactive approval in the batch transitions the turn into `⚠️ Waiting`.
2. **Interactive Prompts**: If tool name is `ask_question`, `ask`, `prompt_user`, `get_confirmation`, `request_feedback` ➔ `⚠️ Waiting`.
3. **Command Execution & Worker Verification**:
   - Matches `command(...)` and `run_command(...)` rules: `allow` rules strictly use anchored regex (`^(?i)...$`) preventing command chaining attacks, while `ask` rules support substring and glob matching.
   - Preserves JSON-decoded command text and Windows backslashes; never globally unescapes `\n`, `\r`, or `\t` a second time.
   - When permission requires confirmation (or under nil permissions baseline), inspects the process tree via `hasActiveWorker`:
     - **Directory boundary scoping (`requiredDir`)**: If the command begins with directory navigation (`cd <dir> && ...` or `Push-Location`), verifies that descendant processes match the target directory, preventing false positives from identical commands in other directories.
     - **Primary Executable & Argument Token Matching**: Compares primary executable image (`targetExe`, `altExe`, or `innerExe` e.g. for `powershell -c ...`). Checks all tokens via `matchArgToken` with word boundary enforcement across both `coreTokens` (>= 3 runes, non-flag) and `allTokens`.
     - **Adjacent Phrase Boundary Matching**: Validates multi-token command phrases (`strings.Join(pat.allTokens, " ")`) via `matchPhraseBoundary`, ensuring flags and parameters match adjacent word/quote boundaries without partial substring collisions (e.g. `Start-Sleep -Seconds 5` will never match `git log -n 5`, and `git log -n 5` will not match `git log -n 50`).
     - **Fallback Substantive Statement Rule**: If primary exe check does not match (e.g. PowerShell cmdlets like `Start-Sleep`, shell scripts, tool wrappers like `npm`/`mvn`, or compound command pipelines), tests candidate phrases against `firstStmt`, `subStatements`, and `normalizedFull` with quote/escape stripping, filtering out noise words (`noprofile`, `bypass`, etc.).
     - If matching worker child exists (user confirmed, command currently executing) ➔ `⚙️ Working`.
     - If no matching worker child exists (blocked on interactive `[y/n]` prompt) ➔ `⚠️ Waiting`.
4. **Sensitive File Operations**:
   - For `read_file`, `write_file`, `edit_file`, `replace_file_content`, `multi_replace_file_content`, `write_to_file`, `view_file`:
     - Normalizes Windows `\` and Unix `/` separators.
     - Rooted and root-relative paths (starting with `/` or `\`) are strictly validated against `cwd` boundaries across both Windows drive volumes and Unix roots.
     - Windows drive-relative paths (`C:foo.txt`, `D:foo.txt`) are rejected before `filepath.Join` and conservatively transition to `⚠️ Waiting`.
     - Out-of-workspace writes automatically require user confirmation ➔ `⚠️ Waiting`.
5. **MCP Tools & Extensions**: Any custom extension or MCP tool matching `permissions.ask` ➔ `⚠️ Waiting`.
6. **Task Priority Rule**: Pending user approvals take precedence over background tasks/subagents; interactive prompts are faithfully rendered as `⚠️ Waiting`.
7. **Default**: Otherwise ➔ `⚙️ Working`.

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

### Optional Windows title animation

`title.command` may include `--animate` (or install using `install.ps1 -Animate`).
The ordinary telemetry callback stays short-lived. A separate `--animation-worker`
process attaches to the owning agy console and calls `SetConsoleTitleW` every 100ms.
Workers are isolated by PID + creation time and a named mutex; stdin/stdout pipes
are not inherited. A small atomically replaced state file carries the resolved
title and Working flag. Waiting/Ready/Done stop animation; `--text` and a custom
Working symbol retain static display. Removing `--animate` stops the worker on its
next settings poll. The state file records the callback's selected settings path;
CLI-only workers do not poll unrelated default settings. On Windows, callbacks
honor the owning agy's `--settings` argument. `-NoAnimate` disables inherited
animation through the installer, including PowerShell 5.1 `-File` calls.
Workers ignore console Ctrl+C and retire under the state lock so a fast resume
cannot lose the next worker. Linux/macOS currently retain static titles.
If the selected settings file disappears, the worker restores the static title
and stops on its next settings poll. Animation boolean flags support
`--animate`, `--animate=true`, `--animate=false` and `--no-animate`.

Callbacks and workers share a time-based spinner phase; callbacks output a spinner
frame immediately when animation publication succeeds. The worker paints before
waiting for its first 100ms tick. Callback start timestamps reject older in-flight
updates. Settings/task inspection runs outside state ownership and is followed by
a state comparison before painting, so a new foreground update wins.
Animation-enabled callbacks persist static states too, including the first Ready,
Waiting or Done before any worker exists. This initial timestamp record rejects a
late first Working callback; ordinary static mode still avoids creating animation
state files. Invalid/incomplete settings reads leave the worker running until its
next settings poll; valid removal/disable and missing files stop it as before.

An explicit idle callback with background tasks (and no active subagents) may
include a completion check in animation state. The worker rechecks known task-log
activity roughly once per second, applies the existing task-timeout policy, and
publishes a static Done title without another callback. Missing/unreadable logs
are unknown, and foreground/subagent silence never proves completion.

`--trace-state` enables bounded per-process diagnostic JSONL metadata (no prompt,
command, file contents, title or workspace path). Record incoming state, counts,
decision reason, and publication failures when investigating delayed completion.
Worker title API failures include a numeric Win32 error when available, without
recording the title text or settings path.
Final-state publication retries are bounded. If a final state cannot replace the
state file, a named event asks the existing worker to stop before the callback's
static stdout title is applied; no terminal-output transport or WT profile logic
is changed by this fallback.

Do not render terminal control sequences into the callback's captured stdout.
`go test ./...` includes hidden-console tests for continuous playback, independent
consoles, static transitions, settings disable and owner-process exit.

### 4.1 Validate Go Engine & Build
```powershell
gofmt -l .
go vet ./...
go test ./...
go build -ldflags="-s -w" -o title.exe ./cmd/title
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\test_router.ps1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\test_review.ps1
pwsh -NoProfile -File .\test_review.ps1
bash test_py_sh.sh
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
- [x] **Custom Terminal Tab Logo / Profile Integration**:
  - Implemented high-res Antigravity logo assets (`assets/antigravity.png`, `assets/antigravity.ico`).
  - Added `-WTProfile` in `install.ps1` to automatically configure Windows Terminal with a dedicated "Google Antigravity" profile.
  - Implemented smart router in PowerShell `$PROFILE` so typing `agy` from any normal tab automatically opens under the Antigravity profile with dedicated logo.
- [x] **Custom State Emoji / Text Mapping Config**:
  - Implemented configurable state indicators via `--ready`, `--working`, `--waiting`, and `--done` CLI flags.
  - Supports individual or full symbol overrides across both emoji and text modes with zero runtime overhead.
- [x] **Config Hot-Reload & Zero-Backslash Safety**:
  - Hot-reload via `settings.json` `title.command` dynamic argument parsing with instant runtime reflection (0 restart needed).
  - Standalone custom JSON keys in `title` are avoided because Antigravity CLI's native Go schema purges unwhitelisted properties upon CLI restarts/rewrites.
  - Robust quote stripping (`cleanSymbol`) and space-aware argument handling to eliminate harmful `\"` backslash escapes in `command`.
  - Multi-tier priority resolution: `settings.json command string > CLI Flag > Built-in preset` (for instant hot-reload).
- [x] **Optional Windows Title Animation & Cross-Platform Hardening (`v0.4.0`)**:
  - Added detached `--animation-worker` with PID + creation-time mutex isolation, Ctrl+C resilience, `--settings` scoping and clean lock retirement.
  - Added JSONC-aware Windows Terminal installer preservation, pure-ASCII PS profile router, WoW64 CWD matching, bounded transcript inspection, and POSIX installer regressions.
- [-] **Windows Terminal Tab Progress & Visual Ring (`OSC 9;4`)**:
  - Fully tested and evaluated: Windows Terminal's native WinUI ProgressRing forces system accent color (blue, breaking yellow warning indication) and occludes the tab profile icon. Dropped in favor of pure 4-state Emoji in title.
- [ ] **Linux / macOS Terminal Enhancements**:
  - Verify escape sequence compatibility across iTerm2, Kitty, WezTerm, and Alacritty.
