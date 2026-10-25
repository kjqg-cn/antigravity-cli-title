package main

import (
	"os"
	"path/filepath"
	"regexp"
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

func TestUnescapeCommand(t *testing.T) {
	raw := `echo \"hello\nworld\"`
	expected := "echo \"hello\nworld\""
	got := unescapeCommand(raw)
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

	// 2. CLI flags override config
	s2 := resolveConfiguredSymbols(false, "'⚪'", "", "⚠️", "", cfg)
	if s2.Ready != "⚪" {
		t.Errorf("Expected CLI override for Ready ('⚪' -> '⚪'), got: %s", s2.Ready)
	}
	if s2.Working != "🛠️" {
		t.Errorf("Expected config value for Working, got: %s", s2.Working)
	}
	if s2.Waiting != "⚠️" {
		t.Errorf("Expected CLI override for Waiting, got: %s", s2.Waiting)
	}
	// 3. Command string hot-reload test (when user edits command field directly in settings.json)
	cfgCmd := &TitleConfig{
		Command: "C:/path/title.exe --no-prefix --working ⚡ --done 🎉",
		Waiting: "🔔", // direct key overrides
	}
	s3 := resolveConfiguredSymbols(false, "", "", "", "", cfgCmd)
	if s3.Working != "⚡" {
		t.Errorf("Expected Working to be parsed from Command string, got: %s", s3.Working)
	}
	if s3.Done != "🎉" {
		t.Errorf("Expected Done to be parsed from Command string, got: %s", s3.Done)
	}
	if s3.Waiting != "🔔" {
		t.Errorf("Expected Waiting to be direct key override, got: %s", s3.Waiting)
	}
	if s3.Ready != "⚪" {
		t.Errorf("Expected Ready to fallback to default, got: %s", s3.Ready)
	}
}
