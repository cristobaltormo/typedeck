//go:build !windows

package platform

import "os/exec"

func hideWindow(*exec.Cmd) {}
