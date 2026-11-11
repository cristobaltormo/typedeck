package capture

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const helperCheckTimeout = 5 * time.Second

func (b *macBackend) Checks() []Check {
	path := b.path()
	if _, err := os.Stat(path); err != nil {
		return []Check{{Problem: (&helperMissingError{path: path}).Error() + "; the macOS archive includes it, or build it with 'make macos-helpers'"}}
	}
	state, err := runHelperCheck(path)
	if err != nil {
		return []Check{{Problem: fmt.Sprintf("%s does not run: %v", HelperName, err)}}
	}
	checks := []Check{{OK: true, Message: HelperName + " found at " + path}}
	checks = append(checks, accessChecks(state, path)...)
	return append(checks, Check{Problem: "optional: keys typed into password fields are hidden from every keyboard tool by macOS (secure input), so macros do not run there"})
}

func accessChecks(state map[string]string, path string) []Check {
	if state["tap"] == "1" {
		return []Check{{OK: true, Message: "macOS lets " + HelperName + " read and take keys: the keyboard tap installs and removes cleanly"}}
	}
	return []Check{
		permissionCheck("Accessibility", state["accessibility"] == "1", path),
		permissionCheck("Input Monitoring", state["input_monitoring"] == "1", path),
		{Problem: "the keyboard tap cannot be installed yet; it needs both permissions above"},
	}
}

func permissionCheck(name string, granted bool, path string) Check {
	if granted {
		return Check{OK: true, Message: name + " is allowed for " + HelperName}
	}
	return Check{Problem: fmt.Sprintf("%s is not allowed: open System Settings > Privacy & Security > %s and turn on %s (use + and choose %s if it is not listed); it is asked for when software capture is first turned on", name, name, HelperName, path)}
}

func runHelperCheck(path string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), helperCheckTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--check").Output()
	if err != nil {
		return nil, err
	}
	return parseCheck(string(out)), nil
}

func parseCheck(out string) map[string]string {
	state := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), " "); ok {
			state[k] = strings.TrimSpace(v)
		}
	}
	return state
}
