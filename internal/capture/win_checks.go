//go:build windows

package capture

import (
	"fmt"
	"unsafe"
)

var unsafeSizeofInput = unsafe.Sizeof(input{})

func (b *winBackend) Checks() []Check {
	return []Check{sessionCheck(), b.hookCheck(), elevationCheck()}
}

func sessionCheck() Check {
	pid, _, _ := pGetCurrentPID.Call()
	var id uint32
	pProcessIDToSession.Call(pid, ptr(&id))
	if id == 0 {
		return Check{Problem: "running in session 0 (a service): it has no keyboard; start it from your own session"}
	}
	return Check{OK: true, Message: fmt.Sprintf("interactive session %d", id)}
}

func (b *winBackend) hookCheck() Check {
	if b.Alive() {
		return Check{OK: true, Message: "the keyboard hook is installed and running"}
	}
	probe := &winBackend{}
	f := NewFilter()
	if err := probe.Start(f); err != nil {
		return Check{Problem: fmt.Sprint("the keyboard hook cannot be installed: ", err)}
	}
	probe.Stop()
	return Check{OK: true, Message: "the keyboard hook installs and removes cleanly"}
}

func elevationCheck() Check {
	if isElevated() {
		return Check{OK: true, Message: "running elevated: administrator windows are visible too"}
	}
	return Check{Problem: "optional: not elevated, so keys typed in administrator windows (and the UAC prompt) are not seen; that is a Windows rule"}
}

func isElevated() bool {
	proc, _, _ := pGetCurrentProcess.Call()
	var token uintptr
	if r, _, _ := pOpenProcessToken.Call(proc, tokenQuery, ptr(&token)); r == 0 {
		return false
	}
	var elevated, size uint32
	r, _, _ := pGetTokenInfo.Call(token, tokenElevation, ptr(&elevated), 4, ptr(&size))
	return r != 0 && elevated != 0
}
