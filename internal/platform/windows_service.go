//go:build windows

package platform

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/cristobaltormo/typedeck/internal/winreg"
)

const (
	taskName = "Typedeck"
	runKey   = `Software\Microsoft\Windows\CurrentVersion\Run`
)

func guiExecutable(exe string) string {
	if strings.EqualFold(filepath.Base(exe), "typedeck-cli.exe") {
		gui := filepath.Join(filepath.Dir(exe), "typedeck.exe")
		if _, err := os.Stat(gui); err == nil {
			return gui
		}
	}
	return exe
}

func taskExists() bool {
	_, err := Run(10*time.Second, "", "schtasks", "/Query", "/TN", taskName)
	return err == nil
}

func runKeyCommand() (string, bool) { return winreg.String(winreg.CurrentUser, runKey, taskName) }

func autostartInstalled() (bool, string) {
	if taskExists() {
		return true, "scheduled task"
	}
	if _, ok := runKeyCommand(); ok {
		return true, "Run registry key"
	}
	return false, ""
}

func createTask(exe string) error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	f, err := os.CreateTemp("", "typedeck-task-*.xml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(utf16WithBOM(taskXML(u.Username, exe)))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	_, err = Run(20*time.Second, "", "schtasks", "/Create", "/TN", taskName, "/XML", f.Name(), "/F")
	return err
}

func (*windows) InstallService(exe string) (string, error) {
	exe = guiExecutable(exe)
	if err := createTask(exe); err == nil {
		_ = winreg.DeleteValue(winreg.CurrentUser, runKey, taskName)
		_, _ = Run(15*time.Second, "", "schtasks", "/Run", "/TN", taskName)
		return `Scheduled task "` + taskName + `" created: it starts when you sign in (and is already running)`, nil
	}
	if err := winreg.SetString(winreg.CurrentUser, runKey, taskName, `"`+exe+`" --background`); err != nil {
		return "", fmt.Errorf("could not create the scheduled task or the Run entry: %w", err)
	}
	_ = Detached(exe, "--background")
	return "Windows did not allow a scheduled task, so Typedeck was added to your Run entries: it starts when you sign in (and is already running)", nil
}

func (*windows) UninstallService() (string, error) {
	if taskExists() {
		if out, err := Run(15*time.Second, "", "schtasks", "/Delete", "/TN", taskName, "/F"); err != nil {
			return "", fmt.Errorf("schtasks: %s", out)
		}
	}
	if err := winreg.DeleteValue(winreg.CurrentUser, runKey, taskName); err != nil {
		return "", errors.New("could not remove the Run entry: " + err.Error())
	}
	if !StopRunning() {
		_, _ = Run(10*time.Second, "", "taskkill", "/F", "/IM", "typedeck.exe")
	}
	return "Start with your session removed and Typedeck stopped (the configuration is kept)", nil
}
