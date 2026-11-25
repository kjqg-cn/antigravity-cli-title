//go:build windows

package main

import (
	"encoding/binary"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// CurrentDirectory is a UNICODE_STRING in RTL_USER_PROCESS_PARAMETERS.
func getProcessCwd(pid int) string {
	h, err := syscall.OpenProcess(0x0400|0x0010, false, uint32(pid))
	if err != nil {
		return ""
	}
	defer syscall.CloseHandle(h)
	var pbi processBasicInformation
	r, _, _ := procNtQueryInfo.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&pbi)), unsafe.Sizeof(pbi), 0)
	if r != 0 || pbi.PebBaseAddress == 0 {
		return ""
	}
	peb, pointerSize := remotePeb(h, pbi.PebBaseAddress)
	ppOffset, cwdOffset := uintptr(0x20), uintptr(0x38)
	if pointerSize == 4 {
		ppOffset, cwdOffset = 0x10, 0x24
	}
	ptr := make([]byte, pointerSize)
	if !readProcessMemory(h, peb+ppOffset, ptr) {
		return ""
	}
	params := remotePointer(ptr)
	str := make([]byte, pointerSize*2)
	if params == 0 || !readProcessMemory(h, params+cwdOffset, str) {
		return ""
	}
	n := binary.LittleEndian.Uint16(str[:2])
	addr := remotePointer(str[pointerSize:])
	if n == 0 || n%2 != 0 || addr == 0 {
		return ""
	}
	buf := make([]byte, n)
	if !readProcessMemory(h, addr, buf) {
		return ""
	}
	chars := make([]uint16, n/2)
	for i := range chars {
		chars[i] = binary.LittleEndian.Uint16(buf[2*i:])
	}
	return string(utf16.Decode(chars))
}

func remotePeb(h syscall.Handle, nativePeb uintptr) (uintptr, int) {
	var wow64Peb uintptr
	r, _, _ := procNtQueryInfo.Call(uintptr(h), 26, uintptr(unsafe.Pointer(&wow64Peb)), unsafe.Sizeof(wow64Peb), 0)
	if r == 0 && wow64Peb != 0 {
		return wow64Peb, 4
	}
	return nativePeb, 8
}

func remotePointer(b []byte) uintptr {
	if len(b) == 4 {
		return uintptr(binary.LittleEndian.Uint32(b))
	}
	return uintptr(binary.LittleEndian.Uint64(b))
}
