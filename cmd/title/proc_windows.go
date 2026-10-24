//go:build windows

package main

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	modntdll              = syscall.NewLazyDLL("ntdll.dll")
	procNtQueryInfo       = modntdll.NewProc("NtQueryInformationProcess")
	modkernel32           = syscall.NewLazyDLL("kernel32.dll")
	procReadProcessMemory = modkernel32.NewProc("ReadProcessMemory")
	procGetProcessTimes   = modkernel32.NewProc("GetProcessTimes")

	modwinsqlite3        = syscall.NewLazyDLL("winsqlite3.dll")
	procSqlite3OpenV2    = modwinsqlite3.NewProc("sqlite3_open_v2")
	procSqlite3PrepareV2 = modwinsqlite3.NewProc("sqlite3_prepare_v2")
	procSqlite3Step      = modwinsqlite3.NewProc("sqlite3_step")
	procSqlite3ColText   = modwinsqlite3.NewProc("sqlite3_column_text")
	procSqlite3ColBytes  = modwinsqlite3.NewProc("sqlite3_column_bytes")
	procSqlite3Finalize  = modwinsqlite3.NewProc("sqlite3_finalize")
	procSqlite3Close     = modwinsqlite3.NewProc("sqlite3_close")
)

type processBasicInformation struct {
	ExitStatus                   uintptr
	PebBaseAddress               uintptr
	AffinityMask                 uintptr
	BasePriority                 uintptr
	UniqueProcessId              uintptr
	InheritedFromUniqueProcessId uintptr
}

func readProcessMemory(hProc syscall.Handle, addr uintptr, buf []byte) bool {
	var bytesRead uintptr
	r, _, _ := procReadProcessMemory.Call(
		uintptr(hProc),
		addr,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(&bytesRead)),
	)
	return r != 0 && bytesRead == uintptr(len(buf))
}

func getCommandLine(pid int) string {
	hProc, err := syscall.OpenProcess(0x0400|0x0010, false, uint32(pid))
	if err != nil {
		return ""
	}
	defer syscall.CloseHandle(hProc)

	var pbi processBasicInformation
	var retLen uint32
	status, _, _ := procNtQueryInfo.Call(
		uintptr(hProc),
		0,
		uintptr(unsafe.Pointer(&pbi)),
		uintptr(unsafe.Sizeof(pbi)),
		uintptr(unsafe.Pointer(&retLen)),
	)
	if status != 0 || pbi.PebBaseAddress == 0 {
		return ""
	}

	ppOffset := pbi.PebBaseAddress + 0x20
	addrBuf := make([]byte, 8)
	if !readProcessMemory(hProc, ppOffset, addrBuf) {
		return ""
	}
	procParams := uintptr(binary.LittleEndian.Uint64(addrBuf))
	if procParams == 0 {
		return ""
	}

	header := make([]byte, 16)
	if !readProcessMemory(hProc, procParams+0x70, header) {
		return ""
	}

	cmdLen := binary.LittleEndian.Uint16(header[0:2])
	bufferPtr := uintptr(binary.LittleEndian.Uint64(header[8:16]))
	if cmdLen == 0 || bufferPtr == 0 {
		return ""
	}

	cmdBytes := make([]byte, cmdLen)
	if !readProcessMemory(hProc, bufferPtr, cmdBytes) {
		return ""
	}

	u16 := make([]uint16, cmdLen/2)
	for i := 0; i < len(u16); i++ {
		u16[i] = binary.LittleEndian.Uint16(cmdBytes[i*2 : (i+1)*2])
	}
	return string(utf16.Decode(u16))
}

func getProcessCreationTicks(pid int) int64 {
	hProc, err := syscall.OpenProcess(0x1000, false, uint32(pid))
	if err != nil {
		hProc, err = syscall.OpenProcess(0x0400, false, uint32(pid))
		if err != nil {
			return 0
		}
	}
	defer syscall.CloseHandle(hProc)

	var creationTime, exitTime, kernelTime, userTime syscall.Filetime
	r, _, _ := procGetProcessTimes.Call(
		uintptr(hProc),
		uintptr(unsafe.Pointer(&creationTime)),
		uintptr(unsafe.Pointer(&exitTime)),
		uintptr(unsafe.Pointer(&kernelTime)),
		uintptr(unsafe.Pointer(&userTime)),
	)
	if r != 0 {
		fileTime := (int64(creationTime.HighDateTime) << 32) | int64(creationTime.LowDateTime)
		return fileTime + 504911232000000000
	}
	return 0
}

func getParentPid(pid int) int {
	hProc, err := syscall.OpenProcess(0x1000, false, uint32(pid))
	if err != nil {
		hProc, err = syscall.OpenProcess(0x0400, false, uint32(pid))
		if err != nil {
			return 0
		}
	}
	defer syscall.CloseHandle(hProc)

	var pbi processBasicInformation
	var retLen uint32
	status, _, _ := procNtQueryInfo.Call(
		uintptr(hProc),
		0,
		uintptr(unsafe.Pointer(&pbi)),
		uintptr(unsafe.Sizeof(pbi)),
		uintptr(unsafe.Pointer(&retLen)),
	)
	if status == 0 {
		return int(pbi.InheritedFromUniqueProcessId)
	}
	return 0
}

type procEntry struct {
	pid       int
	parentPid int
	exe       string
}

func getExactAncestorAgy(startPid int) (int, int64) {
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, 0
	}
	defer syscall.CloseHandle(snap)

	procMap := make(map[int]procEntry)
	var entry syscall.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	if err := syscall.Process32First(snap, &entry); err == nil {
		for {
			name := syscall.UTF16ToString(entry.ExeFile[:])
			procMap[int(entry.ProcessID)] = procEntry{
				pid:       int(entry.ProcessID),
				parentPid: int(entry.ParentProcessID),
				exe:       name,
			}
			if err := syscall.Process32Next(snap, &entry); err != nil {
				break
			}
		}
	}

	cur := startPid
	for i := 0; i < 6; i++ {
		e, ok := procMap[cur]
		if !ok || e.parentPid <= 0 {
			p := getParentPid(cur)
			if p <= 0 {
				break
			}
			cur = p
		} else {
			cur = e.parentPid
		}

		parentInfo, ok := procMap[cur]
		exeName := ""
		if ok {
			exeName = strings.ToLower(parentInfo.exe)
		} else {
			cmd := strings.ToLower(getCommandLine(cur))
			exeName = filepath.Base(cmd)
		}

		if exeName == "agy.exe" || exeName == "agy" || strings.HasPrefix(exeName, "agy") {
			ticks := getProcessCreationTicks(cur)
			return cur, ticks
		}
	}
	return 0, 0
}

func hasActiveWorker(agyPid, currentPid int, targetCmd string) bool {
	if agyPid <= 0 {
		return false
	}

	selfAncestors := make(map[int]bool)
	cur := currentPid
	for i := 0; i < 6; i++ {
		if cur <= 0 || cur == agyPid {
			break
		}
		selfAncestors[cur] = true
		cur = getParentPid(cur)
	}

	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(snap)

	procMap := make(map[int]procEntry)
	var entry syscall.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	if err := syscall.Process32First(snap, &entry); err == nil {
		for {
			pid := int(entry.ProcessID)
			ppid := int(entry.ParentProcessID)
			name := strings.ToLower(syscall.UTF16ToString(entry.ExeFile[:]))
			procMap[pid] = procEntry{
				pid:       pid,
				parentPid: ppid,
				exe:       name,
			}
			if err := syscall.Process32Next(snap, &entry); err != nil {
				break
			}
		}
	}

	// BFS traversal to collect all descendants of agyPid up to 5 levels deep
	descendants := make(map[int]bool)
	visited := make(map[int]bool)
	type queueItem struct {
		pid   int
		depth int
	}
	queue := []queueItem{{pid: agyPid, depth: 0}}
	visited[agyPid] = true

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if item.depth >= 5 {
			continue
		}

		for pid, info := range procMap {
			if info.parentPid == item.pid && !visited[pid] {
				visited[pid] = true
				descendants[pid] = true
				queue = append(queue, queueItem{pid: pid, depth: item.depth + 1})
			}
		}
	}

	cleanedTarget := strings.ToLower(strings.TrimSpace(targetCmd))
	var targetTokens []string
	if cleanedTarget != "" {
		fields := strings.FieldsFunc(cleanedTarget, func(r rune) bool {
			return r == ' ' || r == '\t' || r == '\r' || r == '\n' || r == '"' || r == '\'' || r == ';'
		})
		for _, f := range fields {
			f = strings.Trim(f, "\"',;")
			if f == "powershell" || f == "powershell.exe" || f == "pwsh" || f == "cmd" || f == "cmd.exe" ||
				f == "-noprofile" || f == "-executionpolicy" || f == "bypass" || f == "-command" || f == "/c" {
				continue
			}
			if len(f) >= 2 {
				targetTokens = append(targetTokens, f)
			}
		}
	}

	hasAnyValidDescendant := false
	for childPid := range descendants {
		if selfAncestors[childPid] {
			continue
		}
		info := procMap[childPid]
		exe := info.exe
		if exe == "conhost.exe" || exe == "csc.exe" || exe == "cvtres.exe" {
			continue
		}

		cmd := strings.ToLower(getCommandLine(childPid))
		if strings.Contains(cmd, "statusline") || strings.Contains(cmd, "title.ps1") ||
			strings.Contains(cmd, "title.sh") || strings.Contains(cmd, "title.exe") ||
			strings.Contains(cmd, "conhost") {
			continue
		}

		hasAnyValidDescendant = true

		if len(targetTokens) == 0 {
			return true
		}

		if cmd != "" {
			if strings.Contains(cmd, cleanedTarget) {
				return true
			}
			for _, tok := range targetTokens {
				if strings.Contains(cmd, tok) {
					return true
				}
			}
		}

		for _, tok := range targetTokens {
			if exe == tok || exe == tok+".exe" {
				return true
			}
		}
	}

	if hasAnyValidDescendant {
		return true
	}

	return false
}

func getConversationTitle(dbPath, convID string) string {
	if convID == "" || dbPath == "" {
		return ""
	}
	var db uintptr
	pDbPath, err := syscall.BytePtrFromString(dbPath)
	if err != nil {
		return ""
	}
	r, _, _ := procSqlite3OpenV2.Call(
		uintptr(unsafe.Pointer(pDbPath)),
		uintptr(unsafe.Pointer(&db)),
		uintptr(1), // SQLITE_OPEN_READONLY
		0,
	)
	if r != 0 || db == 0 {
		return ""
	}
	defer procSqlite3Close.Call(db)

	sql := fmt.Sprintf("SELECT title FROM conversation_summaries WHERE conversation_id = '%s' LIMIT 1", strings.ReplaceAll(convID, "'", "''"))
	pSql, err := syscall.BytePtrFromString(sql)
	if err != nil {
		return ""
	}

	var stmt uintptr
	r, _, _ = procSqlite3PrepareV2.Call(
		db,
		uintptr(unsafe.Pointer(pSql)),
		uintptr(len(sql)),
		uintptr(unsafe.Pointer(&stmt)),
		0,
	)
	if r != 0 || stmt == 0 {
		return ""
	}
	defer procSqlite3Finalize.Call(stmt)

	r, _, _ = procSqlite3Step.Call(stmt)
	if r == 100 { // SQLITE_ROW
		ptr, _, _ := procSqlite3ColText.Call(stmt, 0)
		nBytes, _, _ := procSqlite3ColBytes.Call(stmt, 0)
		if ptr != 0 && nBytes > 0 {
			buf := make([]byte, nBytes)
			var bytesRead uintptr
			// Read safely from the current process memory without unsafe.Pointer(uintptr) conversion
			currProc := syscall.Handle(^uintptr(0))
			r, _, _ := procReadProcessMemory.Call(
				uintptr(currProc),
				ptr,
				uintptr(unsafe.Pointer(&buf[0])),
				nBytes,
				uintptr(unsafe.Pointer(&bytesRead)),
			)
			if r != 0 && bytesRead > 0 {
				return string(buf[:bytesRead])
			}
		}
	}
	return ""
}
