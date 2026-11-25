package main

import (
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

type commandStatement struct {
	Text  string
	Piped bool
}

var workerSetupAssignment = regexp.MustCompile(`(?i)^\$(?:(?:global|script):)?(?:ProgressPreference|ErrorActionPreference|OutputEncoding|PSNativeCommandUseErrorActionPreference)\s*=`)

func resolveWorkerDir(cwd, dir string) string {
	if filepath.IsAbs(dir) {
		return filepath.Clean(dir)
	}
	if runtime.GOOS == "windows" && strings.HasPrefix(dir, `\`) {
		return filepath.Join(filepath.VolumeName(cwd)+`\`, strings.TrimLeft(dir, `\/`))
	}
	return filepath.Clean(filepath.Join(cwd, dir))
}

// Extract shell payloads only at argument boundaries. Paths and --config are not -c.
func shellPayload(command string) (string, bool) {
	args := splitArgs(command)
	for i, arg := range args {
		switch strings.ToLower(arg) {
		case "-command", "--command", "-c", "/c":
			if i+1 < len(args) {
				return strings.Join(args[i+1:], " "), true
			}
		}
	}
	return "", false
}

func commandBase(s string) string {
	s = strings.ReplaceAll(strings.ToLower(s), "\\", "/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	return strings.TrimSuffix(s, ".exe")
}

func executableStatements(command string) []string {
	command = strings.TrimSpace(command)
	if strings.HasPrefix(command, "(") && strings.HasSuffix(command, ")") {
		command = strings.TrimSpace(command[1 : len(command)-1])
	}
	var result []string
	for _, part := range splitCommandParts(command) {
		stmt := part.Text
		if part.Piped {
			continue
		}
		args := splitArgs(stmt)
		if len(args) == 0 || isPrepStatement(stmt) || args[0] == "." || workerSetupAssignment.MatchString(stmt) {
			continue
		}
		switch commandBase(args[0]) {
		case "powershell", "pwsh", "cmd", "bash", "sh", "zsh":
			if payload, ok := shellPayload(stmt); ok {
				result = append(result, executableStatements(payload)...)
			}
		default:
			result = append(result, stmt)
		}
	}
	return result
}

func sameCommand(a, b string) bool {
	aa, bb := splitArgs(a), splitArgs(b)
	if len(aa) == 0 || len(aa) != len(bb) || commandBase(aa[0]) != commandBase(bb[0]) {
		return false
	}
	for i := 1; i < len(aa); i++ {
		if !strings.EqualFold(aa[i], bb[i]) {
			return false
		}
	}
	return true
}
