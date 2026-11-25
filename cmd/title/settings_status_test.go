package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsReadStatus(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	for _, tc := range []struct {
		name, content    string
		invalid, enabled bool
	}{
		{"empty", "", true, false},
		{"incomplete", `{"title":`, true, false},
		{"null", "null", true, false},
		{"array", "[]", true, false},
		{"bad_title", `{"title":"invalid"}`, true, false},
		{"removed", "{}", false, false},
		{"disabled", `{"title":{"command":"title --animate","enabled":false}}`, false, false},
		{"enabled", `{"title":{"command":"title --animate","enabled":true}}`, false, true},
		{"bad_permissions", `{"title":{"command":"title --animate"},"permissions":"bad"}`, false, true},
		{"bom", "\xef\xbb\xbf" + `{"title":{"command":"title --animate"}}`, false, true},
		{"too_large", "{}" + strings.Repeat(" ", 1<<20), true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(p, []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, _, err := readSettingsWithStatus(p)
			if (err != nil) != tc.invalid {
				t.Fatalf("error=%v want invalid=%v", err, tc.invalid)
			}
			if isTitleCommand(cfg) != tc.enabled {
				t.Fatal("configuration enablement changed", cfg)
			}
		})
	}
	if _, _, err := readSettingsWithStatus(filepath.Join(t.TempDir(), "missing.json")); !os.IsNotExist(err) {
		t.Fatal("missing-file classification lost", err)
	}
	// A malformed title must not discard independently valid permission rules.
	if err := os.WriteFile(p, []byte(`{"title":"bad","permissions":{"ask":["command(*)"]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, perms := readSettings(p)
	if cfg != nil || perms == nil || len(perms.Ask) != 1 {
		t.Fatal(cfg, perms)
	}
}
