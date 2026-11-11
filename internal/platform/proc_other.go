//go:build !windows

package platform

import "os/exec"

func hideWindow(*exec.Cmd) {}

func decodeOutput(b []byte) string { return string(b) }
