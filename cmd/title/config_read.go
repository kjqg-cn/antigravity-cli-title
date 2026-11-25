package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func readBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}

func readSettings(path string) (*TitleConfig, *Permissions) {
	title, perms, _ := readSettingsWithStatus(path)
	return title, perms
}

// Separate invalid/incomplete writes from a valid configuration disabling title.
// Callers that animate may retry invalid input at the next bounded settings poll.
func readSettingsWithStatus(path string) (*TitleConfig, *Permissions, error) {
	b, err := readBounded(path, (1<<20)+1)
	if err != nil {
		return nil, nil, err
	}
	if len(b) > 1<<20 {
		return nil, nil, errors.New("settings exceeds read budget")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(cleanStdin(b), &fields) != nil || fields == nil {
		return nil, nil, errors.New("settings is not a complete JSON object")
	}
	var title *TitleConfig
	var perms *Permissions
	if json.Unmarshal(fields["permissions"], &perms) != nil {
		perms = nil
	}
	if raw, ok := fields["title"]; ok && json.Unmarshal(raw, &title) != nil {
		return nil, perms, errors.New("invalid title configuration")
	}
	var enabled struct {
		Enabled *bool `json:"enabled"`
	}
	if json.Unmarshal(fields["title"], &enabled) == nil && enabled.Enabled != nil && !*enabled.Enabled {
		title = nil
	}
	return title, perms, nil
}

func isTitleCommand(cfg *TitleConfig) bool {
	if cfg == nil {
		return false
	}
	tokens := splitArgs(cfg.Command)
	if len(tokens) == 0 {
		return false
	}
	base := strings.ToLower(filepath.Base(tokens[0]))
	return base == "title" || base == "title.exe"
}

func expandFlagEquals(args []string) []string {
	var result []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch strings.ToLower(arg) {
		case "--prefix", "--max-length", "--task-timeout", "--ready", "--ready-symbol", "--working", "--working-symbol", "--waiting", "--waiting-symbol", "--done", "--done-symbol":
			result = append(result, arg)
			if i+1 < len(args) {
				i++
				result = append(result, args[i])
			}
			continue
		}
		key, value, ok := strings.Cut(arg, "=")
		if ok {
			switch strings.ToLower(key) {
			case "--animate":
				if strings.EqualFold(value, "true") {
					result = append(result, "--animate")
					continue
				}
				if strings.EqualFold(value, "false") {
					result = append(result, "--no-animate")
					continue
				}
			case "--prefix", "--max-length", "--task-timeout", "--ready", "--ready-symbol", "--working", "--working-symbol", "--waiting", "--waiting-symbol", "--done", "--done-symbol":
				result = append(result, key, value)
				continue
			}
		}
		result = append(result, arg)
	}
	return result
}

func settingsArgument(command, cwd, defaultPath string) string {
	args := splitArgs(command)
	for i := 1; i < len(args); i++ {
		arg := args[i]
		var path string
		if strings.EqualFold(arg, "--settings") && i+1 < len(args) {
			path = args[i+1]
		} else if key, value, ok := strings.Cut(arg, "="); ok && strings.EqualFold(key, "--settings") {
			path = value
		}
		if path != "" {
			if filepath.IsAbs(path) {
				return filepath.Clean(path)
			}
			if cwd != "" {
				return resolveWorkerDir(cwd, path)
			}
		}
	}
	return defaultPath
}
