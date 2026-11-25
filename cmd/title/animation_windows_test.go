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
	// Simulate another process truncating settings before completing its write.
	// Keep the incomplete object across a poll: it must not disable the worker.
	if err := os.WriteFile(settingsPath, []byte(`{"title":`), 0600); err != nil {
		t.Fatal(err)
	}
	updateAnimation(homeDir, pid, ticks, s)
	time.Sleep(1250 * time.Millisecond)
	if release, ok := animationLock(animationKey(pid, ticks), "worker"); ok {
		release()
		t.Fatal("incomplete settings stopped animation")
	}
	if consoleTitle() == s.StaticTitle {
		t.Fatal("incomplete settings restored static Working")
	}
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
	// Idle background work must decay with no further callbacks, while a new
	// Working event must be able to supersede that autonomous completion.
	tasksDir := filepath.Join(homeDir, "tasks")
	if err := os.MkdirAll(tasksDir, 0700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(tasksDir, "task.log")
	if err := os.WriteFile(logPath, []byte("running"), 0600); err != nil {
		t.Fatal(err)
	}
	s.Reason = "background_tasks"
	s.UpdatedAt = time.Now().UnixNano()
	s.Trace = true
	s.Completion = &backgroundCompletion{ConversationID: "test", TranscriptPath: filepath.Join(homeDir, "logs", "transcript.jsonl"), TaskTimeout: 700 * time.Millisecond, DoneTitle: "✅ " + label}
	if err := updateAnimation(homeDir, pid, ticks, s); err != nil {
		t.Fatal(err)
	}
	limit = time.Now().Add(3 * time.Second)
	for consoleTitle() != "✅ "+label && time.Now().Before(limit) {
		time.Sleep(40 * time.Millisecond)
	}
	if consoleTitle() != "✅ "+label {
		t.Fatal("idle background timeout needed another callback", consoleTitle())
	}
	bState, err := readBounded(animationPath(homeDir, animationKey(pid, ticks)), 65537)
	var completed animationState
	if err != nil || json.Unmarshal(bState, &completed) != nil || completed.Animate {
		t.Fatal("completion was not published", err)
	}
	s.Completion = nil
	s.Reason = "foreground"
	s.UpdatedAt += 2
	if err := updateAnimation(homeDir, pid, ticks, s); err != nil {
		t.Fatal(err)
	}
	s.Animate = false
	s.StaticTitle = "⚠️ " + label
	s.UpdatedAt += 2
	if err := updateAnimation(homeDir, pid, ticks, s); err != nil {
		t.Fatal(err)
	}
	old := s
	old.Animate = true
	old.UpdatedAt--
	if err := updateAnimation(homeDir, pid, ticks, old); err != errSupersededCallback {
		t.Fatal("old Working callback overwrote final state", err)
	}
	limit = time.Now().Add(time.Second)
	for consoleTitle() != s.StaticTitle && time.Now().Before(limit) {
		time.Sleep(25 * time.Millisecond)
	}
	if consoleTitle() != s.StaticTitle {
		t.Fatal("final transition was lost")
	}
	s.Animate = true
	s.UpdatedAt++
	if err := updateAnimation(homeDir, pid, ticks, s); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	// Simulate a filesystem refusing replacement of the final state. The
	// out-of-band cancellation must stop the old worker before stdout is applied.
	stateFile, _ := syscall.UTF16PtrFromString(animationPath(homeDir, animationKey(pid, ticks)))
	handle, err := syscall.CreateFile(stateFile, syscall.GENERIC_READ, syscall.FILE_SHARE_READ, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	s.Animate = false
	s.UpdatedAt++
	s.StaticTitle = "✅ " + label
	publishErr := updateAnimation(homeDir, pid, ticks, s)
	_ = syscall.CloseHandle(handle)
	if publishErr == nil {
		t.Fatal("expected locked final state to reject replacement")
	}
	if release, ok := animationLock(animationKey(pid, ticks), "worker"); ok {
		release()
	} else {
		t.Fatal("failed final publication left spinner alive")
	}
	finalTitle, _ := syscall.UTF16PtrFromString(s.StaticTitle)
	setAnimationTitle.Call(uintptr(unsafe.Pointer(finalTitle)))
	time.Sleep(200 * time.Millisecond)
	if consoleTitle() != s.StaticTitle {
		t.Fatal("cancelled worker overwrote final stdout title")
	}
	s.Animate = true
	s.UpdatedAt++
	if err := updateAnimation(homeDir, pid, ticks, s); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	b, _ := json.Marshal(map[string]interface{}{"pid": pid, "ticks": ticks, "frames": frames})
	if err := os.WriteFile(os.Getenv("AGY_TITLE_REPORT"), b, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestAnimationIsolatedConsoles(t *testing.T) {
	exe, _ := os.Executable()
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
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

func TestAnimationPublicationFailureAndStaleCallback(t *testing.T) {
	home := t.TempDir()
	pid, ticks := os.Getpid(), getProcessCreationTicks(os.Getpid())
	key := animationKey(pid, ticks)
	p := animationPath(home, key)
	latest := animationState{StaticTitle: "✅ Done", UpdatedAt: 20}
	if err := writeAnimationState(p, latest); err != nil {
		t.Fatal(err)
	}
	old := animationState{Animate: true, StaticTitle: "⚙️ Old", UpdatedAt: 10}
	if err := updateAnimation(home, pid, ticks, old); err != errSupersededCallback {
		t.Fatal("stale callback accepted", err)
	}
	b, err := os.ReadFile(p)
	var got animationState
	if err != nil || json.Unmarshal(b, &got) != nil || got.StaticTitle != latest.StaticTitle {
		t.Fatal("final status changed", got, err)
	}
	// Replacing a directory must fail and must clean only its new scratch file.
	blocked := filepath.Join(home, "blocked")
	if err := os.Mkdir(blocked, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeAnimationState(blocked, latest); err == nil {
		t.Fatal("expected atomic publication error")
	}
	leftovers, err := filepath.Glob(blocked + ".tmp.*")
	if err != nil || len(leftovers) != 0 {
		t.Fatal("publication scratch files leaked", leftovers, err)
	}
}

func TestFirstStaticCallbackBlocksLateFirstWorking(t *testing.T) {
	pid, ticks := os.Getpid(), getProcessCreationTicks(os.Getpid())
	for _, reason := range []string{"waiting", "turn_complete", "ready"} {
		t.Run(reason, func(t *testing.T) {
			home := t.TempDir()
			p := animationPath(home, animationKey(pid, ticks))
			final := animationState{Enabled: true, StaticTitle: "Final " + reason, Reason: reason, UpdatedAt: 20}
			if err := updateAnimation(home, pid, ticks, final); err != nil {
				t.Fatal(err)
			}
			late := animationState{Enabled: true, Animate: true, StaticTitle: "⚙️ Old", UpdatedAt: 10}
			if err := updateAnimation(home, pid, ticks, late); err != errSupersededCallback {
				t.Fatal("late first Working accepted", err)
			}
			b, err := os.ReadFile(p)
			var saved animationState
			if err != nil || json.Unmarshal(b, &saved) != nil || saved.StaticTitle != final.StaticTitle || saved.UpdatedAt != 20 || saved.Animate {
				t.Fatal("first static state lost", saved, err)
			}
		})
	}
	home := t.TempDir()
	if err := updateAnimation(home, pid, ticks, animationState{StaticTitle: "Static mode", UpdatedAt: 30}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(animationPath(home, animationKey(pid, ticks))); !os.IsNotExist(err) {
		t.Fatal("static mode created animation record", err)
	}
}

func TestAnimationDrawFailureTrace(t *testing.T) {
	for _, tc := range []struct {
		result uintptr
		err    error
		want   string
	}{
		{0, syscall.Errno(6), "Win32 error 6"},
		{0, syscall.Errno(0), "no Win32 error reported"},
	} {
		err := animationDrawError(tc.result, tc.err)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatal(err, tc.want)
		}
		home := t.TempDir()
		recordWorkerTrace(home, 123, 456, animationState{Animate: true, Trace: true, Reason: "foreground"}, err)
		p := filepath.Join(home, ".gemini", "antigravity-cli-title", "diagnostic_123_456.jsonl")
		b, readErr := os.ReadFile(p)
		if readErr != nil || !strings.Contains(string(b), "SetConsoleTitleW failed") || !strings.Contains(string(b), tc.want) {
			t.Fatal(string(b), readErr)
		}
	}
	if err := animationDrawError(1, syscall.Errno(6)); err != nil {
		t.Fatal("successful API surfaced stale LastError", err)
	}
}
