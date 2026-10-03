package actions

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cristobaltormo/typedeck/internal/config"
	"github.com/cristobaltormo/typedeck/internal/hid"
	"github.com/cristobaltormo/typedeck/internal/obs"
	"github.com/cristobaltormo/typedeck/internal/platform"
)

type Result struct {
	OK     bool   `json:"ok"`
	Output string `json:"output"`
	MS     int64  `json:"ms"`
}

type Hardware interface {
	Connected() bool
	Tap(hid.Stroke) error
	Consumer(usage uint16) error
}

type Env interface {
	Settings() config.Settings
	HUD(title, subtitle string, force bool)
	Goto(target any)
	ToggleCaffeinate() bool
	StartTimer(minutes float64, label string)
	Hardware() Hardware
	Layer() int
	LayerName() string
	SetKeyHistory(mode string) bool
}

var consumerKeys = map[string]uint16{"playpause": 0xCD, "next": 0xB5, "prev": 0xB6, "mute": 0xE2, "volup": 0xE9, "voldown": 0xEA}

func Execute(a config.Action, env Env, capture bool) Result {
	t0 := time.Now()
	out, err := dispatch(a, env, capture)
	ms := time.Since(t0).Milliseconds()
	if err != nil {
		msg := err.Error()
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return Result{OK: false, Output: msg, MS: ms}
	}
	if len(out) > 600 {
		out = out[:600]
	}
	return Result{OK: true, Output: out, MS: ms}
}

func run(timeout time.Duration, stdin string, name string, args ...string) (string, error) {
	return platform.Run(timeout, stdin, name, args...)
}

func detached(name string, args ...string) error { return platform.Detached(name, args...) }

func useHardware(env Env) Hardware {
	if env.Settings().Input == "software" {
		return nil
	}
	if hw := env.Hardware(); hw != nil && hw.Connected() {
		return hw
	}
	return nil
}

func dispatch(a config.Action, env Env, capture bool) (string, error) {
	switch a.Type {
	case "app":
		return "", appAction(a)
	case "url":
		u := Substitute(a.URL, false)
		if EditorFocus != nil && isEditorURL(u) && EditorFocus() {
			return "", nil
		}
		return "", platform.Current.OpenURL(u)
	case "shell":
		cmd := Substitute(a.Cmd, true)
		if strings.TrimSpace(cmd) == "" {
			return "", errors.New("empty command")
		}
		sh, args := platform.Current.ShellCommand(cmd)
		if capture || (a.ShowOutput != nil && *a.ShowOutput) {
			return run(20*time.Second, "", sh, args...)
		}
		return "", detached(sh, args...)
	case "ssh":
		if strings.TrimSpace(a.Host) == "" || strings.TrimSpace(a.Cmd) == "" {
			return "", errors.New("the server or the command is missing")
		}
		args := []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=5", a.Host, Substitute(a.Cmd, true)}
		if capture || a.ShowOutput == nil || *a.ShowOutput {
			return run(25*time.Second, "", "ssh", args...)
		}
		return "", detached("ssh", args...)
	case "http":
		return httpAction(a)
	case "hotkey":
		return "", hotkey(a.Keys, env)
	case "obs":
		c := env.Settings().OBS
		return obs.Run(obs.Config{Host: c.Host, Port: c.Port, Password: c.Password}, a.Cmd, Substitute(a.Target, false))
	case "text":
		return "", typeText(Substitute(a.Text, false), env)
	case "media":
		return media(a.Cmd, env)
	case "system":
		return system(a.Cmd, env)
	case "timer":
		env.StartTimer(a.Minutes, a.Label)
		return fmt.Sprintf("%g min", a.Minutes), nil
	case "layer":
		env.Goto(a.To)
		return "", nil
	case "history":
		if env.SetKeyHistory(a.Cmd) {
			return "on", nil
		}
		return "off", nil
	case "hud":
		env.HUD(Substitute(a.Text, false), "", true)
		return "", nil
	case "wait":
		time.Sleep(time.Duration(a.Ms) * time.Millisecond)
		return "", nil
	case "sequence":
		return runSteps(a.Steps, env)
	case "if":
		ok, err := condition(a, env)
		if err != nil {
			return "", err
		}
		if ok {
			return runSteps(a.Steps, env)
		}
		return runSteps(a.Else, env)
	}
	return "", fmt.Errorf("unknown action: %s", a.Type)
}

func runSteps(steps []config.Action, env Env) (string, error) {
	last := ""
	for _, s := range steps {
		r := Execute(s, env, true)
		if !r.OK {
			return "", errors.New(r.Output)
		}
		if r.Output != "" {
			last = r.Output
		}
	}
	return last, nil
}

func condition(a config.Action, env Env) (bool, error) {
	v := strings.TrimSpace(Substitute(a.Target, false))
	var ok bool
	switch a.Cond {
	case "app":
		for _, n := range FrontNames("") {
			if strings.Contains(strings.ToLower(n), strings.ToLower(v)) {
				ok = true
			}
		}
	case "layer":
		cur := env.Layer()
		n, err := strconv.Atoi(v)
		ok = (err == nil && n == cur+1) || (err != nil && strings.EqualFold(v, env.LayerName()))
	case "time":
		ok = inTimeRange(v, time.Now())
	case "os":
		ok = strings.EqualFold(v, platform.Current.Name())
	case "obs_stream", "obs_record":
		c := env.Settings().OBS
		active, err := obs.Active(obs.Config{Host: c.Host, Port: c.Port, Password: c.Password}, strings.TrimPrefix(a.Cond, "obs_"))
		if err != nil {
			return false, err
		}
		ok = active
	case "clipboard":
		ok = v != "" && strings.Contains(strings.ToLower(platform.Current.Clipboard()), strings.ToLower(v))
	default:
		return false, fmt.Errorf("unknown condition: %s", a.Cond)
	}
	return ok != a.Not, nil
}

func inTimeRange(spec string, now time.Time) bool {
	from, to, found := strings.Cut(spec, "-")
	if !found {
		return false
	}
	parse := func(s string) (int, bool) {
		t, err := time.Parse("15:04", strings.TrimSpace(s))
		return t.Hour()*60 + t.Minute(), err == nil
	}
	a, ok1 := parse(from)
	b, ok2 := parse(to)
	if !ok1 || !ok2 {
		return false
	}
	cur := now.Hour()*60 + now.Minute()
	if a <= b {
		return cur >= a && cur < b
	}
	return cur >= a || cur < b
}

func appAction(a config.Action) error {
	if strings.TrimSpace(a.App) == "" {
		return errors.New("the app name is missing")
	}
	switch a.Mode {
	case "quit":
		return platform.Current.QuitApp(a.App)
	case "toggle":
		for _, n := range FrontNames("") {
			if strings.EqualFold(n, a.App) {
				if err := platform.Current.HideApp(a.App); err == nil {
					return nil
				}
				break
			}
		}
	}
	return platform.Current.OpenApp(a.App)
}

func FrontASN() string { return platform.Current.FrontID() }

func FrontNames(asn string) []string { return platform.Current.FrontNames(asn) }

func TypingLayout(env Env) *hid.Layout {
	s := env.Settings().TypingLayout
	if s == "us" || s == "es-iso" || s == "es-pc" || s == "es-win" {
		return hid.LayoutByName(s)
	}
	return hid.LayoutByName(platform.Current.SystemLayout())
}

func adapt(st hid.Stroke, l *hid.Layout) hid.Stroke { return platform.Current.AdaptStroke(st, l) }

func hotkey(spec string, env Env) error {
	l := TypingLayout(env)
	if hw := useHardware(env); hw != nil && l.SafeForKeys() {
		st, err := hid.ParseHotkey(spec, l)
		if err != nil {
			return err
		}
		if err := hw.Tap(adapt(st, l)); err == nil {
			return nil
		}
	}
	return platform.Current.SoftHotkey(spec)
}

func typeText(text string, env Env) error {
	l := TypingLayout(env)
	hw := useHardware(env)
	if hw != nil && l.Typeable(text) {
		failed := false
		for _, r := range text {
			strokes, _ := l.Strokes(r)
			for _, s := range strokes {
				if err := hw.Tap(adapt(s, l)); err != nil {
					failed = true
					break
				}
			}
			if failed {
				break
			}
		}
		if !failed {
			return nil
		}
	}
	return platform.Current.Paste(text)
}

func media(cmd string, env Env) (string, error) {
	if hw := useHardware(env); hw != nil {
		if u, ok := consumerKeys[cmd]; ok {
			if err := hw.Consumer(u); err == nil {
				return "", nil
			}
		}
	}
	return platform.Current.SoftMedia(cmd, env.Settings().VolumeStep)
}

func system(cmd string, env Env) (string, error) {
	switch cmd {
	case "caffeinate":
		if env.ToggleCaffeinate() {
			return "activado", nil
		}
		return "desactivado", nil
	case "lock":
		if st, ok := platform.Current.LockStroke(); ok {
			if hw := useHardware(env); hw != nil && TypingLayout(env).SafeForKeys() {
				if err := hw.Tap(adapt(st, TypingLayout(env))); err == nil {
					return "", nil
				}
			}
			return "", platform.Current.SoftStroke(st)
		}
	case "sleepdisplay", "sleep", "screensaver", "screenshot", "darkmode":
	default:
		return "", fmt.Errorf("unknown system command: %s", cmd)
	}
	return "", platform.Current.System(cmd)
}

var httpClient = &http.Client{Timeout: 8 * time.Second}

func httpAction(a config.Action) (string, error) {
	u := Substitute(a.URL, false)
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return "", errors.New("only http or https")
	}
	var body io.Reader
	if a.Body != "" {
		body = strings.NewReader(Substitute(a.Body, false))
	}
	method := a.Method
	if method == "" {
		method = "GET"
	}
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return "", err
	}
	if a.Body != "" {
		ct := "text/plain"
		if t := strings.TrimSpace(a.Body); strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[") {
			ct = "application/json"
		}
		req.Header.Set("Content-Type", ct)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return fmt.Sprintf("%d %s", resp.StatusCode, strings.TrimSpace(string(b))), nil
}

func shellQuote(s string) string { return platform.Current.QuoteArg(s) }

var EditorURL = func() string { return "http://127.0.0.1:7788/" }

var EditorFocus func() bool

func isEditorURL(u string) bool {
	return strings.TrimRight(u, "/") == strings.TrimRight(EditorURL(), "/")
}

func Substitute(text string, quote bool) string {
	if !strings.Contains(text, "{") {
		return text
	}
	now := time.Now()
	r := strings.NewReplacer(
		"{date}", now.Format("2006-01-02"), "{fecha}", now.Format("2006-01-02"),
		"{time}", now.Format("15:04"), "{hora}", now.Format("15:04"),
		"{datetime}", now.Format("2006-01-02 15:04"))
	text = r.Replace(text)
	text = strings.ReplaceAll(text, "{editor}", EditorURL())
	if strings.Contains(text, "{clipboard}") || strings.Contains(text, "{portapapeles}") {
		c := platform.Current.Clipboard()
		if len(c) > 2000 {
			c = c[:2000]
		}
		if quote {
			c = shellQuote(c)
		}
		text = strings.NewReplacer("{clipboard}", c, "{portapapeles}", c).Replace(text)
	}
	return text
}

func Describe(a *config.Action) string {
	if a == nil {
		return ""
	}
	cut := func(s string, n int) string {
		if r := []rune(s); len(r) > n {
			return string(r[:n])
		}
		return s
	}
	switch a.Type {
	case "app":
		return a.App
	case "url", "http":
		return cut(strings.TrimPrefix(strings.TrimPrefix(a.URL, "https://"), "http://"), 40)
	case "shell":
		return cut(a.Cmd, 40)
	case "ssh":
		return cut(a.Host+": "+a.Cmd, 40)
	case "hotkey":
		return a.Keys
	case "text":
		return cut(a.Text, 30)
	case "media", "system":
		return a.Cmd
	case "history":
		return a.Cmd
	case "obs":
		return strings.TrimSpace("OBS " + a.Cmd + " " + a.Target)
	case "timer":
		return strconv.FormatFloat(a.Minutes, 'f', -1, 64) + " min"
	case "sequence":
		return fmt.Sprintf("%d pasos", len(a.Steps))
	case "if":
		return strings.TrimSpace(a.Cond + " " + cut(a.Target, 24))
	}
	return a.Type
}
