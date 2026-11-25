//go:build windows

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProcessDirectoryAndWow64(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got := getProcessCwd(os.Getpid()); !strings.EqualFold(filepath.Clean(got), cwd) {
		t.Fatal("native cwd", got, cwd)
	}
	x86 := filepath.Join(os.Getenv("WINDIR"), "SysWOW64", "cmd.exe")
	if _, err := os.Stat(x86); err != nil {
		t.Skip("WoW64 executable unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, x86, "/c", "ping -n 2 127.0.0.1 >nul")
	cmd.Dir = t.TempDir()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Wait()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		line, dir := getCommandLine(cmd.Process.Pid), getProcessCwd(cmd.Process.Pid)
		if strings.Contains(line, "ping -n 2") && strings.EqualFold(filepath.Clean(dir), cmd.Dir) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("WoW64 command line or cwd was unreadable")
}

func TestDriveRelativeFileToolsRemainWaiting(t *testing.T) {
	symbols := resolveStatusSymbols(true, "", "", "", "")
	for _, cwd := range []string{`C:\workspace`, `D:\workspace`} {
		for _, target := range []string{`C:foo.txt`, `D:foo.txt`, `D:..\secret.txt`} {
			for _, tool := range []string{"write_file", "edit_file", "multi_replace_file_content", "read_file"} {
				result := resolveState(StateInput{
					AgentState: "tool_use", Cwd: cwd, Symbols: symbols,
					ToolCalls: []ToolCall{{Name: tool, Args: map[string]interface{}{"Path": target}}},
				})
				if result.StatusIndicator != symbols.Waiting {
					t.Errorf("%s cwd=%s target=%s: got %s", tool, cwd, target, result.StatusIndicator)
				}
			}
		}
		for _, target := range []string{"file.txt", filepath.Join(cwd, "file.txt")} {
			result := resolveState(StateInput{
				AgentState: "tool_use", Cwd: cwd, Symbols: symbols,
				ToolCalls: []ToolCall{{Name: "write_file", Args: map[string]interface{}{"Path": target}}},
			})
			if result.StatusIndicator != symbols.Working {
				t.Errorf("valid workspace path %s: got %s", target, result.StatusIndicator)
			}
		}
	}
}
