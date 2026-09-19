package hud

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cristobaltormo/typedeck/internal/platform"
)

type Options struct {
	Title, Subtitle string
	Seconds         float64
	Position        string
	Accent          string
	Dots, Active    int
	Sound           bool
}

var Binary = func() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "typedeck-hud")
}()

func Show(o Options) {
	if Binary != "" {
		if _, err := os.Stat(Binary); err == nil {
			args := []string{"--title", o.Title, "--sub", o.Subtitle, "--sec", strconv.FormatFloat(o.Seconds, 'f', 1, 64),
				"--pos", o.Position, "--accent", o.Accent, "--dots", strconv.Itoa(o.Dots), "--active", strconv.Itoa(o.Active)}
			cmd := exec.Command(Binary, args...)
			if cmd.Start() == nil {
				go func() { _ = cmd.Wait() }()
			}
			if o.Sound && strings.TrimSpace(o.Title) != "" {
				platform.Current.Notify("", "", true)
			}
			return
		}
	}
	if platform.Current.Popup(platform.PopupOptions{Title: o.Title, Subtitle: o.Subtitle, Seconds: o.Seconds, Position: o.Position, Accent: o.Accent, Dots: o.Dots, Active: o.Active}) {
		if o.Sound && strings.TrimSpace(o.Title) != "" {
			platform.Current.Notify("", "", true)
		}
		return
	}
	body := o.Subtitle
	if body == "" {
		body = o.Title
	}
	platform.Current.Notify(o.Title, body, o.Sound)
}
