//go:build windows

package platform

import (
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf8"
	"unsafe"
)

var (
	pGetOEMCP         = kernel32.NewProc("GetOEMCP")
	pMultiByteToWChar = kernel32.NewProc("MultiByteToWideChar")
)

func hideWindow(cmd *exec.Cmd) {
	attr := &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if len(cmd.Args) == 3 && strings.EqualFold(strings.TrimSuffix(filepath.Base(cmd.Args[0]), ".exe"), "cmd") && strings.EqualFold(cmd.Args[1], "/C") {
		attr.CmdLine = `cmd.exe /S /C "` + cmd.Args[2] + `"`
	}
	cmd.SysProcAttr = attr
}

func decodeOutput(b []byte) string {
	if len(b) == 0 || utf8.Valid(b) {
		return string(b)
	}
	cp, _, _ := pGetOEMCP.Call()
	n, _, _ := pMultiByteToWChar.Call(cp, 0, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0, 0)
	if n == 0 {
		return string(b)
	}
	u := make([]uint16, n)
	pMultiByteToWChar.Call(cp, 0, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), uintptr(unsafe.Pointer(&u[0])), n)
	return syscall.UTF16ToString(u)
}
