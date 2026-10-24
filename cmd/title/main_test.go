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
