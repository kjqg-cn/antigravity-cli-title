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

	// Should be active while running
	hw := hasActiveWorker(myPid, 0, targetCmd)
	if !hw {
		t.Errorf("expected hasActiveWorker to be true while child is running")
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
