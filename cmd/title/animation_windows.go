//go:build windows

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

var (
	createAnimationMutex   = modkernel32.NewProc("CreateMutexW")
	releaseAnimationMutex  = modkernel32.NewProc("ReleaseMutex")
	setAnimationTitle      = modkernel32.NewProc("SetConsoleTitleW")
	attachAnimationConsole = modkernel32.NewProc("AttachConsole")
	freeAnimationConsole   = modkernel32.NewProc("FreeConsole")
	moveAnimationFile      = modkernel32.NewProc("MoveFileExW")
	ignoreAnimationCtrlC   = modkernel32.NewProc("SetConsoleCtrlHandler")
)

func callbackSettingsPath(defaultPath string, agyPid int) string {
	if agyPid <= 0 {
		return defaultPath
	}
	return settingsArgument(getCommandLine(agyPid), getProcessCwd(agyPid), defaultPath)
}

func animationLock(key, purpose string, waitMillis ...uint32) (func(), bool) {
	// Win32 mutex ownership is thread-affine, including release after Go syscalls.
	runtime.LockOSThread()
	name, _ := syscall.UTF16PtrFromString("Local\\AntigravityTitle_" + purpose + "_" + key)
	h, _, _ := createAnimationMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		runtime.UnlockOSThread()
		return nil, false
	}
	var timeout uint32
	if len(waitMillis) > 0 {
		timeout = waitMillis[0]
	}
	wait, err := syscall.WaitForSingleObject(syscall.Handle(h), timeout)
	if err != nil || (wait != 0 && wait != 0x80) {
		syscall.CloseHandle(syscall.Handle(h))
		runtime.UnlockOSThread()
		return nil, false
	}
	return func() {
		releaseAnimationMutex.Call(h)
		syscall.CloseHandle(syscall.Handle(h))
		runtime.UnlockOSThread()
	}, true
}

func animationPath(homeDir, key string) string {
	return filepath.Join(homeDir, ".gemini", "antigravity-cli-title", "animation_"+key+".json")
}

func updateAnimation(homeDir string, pid int, ticks int64, state animationState) {
	if homeDir == "" || pid <= 0 || ticks == 0 {
		return
	}
	key := animationKey(pid, ticks)
	path := animationPath(homeDir, key)
	if !state.Animate {
		if _, err := os.Stat(path); err != nil {
			return
		}
	}
	unlock, ok := animationLock(key, "state", 20)
	if !ok {
		return
	}
	defer unlock()
	if os.MkdirAll(filepath.Dir(path), 0700) != nil {
		return
	}
	b, err := json.Marshal(state)
	if err != nil || len(b) > 65536 {
		return
	}
	temp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp.*")
	if err != nil {
		return
	}
	tmp := temp.Name()
	published := false
	defer func() {
		_ = temp.Close()
		if !published {
			_ = os.Remove(tmp)
		} // Only this invocation's newly created file.
	}()
	if _, err := temp.Write(b); err != nil {
		return
	}
	if err := temp.Close(); err != nil {
		return
	}
	from, _ := syscall.UTF16PtrFromString(tmp)
	to, _ := syscall.UTF16PtrFromString(path)
	if r, _, _ := moveAnimationFile.Call(uintptr(unsafe.Pointer(from)), uintptr(unsafe.Pointer(to)), 1); r == 0 {
		return
	}
	published = true
	if !state.Animate {
		return
	}
	if release, ok := animationLock(key, "worker"); ok {
		release()
	} else {
		return
	}
	// A separate named mutex admits exactly one worker, even for concurrent callbacks.
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, "--animation-worker", strconv.Itoa(pid), strconv.FormatInt(ticks, 10))
	// No inherited telemetry pipe handles; worker attaches to agy's console itself.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008, NoInheritHandles: true}
	if cmd.Start() == nil {
		_ = cmd.Process.Release()
	}
}

func runAnimationWorker(args []string) {
	if len(args) != 2 {
		return
	}
	pid, e1 := strconv.Atoi(args[0])
	ticks, e2 := strconv.ParseInt(args[1], 10, 64)
	if e1 != nil || e2 != nil || pid <= 0 || ticks <= 0 || getProcessCreationTicks(pid) != ticks {
		return
	}
	key := animationKey(pid, ticks)
	unlock, ok := animationLock(key, "worker")
	if !ok {
		return
	}
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()
	owner, err := syscall.OpenProcess(0x00100000, false, uint32(pid)) // SYNCHRONIZE
	if err != nil {
		return
	}
	defer syscall.CloseHandle(owner)
	freeAnimationConsole.Call()
	if r, _, _ := attachAnimationConsole.Call(uintptr(pid)); r == 0 {
		return
	}
	defer freeAnimationConsole.Call()
	// Attaching resets control-handler state. Ctrl+C belongs to agy, not its renderer.
	if r, _, _ := ignoreAnimationCtrlC.Call(0, 1); r == 0 {
		return
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}
	path := animationPath(homeDir, key)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	frame := 0
	var lastState animationState
	restore := func() {
		if lastState.Animate {
			if title, err := syscall.UTF16PtrFromString(cleanTitleText(lastState.StaticTitle)); err == nil {
				setAnimationTitle.Call(uintptr(unsafe.Pointer(title)))
			}
			lastState.Animate = false
		}
	}
	defer restore()
	for range ticker.C {
		if wait, err := syscall.WaitForSingleObject(owner, 0); err != nil || wait != syscall.WAIT_TIMEOUT {
			return
		}
		release, ok := animationLock(key, "state")
		if !ok {
			continue
		}
		b, err := readBounded(path, 65537)
		var state animationState
		if err != nil || len(b) > 65536 || json.Unmarshal(b, &state) != nil {
			restore()
			unlock()
			unlock = nil
			release()
			return
		}
		if frame%10 == 0 && state.SettingsPath != "" {
			if _, err := os.Stat(state.SettingsPath); os.IsNotExist(err) {
				state.Animate = false
			} else if err == nil {
				cfg, _ := readSettings(state.SettingsPath)
				if !isTitleCommand(cfg) {
					state.Animate = false
				} else {
					run := resolveRuntimeConfig(nil, cfg)
					state.Animate = state.Animate && animationEnabled(run)
				}
			}
		}
		title, err := syscall.UTF16PtrFromString(state.frame(frame))
		if err != nil {
			restore()
			unlock()
			unlock = nil
			release()
			return
		}
		r, _, _ := setAnimationTitle.Call(uintptr(unsafe.Pointer(title)))
		lastState = state
		if r == 0 || !state.Animate {
			// Retire while holding state: a new Working callback must see worker free.
			lastState.Animate = false // No deferred title write after releasing worker ownership.
			unlock()
			unlock = nil
			release()
			return
		}
		release()
		frame++
	}
}
