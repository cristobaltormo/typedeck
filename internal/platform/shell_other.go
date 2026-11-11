//go:build !windows

package platform

func StartShell(ShellHooks) {}

func AcquireInstance() bool { return true }

func StartedByUser() bool { return false }

func ShowError(title, text string) {}

func StopRunning() bool { return false }
