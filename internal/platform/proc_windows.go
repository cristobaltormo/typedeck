//go:build windows

package platform

import (
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// hideWindow stops console windows from flashing up and passes "cmd /C <command>" raw: Go escapes quotes for the C
// runtime, which cmd.exe does not understand.
func hideWindow(cmd *exec.Cmd) {
	attr := &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if len(cmd.Args) == 3 && strings.EqualFold(strings.TrimSuffix(filepath.Base(cmd.Args[0]), ".exe"), "cmd") && strings.EqualFold(cmd.Args[1], "/C") {
		attr.CmdLine = `cmd.exe /S /C "` + cmd.Args[2] + `"`
	}
	cmd.SysProcAttr = attr
}
