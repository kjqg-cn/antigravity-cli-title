//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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
	createAnimationEvent   = modkernel32.NewProc("CreateEventW")
	openAnimationEvent     = modkernel32.NewProc("OpenEventW")
	setAnimationEvent      = modkernel32.NewProc("SetEvent")
	resetAnimationEvent    = modkernel32.NewProc("ResetEvent")
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

func updateAnimation(homeDir string, pid int, ticks int64, state animationState) (publishErr error) {
	if homeDir == "" || pid <= 0 || ticks == 0 {
		return errors.New("animation owner unavailable")
	}
	key := animationKey(pid, ticks)
	defer func() {
		if !state.Animate && publishErr != nil && publishErr != errSupersededCallback {
			if b, err := readBounded(animationPath(homeDir, key), 65537); err == nil {
				var latest animationState
				if json.Unmarshal(b, &latest) == nil && state.UpdatedAt > 0 && latest.UpdatedAt > state.UpdatedAt {
					publishErr = errSupersededCallback
					return
				}
			}
			// A final callback must not leave an old spinner running when disk/lock
			// publication fails. Cancel out of band before agy applies stdout.
			if !stopAnimationWorker(key) {
				publishErr = errors.New("final status publication failed; worker stop unconfirmed")
			}
		}
	}()
	path := animationPath(homeDir, key)
	if !state.Animate && !state.Enabled {
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return errors.New("animation state unavailable")
		}
	}
	unlock, ok := animationLock(key, "state", 50)
	if !ok {
		return errors.New("animation state lock timeout")
	}
	defer unlock()
	if previous, err := readBounded(path, 65537); err == nil {
		var saved animationState
		if json.Unmarshal(previous, &saved) == nil && state.UpdatedAt > 0 && saved.UpdatedAt > state.UpdatedAt {
			return errSupersededCallback
		}
	}
	if err := writeAnimationState(path, state); err != nil {
		return err
	}
	if !state.Animate {
		return nil
	}
	if release, ok := animationLock(key, "worker"); ok {
		release()
	} else {
		return nil
	}
	// A separate named mutex admits exactly one worker, even for concurrent callbacks.
	exe, err := os.Executable()
	if err != nil {
		return errors.New("animation executable unavailable")
	}
	cmd := exec.Command(exe, "--animation-worker", strconv.Itoa(pid), strconv.FormatInt(ticks, 10))
	// No inherited telemetry pipe handles; worker attaches to agy's console itself.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008, NoInheritHandles: true}
	if cmd.Start() != nil {
		return errors.New("animation worker launch failed")
	}
	_ = cmd.Process.Release()
	return nil
}

func animationEventName(key string) *uint16 {
	name, _ := syscall.UTF16PtrFromString("Local\\AntigravityTitle_stop_" + key)
	return name
}

func stopAnimationWorker(key string) bool {
	name := animationEventName(key)
	h, _, _ := openAnimationEvent.Call(0x0002, 0, uintptr(unsafe.Pointer(name))) // EVENT_MODIFY_STATE
	if h != 0 {
		setAnimationEvent.Call(h)
		defer syscall.CloseHandle(syscall.Handle(h))
	}
	if release, ok := animationLock(key, "worker", 250); ok {
		if h != 0 {
			resetAnimationEvent.Call(h)
		}
		release()
		return true
	}
	return false
}

// Caller holds state ownership. Atomic replacement cannot expose a partial JSON record.
func writeAnimationState(path string, state animationState) error {
	if os.MkdirAll(filepath.Dir(path), 0700) != nil {
		return errors.New("animation directory unavailable")
	}
	b, err := json.Marshal(state)
	if err != nil || len(b) > 65536 {
		return errors.New("animation state too large")
	}
	temp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp.*")
	if err != nil {
		return errors.New("animation temporary file failed")
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
		return errors.New("animation state write failed")
	}
	if err := temp.Close(); err != nil {
		return errors.New("animation state close failed")
	}
	from, _ := syscall.UTF16PtrFromString(tmp)
	to, _ := syscall.UTF16PtrFromString(path)
	for attempt := 0; attempt < 3; attempt++ {
		if r, _, _ := moveAnimationFile.Call(uintptr(unsafe.Pointer(from)), uintptr(unsafe.Pointer(to)), 1); r != 0 {
			published = true
			return nil
		}
		if attempt < 2 {
			time.Sleep(5 * time.Millisecond)
		}
	}
	return errors.New("animation state replacement failed")
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
	stopName := animationEventName(key)
	stop, _, _ := createAnimationEvent.Call(0, 1, 0, uintptr(unsafe.Pointer(stopName)))
	if stop == 0 {
		return
	}
	defer syscall.CloseHandle(syscall.Handle(stop))
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
	stopped := func() bool {
		wait, err := syscall.WaitForSingleObject(syscall.Handle(stop), 0)
		if err == nil && wait == 0 {
			lastState.Animate = false
			return true
		}
		return false
	}
	for {
		if stopped() {
			return
		}
		if wait, err := syscall.WaitForSingleObject(owner, 0); err != nil || wait != syscall.WAIT_TIMEOUT {
			return
		}
		release, ok := animationLock(key, "state")
		if !ok {
			<-ticker.C
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
		// Settings and task logs may be slower than a title update. Inspect them
		// outside the state lock, then verify the callback hasn't published new data.
		release()
		if frame%10 == 0 && state.Animate && state.SettingsPath != "" {
			cfg, _, settingsErr := readSettingsWithStatus(state.SettingsPath)
			if os.IsNotExist(settingsErr) {
				state.Animate = false
				state.Reason = "settings_missing"
			} else if settingsErr == nil {
				if !isTitleCommand(cfg) {
					state.Animate = false
					state.Reason = "animation_disabled"
				} else {
					run := resolveRuntimeConfig(nil, cfg)
					state.Animate = state.Animate && animationEnabled(run)
					if !state.Animate {
						state.Reason = "animation_disabled"
					}
				}
			} else {
				recordWorkerTrace(homeDir, pid, ticks, state, errors.New("settings temporarily unreadable or invalid"))
			}
		}
		if frame%10 == 0 {
			state = state.refreshCompletion(homeDir)
		}
		if stopped() {
			return
		}
		release, ok = animationLock(key, "state")
		if !ok {
			<-ticker.C
			continue
		}
		current, err := readBounded(path, 65537)
		if err != nil || !bytes.Equal(b, current) {
			release()
			<-ticker.C
			continue
		}
		if !state.Animate && state.Reason == "background_timeout" {
			if err := writeAnimationState(path, state); err != nil {
				recordWorkerTrace(homeDir, pid, ticks, state, err)
			}
		}
		title, err := syscall.UTF16PtrFromString(state.frameAt(time.Now()))
		if err != nil {
			restore()
			unlock()
			unlock = nil
			release()
			return
		}
		r, _, drawErr := setAnimationTitle.Call(uintptr(unsafe.Pointer(title)))
		lastState = state
		if r == 0 || !state.Animate {
			recordWorkerTrace(homeDir, pid, ticks, state, animationDrawError(r, drawErr))
			// Retire while holding state: a new Working callback must see worker free.
			lastState.Animate = false // No deferred title write after releasing worker ownership.
			unlock()
			unlock = nil
			release()
			return
		}
		release()
		frame++
		<-ticker.C
	}
}

func animationDrawError(result uintptr, callErr error) error {
	if result != 0 {
		return nil
	}
	if code, ok := callErr.(syscall.Errno); ok && code != 0 {
		return fmt.Errorf("SetConsoleTitleW failed (Win32 error %d)", uint32(code))
	}
	return errors.New("SetConsoleTitleW failed (no Win32 error reported)")
}
