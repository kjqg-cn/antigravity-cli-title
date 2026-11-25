package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestGlobToRegex(t *testing.T) {
	pat, patSub := globToRegex("git*")
	rx1, err := regexp.Compile(pat)
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	if !rx1.MatchString("git status") {
		t.Errorf("Expected git* to match 'git status'")
	}
	if rx1.MatchString("mygit") {
		t.Errorf("Expected git* not to match 'mygit'")
	}

	rxSub, err := regexp.Compile(patSub)
	if err != nil {
		t.Fatalf("Compile error for patSub: %v", err)
	}
	if !rxSub.MatchString("run git status") {
		t.Errorf("Expected patSub to match 'run git status'")
	}
}

func TestFileGlobToRegex(t *testing.T) {
	pat, patSub := fileGlobToRegex("*.json")
	rx, err := regexp.Compile(pat)
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	if !rx.MatchString("config.json") {
		t.Errorf("Expected *.json to match 'config.json'")
	}

	rxSub, err := regexp.Compile(patSub)
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	if !rxSub.MatchString("path/to/settings.json") {
		t.Errorf("Expected patSub to match 'path/to/settings.json'")
	}
}

func TestCommandPathPreserved(t *testing.T) {
	raw := `python C:\temp\new\runner.py`
	expected := strings.ToLower(raw)
	got := normalizeCmdText(raw)
	if got != expected {
		t.Errorf("Expected %q, got %q", expected, got)
	}
}

func TestCleanStdin(t *testing.T) {
	// UTF-8 BOM
	withBOM := []byte("\xef\xbb\xbf{\"status\": \"ok\"}")
	cleaned := cleanStdin(withBOM)
	if string(cleaned) != "{\"status\": \"ok\"}" {
		t.Errorf("Failed to strip UTF-8 BOM: %s", string(cleaned))
	}

	// UTF-16LE BOM
	withU16LE := []byte{0xff, 0xfe, 'a', 0x00, 'b', 0x00}
	cleaned16 := cleanStdin(withU16LE)
	if string(cleaned16) != "ab" {
		t.Errorf("Failed to decode UTF-16LE: %s", string(cleaned16))
	}
}

func TestHasRecentlyActiveTasks(t *testing.T) {
	tempDir := t.TempDir()
	tasksDir := filepath.Join(tempDir, "tasks")
	_ = os.MkdirAll(tasksDir, 0755)

	// 1. Initially no log files -> returns false
	if hasRecentlyActiveTasks("", "", filepath.Join(tempDir, "logs", "transcript.jsonl"), 60*time.Second) {
		t.Errorf("Expected false when no log files exist")
	}

	// 2. Create an active log file modified just now
	logFile := filepath.Join(tasksDir, "task-100.log")
	_ = os.WriteFile(logFile, []byte("running"), 0644)

	if !hasRecentlyActiveTasks("", "", filepath.Join(tempDir, "logs", "transcript.jsonl"), 60*time.Second) {
		t.Errorf("Expected true when active log file was just modified")
	}

	// 3. Set log file mod time to 2 minutes ago
	oldTime := time.Now().Add(-2 * time.Minute)
	_ = os.Chtimes(logFile, oldTime, oldTime)

	if hasRecentlyActiveTasks("", "", filepath.Join(tempDir, "logs", "transcript.jsonl"), 60*time.Second) {
		t.Errorf("Expected false when log file is older than 60s")
	}
}

func TestResolveStatusSymbols(t *testing.T) {
	// 1. Default emoji symbols
	s1 := resolveStatusSymbols(false, "", "", "", "")
	if s1.Ready != "⚪" || s1.Working != "⚙️" || s1.Waiting != "⚠️" || s1.Done != "✅" {
		t.Errorf("Unexpected default symbols: %+v", s1)
	}

	// 2. Default text symbols
	s2 := resolveStatusSymbols(true, "", "", "", "")
	if s2.Ready != "[READY]" || s2.Working != "[WORKING]" || s2.Waiting != "[WAITING]" || s2.Done != "[DONE]" {
		t.Errorf("Unexpected text symbols: %+v", s2)
	}

	// 3. Custom symbols override defaults
	s3 := resolveStatusSymbols(false, "💤", "⚡", "🔔", "🎉")
	if s3.Ready != "💤" || s3.Working != "⚡" || s3.Waiting != "🔔" || s3.Done != "🎉" {
		t.Errorf("Unexpected custom symbols: %+v", s3)
	}

	// 4. Partial custom overrides with text mode
	s4 := resolveStatusSymbols(true, "", "RUNNING", "", "SUCCESS")
	if s4.Ready != "[READY]" || s4.Working != "RUNNING" || s4.Waiting != "[WAITING]" || s4.Done != "SUCCESS" {
		t.Errorf("Unexpected partial text override symbols: %+v", s4)
	}
}

func TestCleanSymbol(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"🔔", "🔔"},
		{"  🔔  ", "🔔"},
		{"\"🔔\"", "🔔"},
		{"'🔔'", "🔔"},
		{"\"[WAITING]\"", "[WAITING]"},
		{"'WAITING'", "WAITING"},
		{"\"  🔔  \"", "🔔"},
		{"\"\"", ""},
		{"''", ""},
	}

	for _, tt := range tests {
		actual := cleanSymbol(tt.input)
		if actual != tt.expected {
			t.Errorf("cleanSymbol(%q) = %q, expected %q", tt.input, actual, tt.expected)
		}
	}
}

func TestResolveConfiguredSymbolsPriority(t *testing.T) {
	cfg := &TitleConfig{
		Ready:   "🟢",
		Working: "🛠️",
		Waiting: "\"🔔\"", // quoted in config
		Done:    "🏁",
	}

	// 1. Config applies when CLI flags are empty
	s1 := resolveConfiguredSymbols(false, "", "", "", "", cfg)
	if s1.Ready != "🟢" || s1.Working != "🛠️" || s1.Waiting != "🔔" || s1.Done != "🏁" {
		t.Errorf("Expected config to apply, got: %+v", s1)
	}

	// 2. Settings.json direct keys override CLI flags (hot-reload priority)
	s2 := resolveConfiguredSymbols(false, "'⚪'", "", "⚠️", "", cfg)
	if s2.Ready != "🟢" {
		t.Errorf("Expected config override for Ready ('🟢' over '⚪'), got: %s", s2.Ready)
	}
	if s2.Working != "🛠️" {
		t.Errorf("Expected config value for Working, got: %s", s2.Working)
	}
	if s2.Waiting != "🔔" {
		t.Errorf("Expected config override for Waiting ('🔔' over '⚠️'), got: %s", s2.Waiting)
	}
	if s2.Done != "🏁" {
		t.Errorf("Expected config value for Done, got: %s", s2.Done)
	}

	// 3. CLI flags override defaults when not specified in config
	cfgPartial := &TitleConfig{
		Working: "⚡",
	}
	s3 := resolveConfiguredSymbols(false, "🔴", "", "🔔", "", cfgPartial)
	if s3.Ready != "🔴" {
		t.Errorf("Expected CLI flag for Ready ('🔴') when config doesn't specify it, got: %s", s3.Ready)
	}
	if s3.Working != "⚡" {
		t.Errorf("Expected config for Working ('⚡' over CLI default), got: %s", s3.Working)
	}
	if s3.Waiting != "🔔" {
		t.Errorf("Expected CLI flag for Waiting ('🔔') when config doesn't specify it, got: %s", s3.Waiting)
	}
	if s3.Done != "✅" {
		t.Errorf("Expected default Done ('✅'), got: %s", s3.Done)
	}

	// 4. Command string hot-reload test: direct key > command string > CLI flag
	cfgCmd := &TitleConfig{
		Command: "C:/path/title.exe --no-prefix --ready 🟡 --working ⚡ --done 🎉",
		Waiting: "🔔", // direct key overrides
	}
	s4 := resolveConfiguredSymbols(false, "⚪", "", "", "🏁", cfgCmd)
	if s4.Ready != "🟡" {
		t.Errorf("Expected Ready to be parsed from Command string ('🟡' over CLI '⚪'), got: %s", s4.Ready)
	}
	if s4.Working != "⚡" {
		t.Errorf("Expected Working to be parsed from Command string, got: %s", s4.Working)
	}
	if s4.Done != "🎉" {
		t.Errorf("Expected Done to be parsed from Command string ('🎉' over CLI '🏁'), got: %s", s4.Done)
	}
	if s4.Waiting != "🔔" {
		t.Errorf("Expected Waiting to be direct key override, got: %s", s4.Waiting)
	}
}

func TestMatchWord(t *testing.T) {
	cases := []struct {
		s        string
		word     string
		expected bool
	}{
		{"node ./packages/algorithm/index.js", "go", false},
		{"c:\\go\\bin\\go.exe test", "go", true},
		{"go test ./...", "go", true},
		{"cargo build", "go", false},
		{"echo mango", "go", false},
		{"python.exe script.py", "python", true},
		{"python3 script.py", "python", false},
		{"rm -rf /tmp/test", "rm", true},
		{"format c:", "rm", false},
		{"powershell -Command Start-Sleep -Seconds 5", "start-sleep", true},
		{"", "go", false},
		{"go", "", false},
		{"bin/sh -c run", "sh", true},
	}

	for _, c := range cases {
		actual := matchWord(c.s, c.word)
		if actual != c.expected {
			t.Errorf("matchWord(%q, %q) = %v; want %v", c.s, c.word, actual, c.expected)
		}
	}
}

func TestParseTranscriptTail_MultiToolBatch(t *testing.T) {
	tempDir := t.TempDir()
	transcriptPath := filepath.Join(tempDir, "transcript.jsonl")

	// Case 1: PLANNER_RESPONSE with 2 tool calls, followed by 1 GENERIC frame (incomplete batch)
	// Under P1-1: FIFO slicing is discarded; the entire batch (len == 2) is preserved so pending approvals aren't lost
	content1 := `{"type":"USER_INPUT","content":"test"}
{"type":"PLANNER_RESPONSE","tool_calls":[{"name":"read_file","args":{"Path":"foo.txt"}},{"name":"run_command","args":{"CommandLine":"rm -rf /"}}]}
{"type":"GENERIC","content":"file contents"}
`
	if err := os.WriteFile(transcriptPath, []byte(content1), 0644); err != nil {
		t.Fatal(err)
	}

	pr, err := parseTranscriptTail(transcriptPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr == nil {
		t.Fatalf("expected non-nil PlannerResponse when only 1 of 2 tools completed, got nil (approval blindness)")
	}
	if len(pr.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls in pr (full batch preserved), got %d", len(pr.ToolCalls))
	}

	// Case 2: 2nd GENERIC frame added -> all tools completed -> should return nil
	content2 := content1 + `{"type":"GENERIC","content":"command output"}
`
	if err := os.WriteFile(transcriptPath, []byte(content2), 0644); err != nil {
		t.Fatal(err)
	}

	pr2, err := parseTranscriptTail(transcriptPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr2 != nil {
		t.Errorf("expected nil PlannerResponse when all tools completed, got %+v", pr2)
	}

	// Case 3: Out-of-order completion: tool 0 is ask_question, tool 1 is read_file (completed with 1 GENERIC)
	content3 := `{"type":"USER_INPUT","content":"interactive question"}
{"type":"PLANNER_RESPONSE","tool_calls":[{"name":"ask_question","args":{"question":"proceed?"}},{"name":"read_file","args":{"Path":"config.json"}}]}
{"type":"GENERIC","content":"{\"config\":\"val\"}"}
`
	if err := os.WriteFile(transcriptPath, []byte(content3), 0644); err != nil {
		t.Fatal(err)
	}
	pr3, err := parseTranscriptTail(transcriptPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr3 == nil {
		t.Fatalf("expected non-nil PlannerResponse when interactive ask_question is still pending")
	}
	if len(pr3.ToolCalls) != 2 {
		t.Fatalf("expected full batch preserved with len=2, got %d", len(pr3.ToolCalls))
	}
	hasAskQuestion := false
	for _, tc := range pr3.ToolCalls {
		if tc.Name == "ask_question" {
			hasAskQuestion = true
			break
		}
	}
	if !hasAskQuestion {
		t.Errorf("expected ask_question to be preserved in pending tools, but it was lost")
	}
}

func TestGetFirstPromptFromTranscript_Subagent(t *testing.T) {
	tempDir := t.TempDir()
	transcriptPath := filepath.Join(tempDir, "transcript.jsonl")

	// Subagent transcript with SYSTEM_MESSAGE containing <original_task>
	subagentContent := `{"type":"SYSTEM_MESSAGE","content":"You are a subagent.<SYSTEM_MESSAGE>\n<original_task>\n**Task**: /boost 逐项修复\nAdditional Context:\n- foo\n</original_task>\n</SYSTEM_MESSAGE>"}
{"type":"PLANNER_RESPONSE","tool_calls":[]}
`
	if err := os.WriteFile(transcriptPath, []byte(subagentContent), 0644); err != nil {
		t.Fatal(err)
	}

	title, found := getFirstPromptFromTranscript(transcriptPath, 35)
	if !found {
		t.Fatalf("expected found=true for subagent transcript, got false")
	}
	expected := "/boost 逐项修复"
	if title != expected {
		t.Errorf("expected title %q, got %q", expected, title)
	}
}

func TestGetFirstPromptFromTranscript_SubagentMessageContent(t *testing.T) {
	tempDir := t.TempDir()
	transcriptPath := filepath.Join(tempDir, "transcript.jsonl")

	// Subagent transcript with [Message] ... content=... (without <original_task>)
	msgContent := `{"type":"SYSTEM_MESSAGE","content":"The following is a <SYSTEM_MESSAGE> not actually sent by the user.\n\n<SYSTEM_MESSAGE>\n[Message] timestamp=2026-10-20T23:06:00Z sender=foo priority=MESSAGE_PRIORITY_HIGH content=Go 代码结构排查专家\n请排查本地文件\n</SYSTEM_MESSAGE>"}`
	if err := os.WriteFile(transcriptPath, []byte(msgContent), 0644); err != nil {
		t.Fatal(err)
	}

	title, found := getFirstPromptFromTranscript(transcriptPath, 35)
	if !found {
		t.Fatalf("expected found=true for subagent message transcript, got false")
	}
	expected := "Go 代码结构排查专家"
	if title != expected {
		t.Errorf("expected title %q, got %q", expected, title)
	}
}

func TestGetFirstPromptFromTranscript_UTF8BOM(t *testing.T) {
	tempDir := t.TempDir()
	transcriptPath := filepath.Join(tempDir, "transcript.jsonl")

	// Transcript starting with UTF-8 BOM (\xef\xbb\xbf)
	bomContent := append([]byte("\xef\xbb\xbf"), []byte(`{"type":"USER_INPUT","content":"<USER_REQUEST>Fix bug with UTF-8 BOM</USER_REQUEST>"}`)...)
	if err := os.WriteFile(transcriptPath, bomContent, 0644); err != nil {
		t.Fatal(err)
	}

	title, found := getFirstPromptFromTranscript(transcriptPath, 35)
	if !found {
		t.Fatalf("expected found=true for BOM transcript, got false")
	}
	expected := "Fix bug with UTF-8 BOM"
	if title != expected {
		t.Errorf("expected title %q, got %q", expected, title)
	}
}

func TestCleanPromptLine(t *testing.T) {
	cases := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"**Task**: /boost fix bug", 35, "/boost fix bug"},
		{"task: do work", 35, "do work"},
		{"# Heading prompt", 35, "Heading prompt"},
		{"**Task**: [FEAT] very long line that exceeds the maximum title length limit", 20, "[FEAT] very long ..."},
	}

	for _, c := range cases {
		actual := cleanPromptLine(c.input, c.maxLen)
		if actual != c.expected {
			t.Errorf("cleanPromptLine(%q, %d) = %q; want %q", c.input, c.maxLen, actual, c.expected)
		}
	}
}

func TestIsPathInsideCwd(t *testing.T) {
	tempDir := t.TempDir()
	cwd := filepath.Join(tempDir, "workspace")
	_ = os.MkdirAll(cwd, 0755)

	cases := []struct {
		name     string
		cwd      string
		target   string
		expected bool
	}{
		// Boundary confusion cases (P1-2)
		{"Sibling prefix confusion", cwd, cwd + "2", false},
		{"Sibling file prefix confusion", cwd, filepath.Join(tempDir, "workspace2", "secret.txt"), false},
		{"Direct child file", cwd, filepath.Join(cwd, "file.txt"), true},
		{"Subdir child file", cwd, filepath.Join(cwd, "subdir", "sub.txt"), true},
		{"Relative direct child", cwd, "file.txt", true},
		{"Relative subdir child", cwd, "subdir/sub.txt", true},
		{"Relative dot slash child", cwd, "./file.txt", true},
		{"Relative parent traversal escape", cwd, "../secret.txt", false},
		{"Relative deep parent traversal escape", cwd, "subdir/../../secret.txt", false},
		{"Same directory", cwd, cwd, true},
		{"Same directory relative dot", cwd, ".", true},
		{"Empty cwd", "", "file.txt", false},
		{"Empty target", cwd, "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual := isPathInsideCwd(c.cwd, c.target)
			if actual != c.expected {
				t.Errorf("isPathInsideCwd(%q, %q) = %v; want %v", c.cwd, c.target, actual, c.expected)
			}
		})
	}
}

func TestConversationIDSecurity(t *testing.T) {
	cases := []struct {
		id       string
		expected bool
	}{
		{"conv-123_abc", true},
		{"12345", true},
		{"a", true},
		{"../../etc/passwd", false},
		{"..\\windows\\system32", false},
		{"path/traversal", false},
		{"conv id with spaces", false},
		{"conv;rm -rf /", false},
		{"", false},
		{strings.Repeat("a", 128), true},
		{strings.Repeat("a", 129), false},
	}

	for _, c := range cases {
		actual := rxConvID.MatchString(c.id)
		if actual != c.expected {
			t.Errorf("rxConvID.MatchString(%q) = %v; want %v", c.id, actual, c.expected)
		}
	}
}

func TestParseDarwinPsLine(t *testing.T) {
	// 1. Standard CLI process
	line1 := " 12345   100 /bin/bash                Thu Oct 29 19:29:34 2026 /bin/bash -c echo hi"
	pid, ppid, comm, ticks, cmdline, ok := parseDarwinPsLine(line1)
	if !ok {
		t.Fatalf("expected ok=true for line1")
	}
	if pid != 12345 || ppid != 100 {
		t.Errorf("expected pid=12345, ppid=100; got pid=%d, ppid=%d", pid, ppid)
	}
	if comm != "/bin/bash" {
		t.Errorf("expected comm='/bin/bash', got %q", comm)
	}
	if ticks == 0 {
		t.Errorf("expected non-zero ticks")
	}
	if cmdline != "/bin/bash -c echo hi" {
		t.Errorf("expected cmdline='/bin/bash -c echo hi', got %q", cmdline)
	}

	// 2. Process with multi-word comm
	line2 := "  999     1 Google Chrome Helper     Fri Oct 30 08:00:00 2026 /Applications/Google Chrome.app/Contents/MacOS/Google Chrome Helper --type=utility"
	pid2, ppid2, comm2, _, cmdline2, ok2 := parseDarwinPsLine(line2)
	if !ok2 {
		t.Fatalf("expected ok=true for line2")
	}
	if pid2 != 999 || ppid2 != 1 {
		t.Errorf("expected pid=999, ppid=1; got %d, %d", pid2, ppid2)
	}
	if comm2 != "Google Chrome Helper" {
		t.Errorf("expected comm='Google Chrome Helper', got %q", comm2)
	}
	if !strings.Contains(cmdline2, "--type=utility") {
		t.Errorf("expected cmdline to contain flag, got %q", cmdline2)
	}

	// 3. Single-digit day (first 9 days of every month with space padding)
	line3 := " 55555   200 /bin/zsh                 Thu Oct  1 19:29:34 2026 /bin/zsh -c make test"
	pid3, ppid3, comm3, ticks3, cmdline3, ok3 := parseDarwinPsLine(line3)
	if !ok3 {
		t.Fatalf("expected ok=true for line3 (single-digit day)")
	}
	if pid3 != 55555 || ppid3 != 200 {
		t.Errorf("expected pid=55555, ppid=200; got %d, %d", pid3, ppid3)
	}
	if comm3 != "/bin/zsh" {
		t.Errorf("expected comm='/bin/zsh', got %q", comm3)
	}
	// ticks must be a genuine nanosecond timestamp (> 1e18 for 2026), NOT fallback int64(pid)=55555
	if ticks3 <= 1000000000 {
		t.Errorf("expected ticks to be parsed time in nanoseconds, got %d (fallback to pid occurred)", ticks3)
	}
	if cmdline3 != "/bin/zsh -c make test" {
		t.Errorf("expected cmdline='/bin/zsh -c make test', got %q", cmdline3)
	}

	// 4. Comm containing numbers and colons
	line4 := "  888     1 my:app 2025              Wed Jul  8 12:00:00 2026 my:app --serve"
	pid4, _, comm4, ticks4, cmdline4, ok4 := parseDarwinPsLine(line4)
	if !ok4 {
		t.Fatalf("expected ok=true for line4")
	}
	if pid4 != 888 {
		t.Errorf("expected pid=888, got %d", pid4)
	}
	if comm4 != "my:app 2025" {
		t.Errorf("expected comm='my:app 2025', got %q", comm4)
	}
	if ticks4 <= 1000000000 {
		t.Errorf("expected genuine ticks for line4, got %d", ticks4)
	}
	if cmdline4 != "my:app --serve" {
		t.Errorf("expected cmdline='my:app --serve', got %q", cmdline4)
	}

	// 5. Short / invalid lines
	if _, _, _, _, _, ok5 := parseDarwinPsLine(""); ok5 {
		t.Errorf("expected ok=false for empty line")
	}
	if _, _, _, _, _, ok6 := parseDarwinPsLine("not a process line"); ok6 {
		t.Errorf("expected ok=false for invalid line")
	}

	// 6. Localized Chinese (zh_CN) strftime ("%a %b/%e %T %Y", 4 tokens)
	lineZh := " 12345   100 agy                      四 10月/30 01:45:12 2026 /path/to/agy"
	pidZh, ppidZh, commZh, ticksZh, cmdlineZh, okZh := parseDarwinPsLine(lineZh)
	if !okZh {
		t.Fatalf("expected ok=true for lineZh (zh_CN 4-token format)")
	}
	if pidZh != 12345 || ppidZh != 100 {
		t.Errorf("expected pid=12345, ppid=100; got pid=%d, ppid=%d", pidZh, ppidZh)
	}
	if commZh != "agy" {
		t.Errorf("expected comm='agy', got %q", commZh)
	}
	if ticksZh == 0 {
		t.Errorf("expected non-zero ticks for lineZh")
	}
	if cmdlineZh != "/path/to/agy" {
		t.Errorf("expected cmdline='/path/to/agy', got %q", cmdlineZh)
	}

	// 7. Localized Chinese with multi-word comm
	lineZhMulti := "  777     1 Google Chrome Helper     四 10月/30 01:45:12 2026 /Applications/Google Chrome.app/Contents/MacOS/Google Chrome Helper --type=utility"
	pidZhM, ppidZhM, commZhM, ticksZhM, cmdlineZhM, okZhM := parseDarwinPsLine(lineZhMulti)
	if !okZhM {
		t.Fatalf("expected ok=true for lineZhMulti")
	}
	if pidZhM != 777 || ppidZhM != 1 {
		t.Errorf("expected pid=777, ppid=1; got %d, %d", pidZhM, ppidZhM)
	}
	if commZhM != "Google Chrome Helper" {
		t.Errorf("expected comm='Google Chrome Helper', got %q", commZhM)
	}
	if ticksZhM == 0 {
		t.Errorf("expected non-zero ticks for lineZhMulti")
	}
	if !strings.Contains(cmdlineZhM, "--type=utility") {
		t.Errorf("expected cmdline to contain flag, got %q", cmdlineZhM)
	}

	// 8. Localized Chinese with single-digit day (%b/%e space-padded -> 5 tokens)
	lineZhSingle := " 12345   100 agy                      四 10月/ 5 01:45:12 2026 /path/to/agy"
	pidZhS, ppidZhS, commZhS, ticksZhS, cmdlineZhS, okZhS := parseDarwinPsLine(lineZhSingle)
	if !okZhS {
		t.Fatalf("expected ok=true for lineZhSingle (zh_CN 5-token single-digit day)")
	}
	if pidZhS != 12345 || ppidZhS != 100 {
		t.Errorf("expected pid=12345, ppid=100; got pid=%d, ppid=%d", pidZhS, ppidZhS)
	}
	if commZhS != "agy" {
		t.Errorf("expected comm='agy', got %q", commZhS)
	}
	if ticksZhS == 0 {
		t.Errorf("expected non-zero ticks for lineZhSingle")
	}
	if cmdlineZhS != "/path/to/agy" {
		t.Errorf("expected cmdline='/path/to/agy', got %q", cmdlineZhS)
	}

	// 9. Process with slashes in path should not false-match as Chinese weekday
	linePath := " 88888     1 /usr/local/bin/node /path/to/server 10:00:00 2026 /usr/local/bin/node /path/to/server"
	pidP, _, commP, _, _, okP := parseDarwinPsLine(linePath)
	if !okP {
		t.Fatalf("expected ok=true for linePath")
	}
	if pidP != 88888 {
		t.Errorf("expected pid=88888, got %d", pidP)
	}
	if commP == "" {
		t.Errorf("expected non-empty comm for linePath, got empty string")
	}
}

func TestGetFirstPromptFromTranscript_SystemPromptIgnored(t *testing.T) {
	tempDir := t.TempDir()
	transcriptPath := filepath.Join(tempDir, "transcript.jsonl")

	// Transcript where line 1 is general system instructions, and line 2 contains the actual subagent task
	content := `{"type":"SYSTEM_MESSAGE","content":"You are Antigravity, a powerful agentic AI coding assistant.\nAlways follow instructions."}
{"type":"SYSTEM_MESSAGE","content":"The following is a <SYSTEM_MESSAGE> not actually sent by the user.\n\n<SYSTEM_MESSAGE>\n[Message] timestamp=2026-10-30T00:00:00Z sender=parent priority=MESSAGE_PRIORITY_HIGH content=<original_task>\n**Task**: /boost Refactor process tree\nContext:\n- test\n</original_task>\n</SYSTEM_MESSAGE>"}
`
	if err := os.WriteFile(transcriptPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	title, found := getFirstPromptFromTranscript(transcriptPath, 35)
	if !found {
		t.Fatalf("expected found=true, got false")
	}
	expected := "/boost Refactor process tree"
	if title != expected {
		t.Errorf("expected title %q, got %q (system prompt instructions should be ignored)", expected, title)
	}
}

func TestIsPathInsideCwd_RelativeCwd(t *testing.T) {
	// Test relative cwd handling (e.g. "." or "subdir")
	if !isPathInsideCwd(".", "foo.txt") {
		t.Errorf("expected '.' to contain 'foo.txt'")
	}
	if isPathInsideCwd(".", "../outside.txt") {
		t.Errorf("expected '.' NOT to contain '../outside.txt'")
	}
	if isPathInsideCwd(".", "../../outside.txt") {
		t.Errorf("expected '.' NOT to contain '../../outside.txt'")
	}
}

func TestSplitArgs(t *testing.T) {
	cmd := `"C:/Program Files/title.exe" --ready "my ready" --working 'work hard' --prefix "agy: " --no-prefix`
	tokens := splitArgs(cmd)
	expected := []string{
		"C:/Program Files/title.exe",
		"--ready",
		"my ready",
		"--working",
		"work hard",
		"--prefix",
		"agy: ",
		"--no-prefix",
	}
	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %+v", len(expected), len(tokens), tokens)
	}
	for i, exp := range expected {
		if tokens[i] != exp {
			t.Errorf("token %d expected %q, got %q", i, exp, tokens[i])
		}
	}

	// Empty quoted arguments must not be dropped (prevents argument shifting)
	emptyArgs := splitArgs(`--ready "" --done X`)
	expEmpty := []string{"--ready", "", "--done", "X"}
	if len(emptyArgs) != len(expEmpty) {
		t.Fatalf("expected %d tokens for empty quoted arg, got %d: %+v", len(expEmpty), len(emptyArgs), emptyArgs)
	}
	for i, exp := range expEmpty {
		if emptyArgs[i] != exp {
			t.Errorf("emptyArg token %d expected %q, got %q", i, exp, emptyArgs[i])
		}
	}

	// Backslashes outside quotes must NOT be treated as escape (retains Windows path C:\agy)
	pathArgs := splitArgs(`--prefix C:\agy --dir C:\Users\test`)
	expPath := []string{"--prefix", `C:\agy`, "--dir", `C:\Users\test`}
	if len(pathArgs) != len(expPath) {
		t.Fatalf("expected %d tokens for path args, got %d: %+v", len(expPath), len(pathArgs), pathArgs)
	}
	for i, exp := range expPath {
		if pathArgs[i] != exp {
			t.Errorf("pathArg token %d expected %q, got %q", i, exp, pathArgs[i])
		}
	}
}

func TestParseCommandFlags_QuoteAware(t *testing.T) {
	cmd := `--ready "my ready" --working 'work hard' --waiting "⚠️ wait" --done "✅ done" --text --no-prefix --prefix "custom:" --max-length 50 --task-timeout 120`
	flags := parseCommandFlags(cmd)

	if flags.Ready != "my ready" {
		t.Errorf("expected Ready 'my ready', got %q", flags.Ready)
	}
	if flags.Working != "work hard" {
		t.Errorf("expected Working 'work hard', got %q", flags.Working)
	}
	if flags.Waiting != "⚠️ wait" {
		t.Errorf("expected Waiting '⚠️ wait', got %q", flags.Waiting)
	}
	if flags.Done != "✅ done" {
		t.Errorf("expected Done '✅ done', got %q", flags.Done)
	}
	if flags.UseTextStatus == nil || !*flags.UseTextStatus {
		t.Errorf("expected UseTextStatus true")
	}
	if flags.ShowPrefix == nil || *flags.ShowPrefix {
		t.Errorf("expected ShowPrefix false")
	}
	if flags.Prefix == nil || *flags.Prefix != "custom:" {
		t.Errorf("expected Prefix 'custom:', got %v", flags.Prefix)
	}
	if flags.MaxLength == nil || *flags.MaxLength != 50 {
		t.Errorf("expected MaxLength 50, got %v", flags.MaxLength)
	}
	if flags.TaskTimeout == nil || *flags.TaskTimeout != 120*time.Second {
		t.Errorf("expected TaskTimeout 120s, got %v", flags.TaskTimeout)
	}
}

func TestCleanTitleText(t *testing.T) {
	// \x07 is BEL, \x1b is ESC, \x7f is DEL, \x80 is C1 control
	input := "\x1b[31mRed\x07 \t Title \r\n Test\x7f\x80\x9f"
	cleaned := cleanTitleText(input)
	expected := "[31mRed Title Test"
	if cleaned != expected {
		t.Errorf("expected %q, got %q", expected, cleaned)
	}
}

func TestWorkerMatching_Item8(t *testing.T) {
	// Target: git log -n 5
	pat := parseWorkerTarget("git log -n 5")
	if !pat.valid {
		t.Fatalf("expected pat.valid=true")
	}
	if pat.targetExe != "git" {
		t.Errorf("expected targetExe 'git', got %q", pat.targetExe)
	}
	// "5" and "-n" should be filtered out, leaving "log"
	if len(pat.coreTokens) != 1 || pat.coreTokens[0] != "log" {
		t.Errorf("expected coreTokens ['log'], got %+v", pat.coreTokens)
	}

	// Unrelated process running Start-Sleep -Seconds 5 MUST NOT match
	if matchWorkerProcess("powershell.exe", "powershell -NoProfile -Command Start-Sleep -Seconds 5", pat) {
		t.Errorf("expected Start-Sleep -Seconds 5 NOT to match git log -n 5")
	}
	// Different git command (e.g. git fetch) MUST NOT match
	if matchWorkerProcess("git.exe", "git fetch origin", pat) {
		t.Errorf("expected git fetch NOT to match git log -n 5")
	}
	// Matching git log -n 5 MUST match
	if !matchWorkerProcess("git.exe", "git log -n 5", pat) {
		t.Errorf("expected git log -n 5 to match")
	}

	// Shell wrapper targeting Start-Sleep
	patShell := parseWorkerTarget(`powershell -NoProfile -Command "Start-Sleep -Seconds 5"`)
	if !patShell.valid {
		t.Fatalf("expected patShell.valid=true")
	}
	if !matchWorkerProcess("powershell.exe", `powershell -NoProfile -Command Start-Sleep -Seconds 5`, patShell) {
		t.Errorf("expected powershell running Start-Sleep to match")
	}
	if matchWorkerProcess("powershell.exe", `powershell -NoProfile -Command git status`, patShell) {
		t.Errorf("expected powershell running git status NOT to match Start-Sleep")
	}

	// Shell wrapper spawning external executable: sh -c "git status"
	patShGit := parseWorkerTarget(`sh -c "git status"`)
	if !patShGit.valid {
		t.Fatalf("expected patShGit.valid=true")
	}
	if patShGit.targetExe != "sh" {
		t.Errorf("expected targetExe 'sh', got %q", patShGit.targetExe)
	}
	if patShGit.innerExe != "git" {
		t.Errorf("expected innerExe 'git', got %q", patShGit.innerExe)
	}
	// Child process git.exe running git status MUST match
	if !matchWorkerProcess("git", "git status", patShGit) {
		t.Errorf("expected child process 'git' running 'git status' to match sh -c 'git status'")
	}
	if !matchWorkerProcess("git.exe", "git status", patShGit) {
		t.Errorf("expected child process 'git.exe' running 'git status' to match sh -c 'git status'")
	}
	// Shell itself executing the wrapper MUST also match
	if !matchWorkerProcess("sh", `sh -c "git status"`, patShGit) {
		t.Errorf("expected shell 'sh' running wrapper commandline to match")
	}
	// Unrelated git command (e.g. git fetch) MUST NOT match
	if matchWorkerProcess("git", "git fetch origin", patShGit) {
		t.Errorf("expected git fetch NOT to match sh -c 'git status'")
	}
	// Completely unrelated command (e.g. sleep 10) MUST NOT match
	if matchWorkerProcess("sleep", "sleep 10", patShGit) {
		t.Errorf("expected sleep 10 NOT to match sh -c 'git status'")
	}

	// Shell wrapper with PowerShell spawning git commit
	patPsGit := parseWorkerTarget(`powershell -Command "git commit -m 'Initial commit'"`)
	if !patPsGit.valid {
		t.Fatalf("expected patPsGit.valid=true")
	}
	if patPsGit.innerExe != "git" {
		t.Errorf("expected innerExe 'git', got %q", patPsGit.innerExe)
	}
	if !matchWorkerProcess("git.exe", `git commit -m "Initial commit"`, patPsGit) {
		t.Errorf("expected git.exe running git commit to match powershell -Command git commit")
	}

	// Unix 2-letter command inside shell: bash -c "rm -rf build"
	patRm := parseWorkerTarget(`bash -c "rm -rf build"`)
	if !patRm.valid {
		t.Fatalf("expected patRm.valid=true")
	}
	if patRm.innerExe != "rm" {
		t.Errorf("expected innerExe 'rm', got %q", patRm.innerExe)
	}
	if !matchWorkerProcess("rm", "rm -rf build", patRm) {
		t.Errorf("expected 'rm' process running 'rm -rf build' to match bash -c 'rm -rf build'")
	}

	// Shell wrapper with no payload tokens MUST be invalid
	patEmpty := parseWorkerTarget(`powershell -NoProfile -ExecutionPolicy Bypass -Command ""`)
	if patEmpty.valid {
		t.Errorf("expected shell with no payload tokens to be invalid, got valid=true")
	}
}

func TestResolveState_TableDriven(t *testing.T) {
	symbols := StatusSymbols{
		Ready:   "⚪",
		Working: "⚙️",
		Waiting: "⚠️",
		Done:    "✅",
	}

	dummyWorkerTrue := func(agyPid, currentPid int, targetCmd string) bool { return true }
	dummyWorkerFalse := func(agyPid, currentPid int, targetCmd string) bool { return false }

	tests := []struct {
		name          string
		input         StateInput
		wantIndicator string
		wantMarker    string
	}{
		{
			name: "Thinking state",
			input: StateInput{
				AgentState: "thinking",
				Symbols:    symbols,
			},
			wantIndicator: "⚙️",
			wantMarker:    "RUNNING",
		},
		{
			name: "Working state",
			input: StateInput{
				AgentState: "working",
				Symbols:    symbols,
			},
			wantIndicator: "⚙️",
			wantMarker:    "RUNNING",
		},
		{
			name: "Tool use - ask_question",
			input: StateInput{
				AgentState: "tool_use",
				ToolCalls: []ToolCall{
					{Name: "ask_question", Args: map[string]interface{}{"questions": []string{"Continue?"}}},
				},
				Symbols: symbols,
			},
			wantIndicator: "⚠️",
			wantMarker:    "RUNNING",
		},
		{
			name: "Tool use - prompt_user",
			input: StateInput{
				AgentState: "tool_use",
				ToolCalls: []ToolCall{
					{Name: "prompt_user", Args: nil},
				},
				Symbols: symbols,
			},
			wantIndicator: "⚠️",
			wantMarker:    "RUNNING",
		},
		{
			name: "Tool use - run_command in ask rule, worker inactive",
			input: StateInput{
				AgentState: "tool_use",
				ToolCalls: []ToolCall{
					{Name: "run_command", Args: map[string]interface{}{"CommandLine": "rm -rf build"}},
				},
				Perms: &Permissions{
					Ask: []string{"command(rm *)"},
				},
				AgyPid:        1234,
				HasWorkerFunc: dummyWorkerFalse,
				Symbols:       symbols,
			},
			wantIndicator: "⚠️",
			wantMarker:    "RUNNING",
		},
		{
			name: "Tool use - run_command in ask rule, worker active",
			input: StateInput{
				AgentState: "tool_use",
				ToolCalls: []ToolCall{
					{Name: "run_command", Args: map[string]interface{}{"CommandLine": "rm -rf build"}},
				},
				Perms: &Permissions{
					Ask: []string{"command(rm *)"},
				},
				AgyPid:        1234,
				HasWorkerFunc: dummyWorkerTrue,
				Symbols:       symbols,
			},
			wantIndicator: "⚙️",
			wantMarker:    "RUNNING",
		},
		{
			name: "Tool use - run_command allowed exact match",
			input: StateInput{
				AgentState: "tool_use",
				ToolCalls: []ToolCall{
					{Name: "run_command", Args: map[string]interface{}{"CommandLine": "git status"}},
				},
				Perms: &Permissions{
					Allow: []string{"command(git status)"},
				},
				AgyPid:        1234,
				HasWorkerFunc: dummyWorkerFalse,
				Symbols:       symbols,
			},
			wantIndicator: "⚙️",
			wantMarker:    "RUNNING",
		},
		{
			name: "Tool use - run_command command concatenation injection attempt blocked (Item 9)",
			input: StateInput{
				AgentState: "tool_use",
				ToolCalls: []ToolCall{
					{Name: "run_command", Args: map[string]interface{}{"CommandLine": "rm -rf /; git status"}},
				},
				Perms: &Permissions{
					Allow: []string{"command(git status)"},
				},
				AgyPid:        1234,
				HasWorkerFunc: dummyWorkerFalse,
				Symbols:       symbols,
			},
			wantIndicator: "⚠️",
			wantMarker:    "RUNNING",
		},
		{
			name: "Tool use - run_command with run_command(*) syntax support (Item 10)",
			input: StateInput{
				AgentState: "tool_use",
				ToolCalls: []ToolCall{
					{Name: "run_command", Args: map[string]interface{}{"CommandLine": "go test ./..."}},
				},
				Perms: &Permissions{
					Allow: []string{"run_command(go test *)"},
				},
				AgyPid:        1234,
				HasWorkerFunc: dummyWorkerFalse,
				Symbols:       symbols,
			},
			wantIndicator: "⚙️",
			wantMarker:    "RUNNING",
		},
		{
			name: "Tool use - nil permissions baseline semantics (Item 10) with worker inactive",
			input: StateInput{
				AgentState: "tool_use",
				ToolCalls: []ToolCall{
					{Name: "run_command", Args: map[string]interface{}{"CommandLine": "git diff"}},
				},
				Perms:         nil, // unconfigured permissions block
				AgyPid:        1234,
				HasWorkerFunc: dummyWorkerFalse,
				Symbols:       symbols,
			},
			wantIndicator: "⚠️",
			wantMarker:    "RUNNING",
		},
		{
			name: "Tool use - nil permissions baseline semantics with worker active",
			input: StateInput{
				AgentState: "tool_use",
				ToolCalls: []ToolCall{
					{Name: "run_command", Args: map[string]interface{}{"CommandLine": "git diff"}},
				},
				Perms:         nil,
				AgyPid:        1234,
				HasWorkerFunc: dummyWorkerTrue,
				Symbols:       symbols,
			},
			wantIndicator: "⚙️",
			wantMarker:    "RUNNING",
		},
		{
			name: "Tool use - multi-tool batch where second call is ask_question",
			input: StateInput{
				AgentState: "tool_use",
				ToolCalls: []ToolCall{
					{Name: "run_command", Args: map[string]interface{}{"CommandLine": "git status"}},
					{Name: "ask_question", Args: map[string]interface{}{"prompt": "ok?"}},
				},
				Perms: &Permissions{
					Allow: []string{"command(*)"},
				},
				AgyPid:        1234,
				HasWorkerFunc: dummyWorkerTrue,
				Symbols:       symbols,
			},
			wantIndicator: "⚠️",
			wantMarker:    "RUNNING",
		},
		{
			name: "Tool use - sensitive file write outside cwd",
			input: StateInput{
				AgentState: "tool_use",
				ToolCalls: []ToolCall{
					{Name: "write_file", Args: map[string]interface{}{"TargetFile": "/etc/passwd"}},
				},
				Cwd:     "/home/user/project",
				Symbols: symbols,
			},
			wantIndicator: "⚠️",
			wantMarker:    "RUNNING",
		},
		{
			name: "Tool use - sensitive file write inside cwd without ask rule",
			input: StateInput{
				AgentState: "tool_use",
				ToolCalls: []ToolCall{
					{Name: "write_file", Args: map[string]interface{}{"TargetFile": "/home/user/project/main.go"}},
				},
				Cwd:     "/home/user/project",
				Symbols: symbols,
			},
			wantIndicator: "⚙️",
			wantMarker:    "RUNNING",
		},
		{
			name: "Idle state - active subagent running",
			input: StateInput{
				AgentState:      "idle",
				ActiveSubagents: 1,
				Symbols:         symbols,
			},
			wantIndicator: "⚙️",
			wantMarker:    "RUNNING",
		},
		{
			name: "Idle state - active background tasks running",
			input: StateInput{
				AgentState:       "idle",
				HasActiveBgTasks: true,
				Symbols:          symbols,
			},
			wantIndicator: "⚙️",
			wantMarker:    "RUNNING",
		},
		{
			name: "Idle state - completed turn with valid matching marker",
			input: StateInput{
				AgentState:    "idle",
				HasHistory:    true,
				MarkerExists:  true,
				MarkerContent: "5000:123456789:RUNNING",
				AgyPid:        5000,
				AgyTicks:      123456789,
				Symbols:       symbols,
			},
			wantIndicator: "✅",
			wantMarker:    "COMPLETED",
		},
		{
			name: "Idle state - reopen old session (dead PID in marker)",
			input: StateInput{
				AgentState:    "idle",
				HasHistory:    true,
				MarkerExists:  true,
				MarkerContent: "9999:999999999:COMPLETED",
				AgyPid:        5000, // different agy PID
				AgyTicks:      123456789,
				Symbols:       symbols,
			},
			wantIndicator: "⚪",
			wantMarker:    "",
		},
		{
			name: "Idle state - fresh workspace without history",
			input: StateInput{
				AgentState:   "idle",
				HasHistory:   false,
				MarkerExists: false,
				AgyPid:       5000,
				Symbols:      symbols,
			},
			wantIndicator: "⚪",
			wantMarker:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := resolveState(tt.input)
			if res.StatusIndicator != tt.wantIndicator {
				t.Errorf("resolveState() indicator = %q, want %q", res.StatusIndicator, tt.wantIndicator)
			}
			if res.MarkerAction != tt.wantMarker {
				t.Errorf("resolveState() markerAction = %q, want %q", res.MarkerAction, tt.wantMarker)
			}
		})
	}
}

func TestWorkerDetection_FallbackAndCmdlets(t *testing.T) {
	tests := []struct {
		name      string
		procExe   string
		cmdline   string
		targetCmd string
		wantMatch bool
	}{
		{
			name:      "PowerShell cmdlet Start-Sleep",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "Start-Sleep -Seconds 8"`,
			targetCmd: "Start-Sleep -Seconds 8",
			wantMatch: true,
		},
		{
			name:      "Compound semicolon command in PowerShell host",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -Command "cd C:\Windows; ping 127.0.0.1 -n 8"`,
			targetCmd: `cd C:\Windows; ping 127.0.0.1 -n 8`,
			wantMatch: true,
		},
		{
			name:      "Pipeline command matching shell parent",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -Command "ping 127.0.0.1 -n 8 | Select-String TTL"`,
			targetCmd: `ping 127.0.0.1 -n 8 | Select-String TTL`,
			wantMatch: true,
		},
		{
			name:      "Pipeline command first statement matching ping.exe child",
			procExe:   "ping.exe",
			cmdline:   `ping.exe 127.0.0.1 -n 8`,
			targetCmd: `ping 127.0.0.1 -n 8 | Select-String TTL`,
			wantMatch: true,
		},
		{
			name:      "npm run build running in cmd wrapper",
			procExe:   "cmd.exe",
			cmdline:   `cmd.exe /c "npm run build"`,
			targetCmd: "npm run build",
			wantMatch: true,
		},
		{
			name:      "mvn test with env set",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -Command "$env:JAVA_HOME='D:\Develop\Java'; mvn test"`,
			targetCmd: `$env:JAVA_HOME='D:\Develop\Java'; mvn test`,
			wantMatch: true,
		},
		{
			name:      "PowerShell cmdlet Get-Content",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -NoProfile -Command "Get-Content -Path ./data.json -Raw"`,
			targetCmd: "Get-Content -Path ./data.json -Raw",
			wantMatch: true,
		},
		{
			name:      "adb logcat in powershell pipeline",
			procExe:   "adb.exe",
			cmdline:   `adb.exe logcat -d`,
			targetCmd: `adb logcat -d | Select-String "Error"`,
			wantMatch: true,
		},
		{
			name:      "Negative: Unrelated command 'git status' against running Start-Sleep",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -NoProfile -Command "Start-Sleep -Seconds 8"`,
			targetCmd: "git status",
			wantMatch: false,
		},
		{
			name:      "Negative: Empty target command",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -NoProfile -Command "Start-Sleep -Seconds 8"`,
			targetCmd: "",
			wantMatch: false,
		},
		{
			name:      "Negative: Pure shell noise without command",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ""`,
			targetCmd: `powershell -NoProfile -ExecutionPolicy Bypass -Command ""`,
			wantMatch: false,
		},
		{
			name:      "Single-word cmdlet Start-Sleep without arguments",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -NoProfile -Command "Start-Sleep"`,
			targetCmd: "Start-Sleep",
			wantMatch: true,
		},
		{
			name:      "Negative: Unrelated process matching single-word statement 'dir'",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -Command "npm run build --dir dist"`,
			targetCmd: `rm -rf /tmp/foo; dir`,
			wantMatch: false,
		},
		{
			name:      "Negative: Unrelated cmdlet 'Get-Process' against running Start-Sleep",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -NoProfile -Command "Start-Sleep -Seconds 8"`,
			targetCmd: "Get-Process",
			wantMatch: false,
		},
		{
			name:      "Negative: Prep statement cd with different command",
			procExe:   "powershell.exe",
			cmdline:   `cd D:\proj; npm run dev`,
			targetCmd: `cd D:\proj; npm test`,
			wantMatch: false,
		},
		{
			name:      "Negative: Prep statement Set-Location with different command",
			procExe:   "powershell.exe",
			cmdline:   `Set-Location D:\proj; npm run dev`,
			targetCmd: `Set-Location D:\proj; Remove-Item -Recurse dist`,
			wantMatch: false,
		},
		{
			name:      "Negative: Prefix number Start-Sleep 30 vs 300",
			procExe:   "powershell.exe",
			cmdline:   `Start-Sleep -Seconds 300`,
			targetCmd: `Start-Sleep -Seconds 30`,
			wantMatch: false,
		},
		{
			name:      "Negative: Prefix number ping -n 8 vs 80",
			procExe:   "powershell.exe",
			cmdline:   `ping 127.0.0.1 -n 80`,
			targetCmd: `ping 127.0.0.1 -n 8`,
			wantMatch: false,
		},
		{
			name:      "Negative: Pipeline recipient Remove-Item vs pipeline Get-ChildItem | Remove-Item",
			procExe:   "powershell.exe",
			cmdline:   `Get-ChildItem | Remove-Item`,
			targetCmd: `Remove-Item`,
			wantMatch: false,
		},
		{
			name:      "Negative: Path containing command name with backslash",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -Command "python C:\git\repo\main.py"`,
			targetCmd: `git`,
			wantMatch: false,
		},
		{
			name:      "Negative: Path containing command name with slash",
			procExe:   "bash",
			cmdline:   `/usr/bin/python /app/test/server.py`,
			targetCmd: `test`,
			wantMatch: false,
		},
		{
			name:      "Prep statement cd.. with target command",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -Command "cd..; npm test"`,
			targetCmd: `cd..; npm test`,
			wantMatch: true,
		},
		{
			name:      "Item 5: False positive ping -n 80 vs -n 8 under ping.exe",
			procExe:   "ping.exe",
			cmdline:   `ping.exe 8.8.8.8 -n 80`,
			targetCmd: `ping 8.8.8.8 -n 8`,
			wantMatch: false,
		},
		{
			name:      "Item 5: True positive ping -n 8 under ping.exe",
			procExe:   "ping.exe",
			cmdline:   `ping.exe 8.8.8.8 -n 8`,
			targetCmd: `ping 8.8.8.8 -n 8`,
			wantMatch: true,
		},
		{
			name:      "Item 5: False positive git push vs git push --force under git.exe",
			procExe:   "git.exe",
			cmdline:   `git.exe push origin main`,
			targetCmd: `git push --force origin main`,
			wantMatch: false,
		},
		{
			name:      "Item 5: True positive git push --force under git.exe",
			procExe:   "git.exe",
			cmdline:   `git.exe push --force origin main`,
			targetCmd: `git push --force origin main`,
			wantMatch: true,
		},
		{
			name:      "Item 5: False positive cd D:\\a vs cd D:\\b with npm test (shell)",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -Command "cd D:\a; npm test"`,
			targetCmd: `cd D:\b; npm test`,
			wantMatch: false,
		},
		{
			name:      "Item 5: False positive cd D:\\a vs cd D:\\b with npm test (child node)",
			procExe:   "node.exe",
			cmdline:   `npm test`,
			targetCmd: `cd D:\b; npm test`,
			wantMatch: false,
		},
		{
			name:      "Item 5: True positive cd D:\\b; npm test (shell)",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -Command "cd D:\b; npm test"`,
			targetCmd: `cd D:\b; npm test`,
			wantMatch: true,
		},
		{
			name:      "Review Item 3: False positive git push origin main vs running git push origin main --force under git.exe",
			procExe:   "git.exe",
			cmdline:   `git.exe push origin main --force`,
			targetCmd: `git push origin main`,
			wantMatch: false,
		},
		{
			name:      "Review Item 3: False positive git push origin main vs running git push origin main --force under powershell.exe",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -Command "git push origin main --force"`,
			targetCmd: `git push origin main`,
			wantMatch: false,
		},
		{
			name:      "Review Item 3: False positive npm run dev vs running npm run dev -- --port 3001",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -Command "npm run dev -- --port 3001"`,
			targetCmd: `npm run dev`,
			wantMatch: false,
		},
		{
			name:      "Review Item 3: True positive git push origin main under git.exe",
			procExe:   "git.exe",
			cmdline:   `git.exe push origin main`,
			targetCmd: `git push origin main`,
			wantMatch: true,
		},
		{
			name:      "Review Item 4: Substring dir confusion cd D:\\proj vs cd D:\\proj-old",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -Command "cd D:\proj-old; npm test"`,
			targetCmd: `cd D:\proj; npm test`,
			wantMatch: false,
		},
		{
			name:      "Review Item 4: Substring dir confusion cd src vs cd D:\\other\\src",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -Command "cd D:\other\src; npm test"`,
			targetCmd: `cd src; npm test`,
			wantMatch: false,
		},
		{
			name:      "Review Item 4: True positive cd D:\\proj matching cd D:\\proj\\",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -Command "cd D:\proj\; npm test"`,
			targetCmd: `cd D:\proj; npm test`,
			wantMatch: true,
		},
		{
			name:      "Review Item 4: True positive cd D:\\proj matching quoted cd \"D:\\proj\"",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -Command "cd \"D:\proj\"; npm test"`,
			targetCmd: `cd D:\proj; npm test`,
			wantMatch: true,
		},
		{
			name:      "Review Item 4: True positive extractDir cd /d d:\\proj && npm test",
			procExe:   "cmd.exe",
			cmdline:   `cmd.exe /c "cd /d d:\proj && npm test"`,
			targetCmd: `cd /d d:\proj && npm test`,
			wantMatch: true,
		},
		{
			name:      "Review Item 4: True positive extractDir set-location -path d:\\proj",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -Command "set-location -path d:\proj; npm test"`,
			targetCmd: `set-location -path d:\proj; npm test`,
			wantMatch: true,
		},
		{
			name:      "Review Item 5: False positive MCP server peer process node.exe matching node index.js",
			procExe:   "node.exe",
			cmdline:   `node.exe C:\Users\user\.gemini\antigravity-cli\mcp\server-filesystem\dist\index.js D:\proj`,
			targetCmd: `node index.js`,
			wantMatch: false,
		},
		{
			name:      "Review Item 5: True positive node.exe index.js matching node index.js",
			procExe:   "node.exe",
			cmdline:   `node.exe index.js`,
			targetCmd: `node index.js`,
			wantMatch: true,
		},
		{
			name:      "Review Item 5: True positive node.exe \"index.js\" matching node index.js",
			procExe:   "node.exe",
			cmdline:   `node.exe "index.js"`,
			targetCmd: `node index.js`,
			wantMatch: true,
		},
		{
			name:      "Review Item 5: False positive MCP server peer process matching python main.py",
			procExe:   "python.exe",
			cmdline:   `python.exe C:\mcp\server\main.py`,
			targetCmd: `python main.py`,
			wantMatch: false,
		},
		{
			name:      "Review Item 5: True positive python.exe main.py matching python main.py",
			procExe:   "python.exe",
			cmdline:   `python.exe main.py`,
			targetCmd: `python main.py`,
			wantMatch: true,
		},
		{
			name:      "Review Item 5: False positive MCP server running with arguments vs bare node",
			procExe:   "node.exe",
			cmdline:   `node.exe C:\Users\user\.gemini\antigravity-cli\mcp\server-filesystem\dist\index.js D:\proj`,
			targetCmd: `node`,
			wantMatch: false,
		},
		{
			name:      "Review Item 5: True positive bare node without arguments",
			procExe:   "node.exe",
			cmdline:   `node.exe`,
			targetCmd: `node`,
			wantMatch: true,
		},
		{
			name:      "Review Item 5: False positive background server vs bare python",
			procExe:   "python.exe",
			cmdline:   `python.exe -m http.server 8080`,
			targetCmd: `python`,
			wantMatch: false,
		},
		{
			name:      "Review Item 5: True positive bare python without arguments",
			procExe:   "python.exe",
			cmdline:   `python.exe`,
			targetCmd: `python`,
			wantMatch: true,
		},
		{
			name:      "Review Item 4: True positive extractDir with ampersand inside quotes",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -Command "cd 'D:\foo & bar'; npm test"`,
			targetCmd: `cd "D:\foo & bar" && npm test`,
			wantMatch: true,
		},
		{
			name:      "Review Item 4: True positive parenthesized directory statement",
			procExe:   "powershell.exe",
			cmdline:   `powershell.exe -Command "(cd D:\proj; npm test)"`,
			targetCmd: `cd D:\proj; npm test`,
			wantMatch: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pat := parseWorkerTarget(tc.targetCmd)
			got := matchWorkerProcess(tc.procExe, tc.cmdline, pat)
			if got != tc.wantMatch {
				t.Errorf("matchWorkerProcess(%q, %q, %q) = %v; want %v (pat: %+v)",
					tc.procExe, tc.cmdline, tc.targetCmd, got, tc.wantMatch, pat)
			}
		})
	}
}

func TestHotReloadFlagReset(t *testing.T) {
	// Baseline: agy launched with --no-prefix and --text cached in CLI arguments
	cliArgs := []string{"--no-prefix", "--text"}

	// Step 1: user removes --no-prefix and --text from settings.json command
	titleCfg := &TitleConfig{
		Command: "C:/path/to/title.exe",
	}

	cfg := resolveRuntimeConfig(cliArgs, titleCfg)

	if !cfg.ShowPrefix {
		t.Errorf("expected ShowPrefix to be reset to true after removing --no-prefix, got false")
	}
	if cfg.UseTextStatus {
		t.Errorf("expected UseTextStatus to be reset to false after removing --text, got true")
	}
}

func TestPrefixFlagValueHelpOrVersion(t *testing.T) {
	// Tests Item 7: ensure --prefix help or --prefix version does not get swallowed as a help/version command
	argsWithHelpVal := []string{"--prefix", "help"}
	cfg := resolveRuntimeConfig(argsWithHelpVal, nil)
	if cfg.CustomPrefix != "help" {
		t.Errorf("expected CustomPrefix to be 'help', got %q", cfg.CustomPrefix)
	}

	argsWithVersionVal := []string{"--prefix", "version"}
	cfg2 := resolveRuntimeConfig(argsWithVersionVal, nil)
	if cfg2.CustomPrefix != "version" {
		t.Errorf("expected CustomPrefix to be 'version', got %q", cfg2.CustomPrefix)
	}

	// Verify checkHelpOrVersion correctly ignores flag values that happen to be "help" or "version"
	testCases := []struct {
		args        []string
		wantHelp    bool
		wantVersion bool
	}{
		{args: []string{"--prefix", "help"}, wantHelp: false, wantVersion: false},
		{args: []string{"--prefix", "version"}, wantHelp: false, wantVersion: false},
		{args: []string{"--ready", "help"}, wantHelp: false, wantVersion: false},
		{args: []string{"--working", "version"}, wantHelp: false, wantVersion: false},
		{args: []string{"--prefix", "-h"}, wantHelp: false, wantVersion: false},
		{args: []string{"--prefix", "--version"}, wantHelp: false, wantVersion: false},
		{args: []string{"help"}, wantHelp: false, wantVersion: false},
		{args: []string{"version"}, wantHelp: false, wantVersion: false},
		{args: []string{"-h"}, wantHelp: true, wantVersion: false},
		{args: []string{"--help"}, wantHelp: true, wantVersion: false},
		{args: []string{"-v"}, wantHelp: false, wantVersion: true},
		{args: []string{"--version"}, wantHelp: false, wantVersion: true},
		{args: []string{"--prefix", "custom", "--help"}, wantHelp: true, wantVersion: false},
		{args: []string{"--prefix", "custom", "-v"}, wantHelp: false, wantVersion: true},
	}

	for _, tc := range testCases {
		gotHelp, gotVersion := checkHelpOrVersion(tc.args)
		if gotHelp != tc.wantHelp || gotVersion != tc.wantVersion {
			t.Errorf("checkHelpOrVersion(%v) = (%v, %v); want (%v, %v)",
				tc.args, gotHelp, gotVersion, tc.wantHelp, tc.wantVersion)
		}
	}
}

func TestSplitCommandStatements_Unicode(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{
			input:    "echo 正在构建项目; ping 127.0.0.1",
			expected: []string{"echo 正在构建项目", "ping 127.0.0.1"},
		},
		{
			input:    "echo 测试 && echo 完成",
			expected: []string{"echo 测试", "echo 完成"},
		},
		{
			input:    `powershell -Command "echo 编译中..." | findstr 成功`,
			expected: []string{`powershell -Command "echo 编译中..."`, "findstr 成功"},
		},
		{
			input:    `git commit -m "feat: 支持中文与特殊字符 ✨" && git push`,
			expected: []string{`git commit -m "feat: 支持中文与特殊字符 ✨"`, "git push"},
		},
	}

	for _, tc := range tests {
		got := splitCommandStatements(tc.input)
		if len(got) != len(tc.expected) {
			t.Fatalf("splitCommandStatements(%q) returned %d stmts; want %d: %v", tc.input, len(got), len(tc.expected), got)
		}
		for i := range got {
			if got[i] != tc.expected[i] {
				t.Errorf("splitCommandStatements(%q)[%d] = %q; want %q", tc.input, i, got[i], tc.expected[i])
			}
		}
	}

	// Verify target command matching with Chinese commandline
	pat := parseWorkerTarget("echo 正在构建; ping 127.0.0.1")
	if !pat.valid {
		t.Fatalf("expected pat.valid to be true")
	}
}

func TestExtractDir_PushLocation(t *testing.T) {
	testCmds := []string{
		`Push-Location 'D:\Projects\Test'; git status`,
		`push-location "D:\Projects\Test" && git status`,
		`pushd D:\Projects\Test && git status`,
		`cd D:\Projects\Test && git status`,
	}
	for _, cmd := range testCmds {
		pat := parseWorkerTarget(cmd)
		if !pat.valid {
			t.Fatalf("parseWorkerTarget(%q) not valid", cmd)
		}
		cleanDir := strings.Trim(pat.requiredDir, `"'`)
		if cleanDir != `d:\projects\test` {
			t.Errorf("expected requiredDir for %q to be %q, got %q", cmd, `d:\projects\test`, pat.requiredDir)
		}
		if pat.targetExe != "git" {
			t.Errorf("expected targetExe for %q to be 'git', got %q", cmd, pat.targetExe)
		}
		shellCmdline := `powershell.exe -Command "` + cmd + `"`
		if !matchWorkerProcess("powershell.exe", shellCmdline, pat) {
			t.Errorf("expected matchWorkerProcess to match shell process for %q", cmd)
		}
	}
}
