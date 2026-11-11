//go:build linux

package capture

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

func (b *linuxBackend) Checks() []Check {
	checks := []Check{uinputCheck(), keyboardsCheck()}
	if s := b.current(); s != nil {
		checks = append(checks, grabCheck(s.grabFailures()))
	}
	return append(checks, clipboardCheck())
}

func grabCheck(failed map[string]string) Check {
	if len(failed) == 0 {
		return Check{OK: true, Code: "grab", Message: "every keyboard was taken for capture"}
	}
	paths := make([]string, 0, len(failed))
	for p := range failed {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return Check{Code: "grab_failed", Problem: fmt.Sprintf("could not take %s for capture (another program may hold it): keys typed there are not captured", strings.Join(paths, ", "))}
}

func uinputCheck() Check {
	f, err := openUinput()
	if err != nil {
		return Check{Code: "uinput", Problem: uinputError(err).Error()}
	}
	f.Close()
	return Check{OK: true, Code: "uinput", Message: "/dev/uinput can be opened, so keys can be passed on and sent"}
}

func keyboardsCheck() Check {
	readable, denied := probeKeyboards()
	switch {
	case readable > 0:
		return Check{OK: true, Code: "keyboards", Message: fmt.Sprintf("%d keyboard(s) readable in /dev/input", readable)}
	case denied > 0:
		return Check{Code: "input_access", Problem: noReadAccess}
	}
	return Check{Code: "no_keyboard", Problem: "no keyboard found in /dev/input"}
}

func probeKeyboards() (readable, denied int) {
	names, _ := filepath.Glob(filepath.Join(inputDir, "event*"))
	for _, path := range names {
		f, err := openEvdev(path)
		if errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) {
			denied++
			continue
		}
		if err != nil {
			continue
		}
		if isKeyboard(f) {
			readable++
		}
		f.Close()
	}
	return readable, denied
}

func clipboardCheck() Check {
	tool, other := "xclip", "xsel"
	if os.Getenv("XDG_SESSION_TYPE") == "wayland" {
		tool, other = "wl-copy", ""
	}
	for _, name := range []string{tool, other} {
		if name != "" {
			if _, err := exec.LookPath(name); err == nil {
				return Check{OK: true, Code: "clipboard", Message: "clipboard tool found, so text the layout cannot type can be pasted"}
			}
		}
	}
	return Check{Code: "clipboard", Problem: "optional: " + tool + " is missing, so text with characters the keyboard layout cannot type cannot be pasted"}
}
