package engine

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/cristobaltormo/typedeck/internal/actions"
	"github.com/cristobaltormo/typedeck/internal/capture"
	"github.com/cristobaltormo/typedeck/internal/config"
	"github.com/cristobaltormo/typedeck/internal/hid"
	"github.com/cristobaltormo/typedeck/internal/keylog"
)

type Engine struct {
	paths config.Paths

	mu            sync.Mutex
	cfg           config.Config
	layer         int
	manualLayer   int
	toggleBack    int
	autoActive    bool
	events        []Event
	nextID        int
	sig           chan struct{}
	keys          map[byte]*keyState
	confirm       map[confirmKey]time.Time
	lastFire      map[confirmKey]time.Time
	sem           chan struct{}
	stats         Stats
	statsTimer    *time.Timer
	front         []string
	asn           string
	started       time.Time
	lastKey       string
	lastTS        float64
	stopAwake     func()
	learning      bool
	seen          map[string]int
	wake          chan struct{}
	editUntil     time.Time
	kbdTimer      *time.Timer
	kbdLost       bool
	editorOpen    bool
	pendingNotice bool
	keylog        *keylog.Log
	OnPlainKey    func(usage byte, down bool)

	src capture.Source

	execFn   func(config.Action, bool) actions.Result
	injectFn func(hid.Stroke, int)
	hudFn    func(title, sub string, seconds float64, force bool)
}

func New(paths config.Paths, src capture.Source) (*Engine, error) {
	cfg, note, err := config.LoadWithNote(paths)
	if err != nil {
		return nil, err
	}
	if note != "" {
		fmt.Fprintln(os.Stderr, note)
	}
	e := &Engine{paths: paths, cfg: cfg, src: src, sig: make(chan struct{}), keys: map[byte]*keyState{},
		confirm: map[confirmKey]time.Time{}, lastFire: map[confirmKey]time.Time{}, sem: make(chan struct{}, 8), started: time.Now(), nextID: 1, seen: map[string]int{}, wake: make(chan struct{}, 1)}
	if guarded, lost := e.guardCapture(cfg); lost {
		e.cfg = guarded
		e.pendingNotice = true
		_ = config.Save(paths, guarded, false, false)
	}
	e.stats = e.loadStats()
	e.keylog = keylog.New(paths.Keystrokes())
	e.keylog.SetEnabled(cfg.Settings.KeyHistory)
	e.keylog.SetRetention(time.Duration(cfg.Settings.KeyHistoryDays) * 24 * time.Hour)
	e.execFn = func(a config.Action, capture bool) actions.Result { return actions.Execute(a, e, capture) }
	e.hudFn = func(title, sub string, seconds float64, force bool) { e.showHUD(title, sub, seconds, force) }
	e.injectFn = func(s hid.Stroke, holdMS int) {
		if e.src != nil {
			go func() { _ = e.src.TapHeld(s, holdMS) }()
		}
	}
	if src != nil {
		src.Bind(e.handlers())
	}
	return e, nil
}

func (e *Engine) Settings() config.Settings {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cfg.Settings
}

func (e *Engine) Hardware() actions.Hardware {
	if e.src == nil {
		return nil
	}
	return e.src
}

func (e *Engine) Config() config.Config {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cfg
}

func (e *Engine) SetConfig(c config.Config) (config.Config, error) {
	c, _ = e.guardCapture(c)
	if err := config.Save(e.paths, c, true, false); err != nil {
		return c, err
	}
	loaded, err := config.Load(e.paths)
	if err != nil {
		return c, err
	}
	e.applyConfig(loaded)
	return loaded, nil
}

func (e *Engine) ApplyRestored(c config.Config) config.Config { return e.applyConfig(c) }

func (e *Engine) applyConfig(c config.Config) config.Config {
	c, lost := e.guardCapture(c)
	if lost {
		_ = config.Save(e.paths, c, false, false)
		go e.hudFn(e.tr("capture_consent_lost"), e.tr("capture_consent_lost_sub"), 8, true)
	}
	e.mu.Lock()
	e.cfg = c
	e.keylog.SetEnabled(c.Settings.KeyHistory)
	e.keylog.SetRetention(time.Duration(c.Settings.KeyHistoryDays) * 24 * time.Hour)
	e.asn = ""
	mode := c.Settings.Capture
	if e.layer >= len(c.Layers) {
		e.layer = len(c.Layers) - 1
	}
	if e.manualLayer >= len(c.Layers) {
		e.manualLayer = len(c.Layers) - 1
	}
	e.mu.Unlock()
	select {
	case e.wake <- struct{}{}:
	default:
	}
	e.applyCapture(mode)
	e.syncMaskAsync()
	e.syncKeysAsync()
	return c
}

func (e *Engine) KeyLog() *keylog.Log { return e.keylog }

func (e *Engine) SetKeyHistory(mode string) bool {
	e.mu.Lock()
	cfg := e.cfg
	on := cfg.Settings.KeyHistory
	e.mu.Unlock()
	switch mode {
	case "on":
		on = true
	case "off":
		on = false
	default:
		on = !on
	}
	cfg.Settings.KeyHistory = on
	if _, err := e.SetConfig(cfg); err != nil {
		return !on
	}
	e.emit(Event{Kind: "settings", Name: "key_history", OK: &on})
	e.hudFn(e.tr(map[bool]string{true: "hist_on", false: "hist_off"}[on]), "", 1.6, true)
	return on
}

func (e *Engine) SetEditorOpen(open bool) {
	e.mu.Lock()
	changed := e.editorOpen != open
	e.editorOpen = open
	e.mu.Unlock()
	if changed {
		e.syncKeysAsync()
	}
}

func (e *Engine) wantsAllKeys() bool {
	software := e.softwareMode()
	e.mu.Lock()
	defer e.mu.Unlock()
	if software {
		return e.learning || e.cfg.Settings.KeyHistory
	}
	return e.editorOpen || e.cfg.Settings.KeyHistory
}

func (e *Engine) syncKeysAsync() {
	if e.src == nil {
		return
	}
	want := e.wantsAllKeys()
	go func() { _ = e.src.Keys(want) }()
}

func (e *Engine) Run(stop <-chan struct{}) {
	if e.pendingNotice {
		go e.hudFn(e.tr("capture_consent_lost"), e.tr("capture_consent_lost_sub"), 8, true)
	}
	e.applyCapture(e.Settings().Capture)
	for {
		if e.wantsFrontApp() {
			e.checkFrontApp()
			select {
			case <-time.After(1500 * time.Millisecond):
			case <-e.wake:
			case <-stop:
				e.SaveStats()
				return
			}
			continue
		}
		select {
		case <-e.wake:
		case <-stop:
			e.SaveStats()
			return
		}
	}
}

func (e *Engine) HandleEvent(ev capture.Event) {
	switch ev.Kind {
	case 'D':
		e.KeyDown(ev.Usage, ev.Mods)
	case 'U':
		e.KeyUp(ev.Usage)
	case 'P', 'R':
		e.plainKey(ev.Usage, ev.Kind == 'P')
	case 'W':
		e.mu.Lock()
		if e.learning {
			e.seen[hexKey(ev.Usage)]++
			e.emitLocked(Event{Kind: "watch", Key: hexKey(ev.Usage)})
		}
		e.mu.Unlock()
	case 'K':
		c := ev.Arg == 1
		e.emit(Event{Kind: "keyboard", Connected: &c})
		e.KeyboardChanged(c)
	}
}

var kbdLostAfter = 10 * time.Second

func (e *Engine) KeyboardChanged(present bool) {
	e.mu.Lock()
	if e.kbdTimer != nil {
		e.kbdTimer.Stop()
		e.kbdTimer = nil
	}
	notify := e.cfg.Settings.NotifyKeyboard
	back := present && e.kbdLost
	if present {
		e.kbdLost = false
	} else {
		e.kbdTimer = time.AfterFunc(kbdLostAfter, func() {
			e.mu.Lock()
			e.kbdLost = true
			on := e.cfg.Settings.NotifyKeyboard
			e.mu.Unlock()
			if on {
				e.hudFn(e.tr("kbd_lost"), e.tr("kbd_lost_sub"), 5, true)
			}
		})
	}
	e.mu.Unlock()
	if back && notify {
		e.hudFn(e.tr("kbd_back"), "", 2, true)
	}
}

type Status struct {
	Layer      int      `json:"layer"`
	ManualLyr  int      `json:"manual_layer"`
	Last       string   `json:"last,omitempty"`
	LastTS     float64  `json:"last_ts"`
	Front      []string `json:"front"`
	Uptime     int      `json:"uptime"`
	AutoActive bool     `json:"auto_active"`
	LastID     int      `json:"last_id"`
	Learning   bool     `json:"learning"`
}

func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	front := e.front
	if front == nil {
		front = []string{}
	}
	return Status{Layer: e.layer, ManualLyr: e.manualLayer, Last: e.lastKey, LastTS: e.lastTS, Front: front,
		Uptime: int(time.Since(e.started).Seconds()), AutoActive: e.autoActive, LastID: e.nextID - 1, Learning: e.learning}
}

func (e *Engine) StartLearn() error {
	if e.src == nil || !e.src.Connected() {
		return fmt.Errorf("keyboard capture is not active")
	}
	e.mu.Lock()
	e.learning, e.seen = true, map[string]int{}
	e.mu.Unlock()
	e.syncMaskAsync()
	e.syncKeysAsync()
	return nil
}

func (e *Engine) StopLearn() {
	e.mu.Lock()
	e.learning = false
	e.mu.Unlock()
	e.syncMaskAsync()
	e.syncKeysAsync()
}

func (e *Engine) Seen() map[string]int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return cloneMap(e.seen)
}

func (e *Engine) OnBoardConnected(port string) {
	c := true
	e.emit(Event{Kind: "board", Connected: &c, Port: port, Name: e.CaptureMode()})
	e.syncKeysAsync()
}

func (e *Engine) OnBoardDisconnected() {
	c := false
	e.mu.Lock()
	if e.kbdTimer != nil {
		e.kbdTimer.Stop()
		e.kbdTimer = nil
	}
	for _, st := range e.keys {
		if st.holdTimer != nil {
			st.holdTimer.Stop()
		}
		st.down = false
	}
	e.mu.Unlock()
	e.emit(Event{Kind: "board", Connected: &c, Name: e.CaptureMode()})
}

func (e *Engine) System() actions.SystemInfo { return actions.GetSystemInfo(e) }
