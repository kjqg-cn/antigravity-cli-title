//go:build windows

package main

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestHasActiveWorker(t *testing.T) {
	targetCmd := `powershell -NoProfile -Command "Start-Sleep -Seconds 5"`
	cmd := exec.Command("powershell", "-NoProfile", "-Command", "Start-Sleep -Seconds 5")
	if err := cmd.Start(); err != nil {
		t.Skipf("powershell start skipped: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
	}()

	myPid := os.Getpid()

	// Wait briefly for process to initialize
	time.Sleep(100 * time.Millisecond)

	// Should be active while running targetCmd
	hw := hasActiveWorker(myPid, 0, targetCmd)
	if !hw {
		t.Errorf("expected hasActiveWorker to be true while child is running targetCmd")
	}

	// Should be active when checking direct cmdlet without powershell wrapper
	if !hasActiveWorker(myPid, 0, "Start-Sleep -Seconds 5") {
		t.Errorf("expected hasActiveWorker to be true for direct cmdlet 'Start-Sleep -Seconds 5'")
	}

	// A shared first statement cannot prove a different compound command was approved.
	if hasActiveWorker(myPid, 0, "Start-Sleep -Seconds 5; Write-Output done") {
		t.Errorf("shared first statement incorrectly proved a compound command active")
	}

	// Should be FALSE when checking an unrelated command (e.g. git status)
	if hasActiveWorker(myPid, 0, "git status") {
		t.Errorf("expected hasActiveWorker to be false for unrelated command 'git status'")
	}

	// Should be FALSE when targetCmd is empty
	if hasActiveWorker(myPid, 0, "") {
		t.Errorf("expected hasActiveWorker to be false for empty targetCmd")
	}

	// Should be FALSE when targetCmd contains only shell wrappers and no target tokens
	if hasActiveWorker(myPid, 0, "powershell -NoProfile -ExecutionPolicy Bypass -Command \"\"") {
		t.Errorf("expected hasActiveWorker to be false when command has no valid target tokens")
	}

	// Kill child process and verify it becomes false
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	time.Sleep(150 * time.Millisecond)

	hwAfter := hasActiveWorker(myPid, 0, targetCmd)
	if hwAfter {
		t.Errorf("expected hasActiveWorker to be false after child is killed")
	}
}
