package platform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

type PopupOptions struct {
	Title, Subtitle string
	Seconds         float64
	Position        string
	Accent          string
	Dots, Active    int
}

type SystemInfo struct {
	TypingLayout string   `json:"typing_layout"`
	KeyboardType string   `json:"keyboard_type"`
	ModRemap     []string `json:"mod_remap"`
	KeysSwapped  bool     `json:"keys_swapped"`
}

type Check struct {
	OK      bool
	Message string
	Problem string
}

var ErrUnsupported = errors.New("no disponible en este sistema")

type Platform interface {
	Name() string
	Version() string

	OpenURL(url string) error
	OpenApp(name string) error
	QuitApp(name string) error
	HideApp(name string) error
	FrontID() string
	FrontNames(id string) []string
	InstalledApps() []string

	SoftHotkey(spec string) error
	SoftStroke(st hid.Stroke) error
	Paste(text string) error
	Clipboard() string
	SoftMedia(cmd string, step int) (string, error)

	System(cmd string) error
	LockStroke() (hid.Stroke, bool)
	KeepAwake() (stop func(), err error)

	Notify(title, body string, sound bool)
	Popup(o PopupOptions) bool

	SystemLayout() string
	AdaptStroke(st hid.Stroke, layout *hid.Layout) hid.Stroke
	Info(layout *hid.Layout) SystemInfo

	ShellCommand(cmd string) (name string, args []string)
	QuoteArg(s string) string
	LogFile() string
	ConfigDir() string
	Doctor() []Check
	InstallService(exe string) (string, error)
	UninstallService() (string, error)
}

var Current Platform = newPlatform()

func Run(timeout time.Duration, stdin string, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	// a child that inherits the output must not keep us waiting after the timeout kills the command
	cmd.WaitDelay = 2 * time.Second
	hideWindow(cmd)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	out := strings.TrimSpace(buf.String())
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("tiempo agotado (%s)", timeout)
	}
	if err != nil {
		if out == "" {
			out = err.Error()
		}
		return out, errors.New(out)
	}
	return out, nil
}

func Detached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = nil, nil
	hideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// Launch starts a program that opens a graphical application without collecting its output: the application would
// inherit the pipe and make us wait until it exits. A quick failure is returned; still running after 3 s counts as launched.
func Launch(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = nil, nil
	hideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		return nil
	}
}

func unixQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func have(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func splitHotkey(spec string) (mods []string, key string, err error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(spec)), "+")
	if len(parts) == 0 || parts[len(parts)-1] == "" {
		return nil, "", errors.New("atajo vacío")
	}
	for _, p := range parts[:len(parts)-1] {
		switch p = strings.TrimSpace(p); p {
		case "cmd", "command", "gui", "win", "super":
			mods = append(mods, "gui")
		case "shift":
			mods = append(mods, "shift")
		case "alt", "option", "opt":
			mods = append(mods, "alt")
		case "ctrl", "control":
			mods = append(mods, "ctrl")
		default:
			return nil, "", fmt.Errorf("modificador desconocido: %s", p)
		}
	}
	key = strings.TrimSpace(parts[len(parts)-1])
	if key == "plus" {
		key = "+"
	}
	return mods, key, nil
}
