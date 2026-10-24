package main

import (
	"regexp"
	"testing"
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
