//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--animation-worker" {
		runAnimationWorker(os.Args[2:])
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func consoleTitle() string {
	buf := make([]uint16, 2048)
	n, _, _ := modkernel32.NewProc("GetConsoleTitleW").Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:n])
}

func TestAnimationConsoleHelper(t *testing.T) {
	if os.Getenv("AGY_TITLE_CONSOLE_TEST") == "" {
		t.Skip("subprocess only")
	}
	pid := os.Getpid()
	ticks := getProcessCreationTicks(pid)
	homeDir, _ := os.UserHomeDir()
	label := os.Getenv("AGY_TITLE_CONSOLE_TEST")
	s := animationState{Animate: true, Title: label, StaticTitle: "⚙️ " + label}
	settingsDir := filepath.Join(homeDir, ".gemini", "antigravity-cli")
	if err := os.MkdirAll(settingsDir, 0700); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(settingsDir, "settings.json")
	// A CLI/custom-config animation must ignore an unrelated disabled default config.
	if err := os.WriteFile(settingsPath, []byte(`{"title":{"command":"other-title","enabled":false}}`), 0600); err != nil {
		t.Fatal(err)
	}
	updateAnimation(homeDir, pid, ticks, s)
	var frames []string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		title := consoleTitle()
		if strings.Contains(title, label) && (len(frames) == 0 || frames[len(frames)-1] != title) {
			frames = append(frames, title)
		}
		if len(frames) >= 3 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(frames) < 3 {
		t.Fatal("no continuous animation", frames)
	}
	if r, _, _ := ignoreAnimationCtrlC.Call(0, 1); r == 0 {
		t.Fatal("cannot protect isolated test owner")
	}
	if r, _, _ := modkernel32.NewProc("GenerateConsoleCtrlEvent").Call(0, 0); r == 0 {
		t.Fatal("cannot send isolated Ctrl+C")
	}
	before := consoleTitle()
	time.Sleep(250 * time.Millisecond)
	if consoleTitle() == before {
		t.Fatal("animation stopped after Ctrl+C")
	}
	for i := 0; i < 8; i++ {
		s.Animate = false
		updateAnimation(homeDir, pid, ticks, s)
		time.Sleep(100 * time.Millisecond)
		s.Animate = true
		updateAnimation(homeDir, pid, ticks, s)
		before := consoleTitle()
		deadline := time.Now().Add(time.Second)
		for consoleTitle() == before && time.Now().Before(deadline) {
			time.Sleep(25 * time.Millisecond)
		}
		if consoleTitle() == before {
			t.Fatal("animation lost on rapid resume", i)
		}
	}
	// Another callback must keep the same owner and release its pipe promptly.
	updateAnimation(homeDir, pid, ticks, s)
	time.Sleep(150 * time.Millisecond)
	for _, static := range []string{"⚠️ " + label, "✅ " + label, "⚪ " + label} {
		s.Animate = true
		updateAnimation(homeDir, pid, ticks, s)
		time.Sleep(200 * time.Millisecond)
		s.Animate = false
		s.StaticTitle = static
		updateAnimation(homeDir, pid, ticks, s)
		limit := time.Now().Add(time.Second)
		for consoleTitle() != static && time.Now().Before(limit) {
			time.Sleep(25 * time.Millisecond)
		}
		if consoleTitle() != static {
			t.Fatal("waiting title not restored", consoleTitle())
		}
	}
	// Leave a worker running: it must exit after this owner process exits.
	s.Animate = true
	updateAnimation(homeDir, pid, ticks, s)
	time.Sleep(200 * time.Millisecond)
	settingsPath = filepath.Join(homeDir, "custom-settings.json")
	s.SettingsPath = settingsPath
	updateAnimation(homeDir, pid, ticks, s)
	if err := os.WriteFile(settingsPath, []byte(`{"title":{"command":"title"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	limit := time.Now().Add(2 * time.Second)
	for consoleTitle() != s.StaticTitle && time.Now().Before(limit) {
		time.Sleep(50 * time.Millisecond)
	}
	if consoleTitle() != s.StaticTitle {
		t.Fatal("configuration disable did not stop animation")
	}
	if err := os.WriteFile(settingsPath, []byte(`{"title":{"command":"title --animate"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	updateAnimation(homeDir, pid, ticks, s)
	time.Sleep(200 * time.Millisecond)
	// Missing selected settings must stop a running worker without a telemetry update.
	// Point at a never-created path to exercise deletion's os.IsNotExist result without deleting fixtures.
	s.SettingsPath = filepath.Join(homeDir, "missing-settings.json")
	updateAnimation(homeDir, pid, ticks, s)
	limit = time.Now().Add(2 * time.Second)
	for consoleTitle() != s.StaticTitle && time.Now().Before(limit) {
		time.Sleep(50 * time.Millisecond)
	}
	if consoleTitle() != s.StaticTitle {
		t.Fatal("missing selected configuration did not stop animation")
	}
	s.SettingsPath = settingsPath
	updateAnimation(homeDir, pid, ticks, s)
	time.Sleep(200 * time.Millisecond)
	b, _ := json.Marshal(map[string]interface{}{"pid": pid, "ticks": ticks, "frames": frames})
	if err := os.WriteFile(os.Getenv("AGY_TITLE_REPORT"), b, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestAnimationIsolatedConsoles(t *testing.T) {
	exe, _ := os.Executable()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	type child struct {
		cmd    *exec.Cmd
		report string
	}
	var children []child
	for i := 0; i < 2; i++ {
		dir := t.TempDir()
		report := filepath.Join(dir, "report.json")
		cmd := exec.CommandContext(ctx, exe, "-test.run=^TestAnimationConsoleHelper$")
		cmd.Env = append(os.Environ(), "AGY_TITLE_CONSOLE_TEST=Tab"+strconv.Itoa(i), "AGY_TITLE_REPORT="+report, "USERPROFILE="+dir)
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x10, HideWindow: true}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		children = append(children, child{cmd, report})
	}
	for i, c := range children {
		if err := c.cmd.Wait(); err != nil {
			t.Fatal("console helper", err)
		}
		b, err := os.ReadFile(c.report)
		if err != nil {
			t.Fatal(err)
		}
		var r struct {
			Pid    int
			Ticks  int64
			Frames []string
		}
		if json.Unmarshal(b, &r) != nil {
			t.Fatal(string(b))
		}
		for _, f := range r.Frames {
			if !strings.Contains(f, fmt.Sprint("Tab", i)) {
				t.Fatal("cross-console title", f)
			}
		}
		deadline := time.Now().Add(2 * time.Second)
		for {
			if release, ok := animationLock(animationKey(r.Pid, r.Ticks), "worker"); ok {
				release()
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("worker survived owner")
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}
