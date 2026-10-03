package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/cristobaltormo/typedeck/internal/actions"
	"github.com/cristobaltormo/typedeck/internal/board"
	"github.com/cristobaltormo/typedeck/internal/config"
	"github.com/cristobaltormo/typedeck/internal/hid"
	"github.com/cristobaltormo/typedeck/internal/hud"
	"github.com/cristobaltormo/typedeck/internal/keylog"
	"github.com/cristobaltormo/typedeck/internal/platform"
)

type Event struct {
	ID        int      `json:"id"`
	TS        float64  `json:"ts"`
	Kind      string   `json:"kind"`
	Key       string   `json:"key,omitempty"`
	Gesture   string   `json:"gesture,omitempty"`
	Layer     *int     `json:"layer,omitempty"`
	LayerName string   `json:"layer_name,omitempty"`
	Name      string   `json:"name,omitempty"`
	Auto      bool     `json:"auto,omitempty"`
	Label     string   `json:"label,omitempty"`
	Type      string   `json:"type,omitempty"`
	OK        *bool    `json:"ok,omitempty"`
	MS        int64    `json:"ms,omitempty"`
	Output    string   `json:"output,omitempty"`
	Names     []string `json:"names,omitempty"`
	Connected *bool    `json:"connected,omitempty"`
	Port      string   `json:"port,omitempty"`
}

type Stats struct {
	Since  string         `json:"since"`
	Total  int            `json:"total"`
	Keys   map[string]int `json:"keys"`
	Layers map[string]int `json:"layers"`
	Types  map[string]int `json:"types"`
	Days   map[string]int `json:"days"`
}

func newStats() Stats {
	return Stats{Since: time.Now().Format("2006-01-02"), Keys: map[string]int{}, Layers: map[string]int{}, Types: map[string]int{}, Days: map[string]int{}}
}

type keyState struct {
	kd        config.KeyDef
	known     bool
	down      bool
	done      bool
	holdFired bool
	mods      byte
	holdTimer *time.Timer
	pending   *time.Timer
	momentary bool
	prevLayer int
}

type confirmKey struct {
	key     byte
	gesture string
}

type Engine struct {
	paths config.Paths

	mu          sync.Mutex
	cfg         config.Config
	layer       int
	manualLayer int
	toggleBack  int
	autoActive  bool
	events      []Event
	nextID      int
	sig         chan struct{}
	keys        map[byte]*keyState
	confirm     map[confirmKey]time.Time
	lastFire    map[confirmKey]time.Time
	sem         chan struct{}
	stats       Stats
	statsTimer  *time.Timer
	front       []string
	asn         string
	started     time.Time
	lastKey     string
	lastTS      float64
	stopAwake   func()
	learning    bool
	seen        map[string]int
	wake        chan struct{}
	editUntil   time.Time
	kbdTimer    *time.Timer
	kbdLost     bool
	editorOpen  bool
	keylog      *keylog.Log
	OnPlainKey  func(usage byte, down bool)

	board *board.Board

	execFn   func(config.Action, bool) actions.Result
	injectFn func(hid.Stroke)
	hudFn    func(title, sub string, seconds float64, force bool)
}

func New(paths config.Paths, brd *board.Board) (*Engine, error) {
	cfg, note, err := config.LoadWithNote(paths)
	if err != nil {
		return nil, err
	}
	if note != "" {
		fmt.Fprintln(os.Stderr, note)
	}
	e := &Engine{paths: paths, cfg: cfg, board: brd, sig: make(chan struct{}), keys: map[byte]*keyState{},
		confirm: map[confirmKey]time.Time{}, lastFire: map[confirmKey]time.Time{}, sem: make(chan struct{}, 8), started: time.Now(), nextID: 1, seen: map[string]int{}, wake: make(chan struct{}, 1)}
	e.stats = e.loadStats()
	e.keylog = keylog.New(paths.Keystrokes())
	e.keylog.SetEnabled(cfg.Settings.KeyHistory)
	e.keylog.SetRetention(time.Duration(cfg.Settings.KeyHistoryDays) * 24 * time.Hour)
	e.execFn = func(a config.Action, capture bool) actions.Result { return actions.Execute(a, e, capture) }
	e.hudFn = func(title, sub string, seconds float64, force bool) { e.showHUD(title, sub, seconds, force) }
	e.injectFn = func(s hid.Stroke) {
		if e.board != nil {
			go func() { _ = e.board.Tap(s) }()
		}
	}
	return e, nil
}

func hexKey(u byte) string { return fmt.Sprintf("%02X", u) }

func (e *Engine) emitLocked(ev Event) {
	ev.ID = e.nextID
	ev.TS = float64(time.Now().UnixNano()) / 1e9
	e.nextID++
	e.events = append(e.events, ev)
	if len(e.events) > 400 {
		e.events = e.events[len(e.events)-400:]
	}
	close(e.sig)
	e.sig = make(chan struct{})
}

func (e *Engine) emit(ev Event) {
	e.mu.Lock()
	e.emitLocked(ev)
	e.mu.Unlock()
}

func (e *Engine) EventsSince(id int, wait time.Duration, cancel <-chan struct{}) ([]Event, int) {
	deadline := time.After(wait)
	for {
		e.mu.Lock()
		var out []Event
		for _, ev := range e.events {
			if ev.ID > id {
				out = append(out, ev)
			}
		}
		last := e.nextID - 1
		sig := e.sig
		e.mu.Unlock()
		if len(out) > 0 || wait <= 0 {
			if out == nil {
				out = []Event{}
			}
			return out, last
		}
		select {
		case <-sig:
		case <-deadline:
			return []Event{}, last
		case <-cancel:
			return []Event{}, last
		}
	}
}

func (e *Engine) Settings() config.Settings {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cfg.Settings
}

func (e *Engine) Hardware() actions.Hardware {
	if e.board == nil {
		return nil
	}
	return e.board
}

func (e *Engine) Layer() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.layer
}

func (e *Engine) LayerName() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.layer < len(e.cfg.Layers) {
		return e.cfg.Layers[e.layer].Name
	}
	return ""
}

func (e *Engine) accentLocked(layer int) string {
	if layer >= 0 && layer < len(e.cfg.Layers) && e.cfg.Layers[layer].Color != "" {
		return e.cfg.Layers[layer].Color
	}
	return e.cfg.Settings.Accent
}

var strings_es = map[string]map[string]string{
	"es": {"layer_of": "Capa %d de %d", "confirm": "Confirmar", "press_again": "Pulsa otra vez: %s", "error": "Error",
		"caff_on": "Mac despierto", "caff_off": "Mac puede dormir", "timer_start": "Temporizador", "timer_end": "Tiempo cumplido",
		"kbd_lost": "Teclado no detectado", "kbd_lost_sub": "Desenchufa el teclado del shield y vuelve a enchufarlo", "kbd_back": "Teclado conectado", "hist_on": "Historial de tecleo activado", "hist_off": "Historial de tecleo desactivado"},
	"en": {"layer_of": "Layer %d of %d", "confirm": "Confirm", "press_again": "Press again: %s", "error": "Error",
		"caff_on": "Mac awake", "caff_off": "Mac can sleep", "timer_start": "Timer", "timer_end": "Time is up",
		"kbd_lost": "Keyboard not detected", "kbd_lost_sub": "Unplug the keyboard from the shield and plug it back in", "kbd_back": "Keyboard connected", "hist_on": "Typing history on", "hist_off": "Typing history off"},
}

func (e *Engine) tr(key string, args ...any) string {
	e.mu.Lock()
	lang := e.cfg.Settings.Language
	e.mu.Unlock()
	f := strings_es[lang][key]
	if f == "" {
		f = strings_es["en"][key]
	}
	if len(args) == 0 {
		return f
	}
	return fmt.Sprintf(f, args...)
}

func (e *Engine) HUD(title, subtitle string, force bool) { e.hudFn(title, subtitle, 0, force) }

func (e *Engine) showHUD(title, sub string, seconds float64, force bool) {
	e.mu.Lock()
	h := e.cfg.Settings.HUD
	layers, cur := len(e.cfg.Layers), e.layer
	accent := e.accentLocked(e.layer)
	e.mu.Unlock()
	if !h.Enabled && !force {
		return
	}
	if seconds <= 0 {
		seconds = h.Seconds
	}
	dots := 0
	if strings.Contains(sub, "\x00dots") {
		sub = strings.ReplaceAll(sub, "\x00dots", "")
		dots = layers
	}
	hud.Show(hud.Options{Title: title, Subtitle: sub, Seconds: seconds, Position: h.Position, Accent: accent, Dots: dots, Active: cur, Sound: h.Sound})
}

func (e *Engine) ToggleCaffeinate() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stopAwake != nil {
		e.stopAwake()
		e.stopAwake = nil
		go e.hudFn(e.trLocked("caff_off"), "", 0, true)
		return false
	}
	stop, err := platform.Current.KeepAwake()
	if err != nil {
		return false
	}
	e.stopAwake = stop
	go e.hudFn(e.trLocked("caff_on"), "", 0, true)
	return true
}

func (e *Engine) trLocked(key string) string {
	f := strings_es[e.cfg.Settings.Language][key]
	if f == "" {
		f = strings_es["en"][key]
	}
	return f
}

func (e *Engine) StartTimer(minutes float64, label string) {
	sub := fmt.Sprintf("%g min", minutes)
	if label != "" {
		sub += " - " + label
	}
	go e.hudFn(e.tr("timer_start"), sub, 0, true)
	time.AfterFunc(time.Duration(minutes*float64(time.Minute)), func() {
		e.hudFn(e.tr("timer_end"), label, 3, true)
		_ = exec.Command("afplay", "/System/Library/Sounds/Glass.aiff").Start()
	})
}

func (e *Engine) Config() config.Config {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cfg
}

func (e *Engine) SetConfig(c config.Config) (config.Config, error) {
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

func (e *Engine) ApplyRestored(c config.Config) { e.applyConfig(c) }

func (e *Engine) applyConfig(c config.Config) {
	e.mu.Lock()
	e.cfg = c
	e.keylog.SetEnabled(c.Settings.KeyHistory)
	e.keylog.SetRetention(time.Duration(c.Settings.KeyHistoryDays) * 24 * time.Hour)
	// new per-application rules must be evaluated against the window that is already in front
	e.asn = ""
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
	e.syncMaskAsync()
	e.syncKeysAsync()
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

func (e *Engine) syncKeysAsync() {
	if e.board == nil {
		return
	}
	e.mu.Lock()
	want := e.editorOpen || e.cfg.Settings.KeyHistory
	e.mu.Unlock()
	go func() { _ = e.board.Keys(want) }()
}

func (e *Engine) plainKey(u byte, down bool) {
	now := time.Now()
	if down {
		e.keylog.Press(u, now)
	} else {
		e.keylog.Release(u, now)
	}
	if e.OnPlainKey != nil {
		e.OnPlainKey(u, down)
	}
}

func (e *Engine) MaskBytes() [32]byte {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.maskLocked()
}

func (e *Engine) maskLocked() [32]byte {
	var m [32]byte
	if time.Now().Before(e.editUntil) {
		return m
	}
	if e.learning {
		for u := 0x04; u < 0xE0; u++ {
			m[u>>3] |= 1 << (u & 7)
		}
		return m
	}
	add := func(keys map[string]config.KeyDef) {
		for id, kd := range keys {
			if !kd.HasAction() {
				continue
			}
			var u byte
			if _, err := fmt.Sscanf(id, "%02X", &u); err == nil {
				m[u>>3] |= 1 << (u & 7)
			}
		}
	}
	add(e.cfg.Global)
	if e.layer < len(e.cfg.Layers) {
		add(e.cfg.Layers[e.layer].Keys)
	}
	for _, i := range e.heldTargetsLocked() {
		add(e.cfg.Layers[i].Keys)
	}
	for u, st := range e.keys {
		if st.down && st.known {
			m[u>>3] |= 1 << (u & 7)
		}
	}
	return m
}

func isMomentary(a *config.Action) bool {
	return a != nil && a.Type == "layer" && a.Momentary
}

func (e *Engine) heldTargetsLocked() []int {
	var out []int
	seen := map[int]bool{}
	visit := func(keys map[string]config.KeyDef) {
		for _, kd := range keys {
			if isMomentary(kd.Hold) {
				if i := e.targetLocked(kd.Hold.To); i != e.layer && !seen[i] {
					seen[i] = true
					out = append(out, i)
				}
			}
		}
	}
	visit(e.cfg.Global)
	if e.layer < len(e.cfg.Layers) {
		visit(e.cfg.Layers[e.layer].Keys)
	}
	return out
}

const editHold = 8 * time.Second

func (e *Engine) Editing() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return time.Now().Before(e.editUntil)
}

func (e *Engine) SetEditing(on bool) {
	e.mu.Lock()
	was := time.Now().Before(e.editUntil)
	if on {
		e.editUntil = time.Now().Add(editHold)
		time.AfterFunc(editHold+50*time.Millisecond, e.syncMaskAsync)
	} else {
		e.editUntil = time.Time{}
	}
	e.mu.Unlock()
	if was != on {
		e.syncMaskAsync()
	}
}

func (e *Engine) syncMaskAsync() {
	if e.board != nil {
		go e.board.SyncMask(false)
	}
}

func (e *Engine) Goto(target any) { e.gotoLayer(target, true, true) }

func (e *Engine) targetLocked(target any) int {
	n := len(e.cfg.Layers)
	idx := e.layer
	switch v := target.(type) {
	case string:
		if v == "prev" {
			idx = (e.layer - 1 + n) % n
		} else {
			idx = (e.layer + 1) % n
		}
	case int:
		idx = max(0, min(v, n-1))
	case float64:
		idx = max(0, min(int(v), n-1))
	}
	return idx
}

func (e *Engine) ToggleLayer(target any) {
	e.mu.Lock()
	idx := e.targetLocked(target)
	back := idx
	if e.layer == idx {
		back = e.toggleBack
		if back == idx || back >= len(e.cfg.Layers) {
			back = 0
		}
	} else {
		e.toggleBack = e.layer
	}
	e.mu.Unlock()
	e.gotoLayer(back, true, true)
}

func (e *Engine) gotoLayer(target any, manual, announce bool) {
	e.mu.Lock()
	n := len(e.cfg.Layers)
	idx := e.targetLocked(target)
	e.layer = idx
	if manual {
		e.manualLayer = idx
		e.autoActive = false
	}
	name := e.cfg.Layers[idx].Name
	i := idx
	e.emitLocked(Event{Kind: "layer", Layer: &i, Name: name, Auto: !manual})
	e.mu.Unlock()
	if e.board != nil {
		go func() { _, _ = e.board.Command("layer", i) }()
	}
	e.syncMaskAsync()
	if announce {
		e.hudFn(name, e.tr("layer_of", idx+1, n)+"\x00dots", 0, false)
	}
}

func (e *Engine) wantsFrontApp() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.cfg.Settings.AutoLayer {
		return false
	}
	for _, l := range e.cfg.Layers {
		if len(l.AutoApps) > 0 {
			return true
		}
	}
	return false
}

func (e *Engine) checkFrontApp() {
	asn := actions.FrontASN()
	e.mu.Lock()
	if asn == e.asn {
		e.mu.Unlock()
		return
	}
	e.asn = asn
	e.mu.Unlock()
	names := actions.FrontNames(asn)
	e.mu.Lock()
	e.front = names
	e.emitLocked(Event{Kind: "front", Names: names})
	low := map[string]bool{}
	for _, n := range names {
		low[strings.ToLower(n)] = true
	}
	match := -1
	for i, l := range e.cfg.Layers {
		for _, a := range l.AutoApps {
			if low[strings.ToLower(a)] {
				match = i
			}
		}
		if match >= 0 {
			break
		}
	}
	announce := e.cfg.Settings.HUD.OnAuto
	cur, manual, auto := e.layer, e.manualLayer, e.autoActive
	e.mu.Unlock()
	switch {
	case match >= 0 && match != cur:
		e.mu.Lock()
		e.autoActive = true
		e.mu.Unlock()
		e.gotoLayer(match, false, announce)
	case match < 0 && auto:
		e.mu.Lock()
		e.autoActive = false
		e.mu.Unlock()
		if cur != manual {
			e.gotoLayer(manual, false, announce)
		}
	}
}

func (e *Engine) Run(stop <-chan struct{}) {
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

func (e *Engine) resolveLocked(id string) (config.KeyDef, bool) {
	if e.layer < len(e.cfg.Layers) {
		if kd, ok := e.cfg.Layers[e.layer].Keys[id]; ok && kd.HasAction() {
			return kd, true
		}
	}
	kd, ok := e.cfg.Global[id]
	return kd, ok && kd.HasAction()
}

func (e *Engine) HandleBoardEvent(ev board.Event) {
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

func (e *Engine) KeyDown(u byte, mods byte) {
	e.keylog.Press(u, time.Now())
	id := hexKey(u)
	e.mu.Lock()
	if e.learning {
		e.seen[id]++
		e.emitLocked(Event{Kind: "watch", Key: id})
		e.mu.Unlock()
		return
	}
	var early []*keyState
	for k, other := range e.keys {
		if k != u && other.down && !other.holdFired && isMomentary(other.kd.Hold) {
			other.holdFired, other.momentary, other.prevLayer = true, true, e.layer
			if other.holdTimer != nil {
				other.holdTimer.Stop()
			}
			early = append(early, other)
		}
	}
	e.mu.Unlock()
	for _, o := range early {
		e.gotoLayer(o.kd.Hold.To, false, false)
	}
	e.mu.Lock()
	e.lastKey, e.lastTS = id, float64(time.Now().UnixNano())/1e9
	e.emitLocked(Event{Kind: "down", Key: id})
	kd, ok := e.resolveLocked(id)
	st := e.keys[u]
	if st == nil {
		st = &keyState{}
		e.keys[u] = st
	}
	if st.holdTimer != nil {
		st.holdTimer.Stop()
	}
	*st = keyState{kd: kd, known: ok, down: true, mods: mods, pending: st.pending}
	if !ok {
		st.done = true
		e.mu.Unlock()
		e.injectFn(hid.Stroke{Usage: u, Mods: mods})
		return
	}
	hasHold, hasDouble := kd.Hold != nil, kd.Double != nil
	if st.pending != nil {
		st.pending.Stop()
		st.pending = nil
		st.done = true
		e.mu.Unlock()
		e.fire(u, "double", kd.Double, kd)
		return
	}
	switch {
	case !hasHold && !hasDouble:
		st.done = true
		e.mu.Unlock()
		e.fire(u, "tap", kd.Tap, kd)
	case hasHold:
		d := time.Duration(e.cfg.Settings.HoldMS) * time.Millisecond
		st.holdTimer = time.AfterFunc(d, func() { e.holdElapsed(u) })
		e.mu.Unlock()
	default:
		e.mu.Unlock()
	}
}

func (e *Engine) holdElapsed(u byte) {
	e.mu.Lock()
	st := e.keys[u]
	if st == nil || !st.down || st.holdFired {
		e.mu.Unlock()
		return
	}
	st.holdFired = true
	kd := st.kd
	if kd.Hold != nil && kd.Hold.Type == "layer" && kd.Hold.Momentary {
		st.momentary, st.prevLayer = true, e.layer
		e.mu.Unlock()
		e.gotoLayer(kd.Hold.To, false, false)
		return
	}
	e.mu.Unlock()
	e.fire(u, "hold", kd.Hold, kd)
}

func (e *Engine) KeyUp(u byte) {
	e.keylog.Release(u, time.Now())
	e.mu.Lock()
	e.emitLocked(Event{Kind: "up", Key: hexKey(u)})
	st := e.keys[u]
	if st == nil || !st.down {
		e.mu.Unlock()
		return
	}
	st.down = false
	if st.holdTimer != nil {
		st.holdTimer.Stop()
	}
	if st.momentary {
		prev := st.prevLayer
		st.momentary = false
		e.mu.Unlock()
		e.gotoLayer(prev, false, false)
		return
	}
	if st.done || st.holdFired {
		e.mu.Unlock()
		return
	}
	kd := st.kd
	if kd.Double != nil {
		d := time.Duration(e.cfg.Settings.DoubleMS) * time.Millisecond
		st.pending = time.AfterFunc(d, func() { e.tapAfterWait(u) })
		e.mu.Unlock()
		return
	}
	e.mu.Unlock()
	e.tap(u, kd)
}

func (e *Engine) tapAfterWait(u byte) {
	e.mu.Lock()
	st := e.keys[u]
	if st == nil || st.pending == nil {
		e.mu.Unlock()
		return
	}
	st.pending = nil
	kd := st.kd
	e.mu.Unlock()
	e.tap(u, kd)
}

func (e *Engine) tap(u byte, kd config.KeyDef) {
	if kd.Tap == nil {
		e.injectFn(hid.Stroke{Usage: u})
		return
	}
	e.fire(u, "tap", kd.Tap, kd)
}

func (e *Engine) fire(u byte, gesture string, a *config.Action, kd config.KeyDef) {
	if a == nil {
		return
	}
	label := kd.Label
	if label == "" {
		label = actions.Describe(a)
	}
	if label == "" {
		label = hexKey(u)
	}
	e.mu.Lock()
	fk := confirmKey{u, gesture}
	if time.Since(e.lastFire[fk]) < 60*time.Millisecond {
		e.mu.Unlock()
		return
	}
	e.lastFire[fk] = time.Now()
	e.mu.Unlock()
	if a.Confirm {
		ck := confirmKey{u, gesture}
		e.mu.Lock()
		last := e.confirm[ck]
		now := time.Now()
		if now.Sub(last) > 3*time.Second {
			e.confirm[ck] = now
			e.mu.Unlock()
			e.hudFn(e.tr("confirm"), e.tr("press_again", label), 2.5, true)
			return
		}
		e.confirm[ck] = time.Time{}
		e.mu.Unlock()
	}
	go e.runAction(u, gesture, *a, label)
}

func (e *Engine) runAction(u byte, gesture string, a config.Action, label string) {
	select {
	case e.sem <- struct{}{}:
		defer func() { <-e.sem }()
	case <-time.After(3 * time.Second):
		e.hudFn(e.tr("error"), "too many actions at once", 2, true)
		return
	}
	e.mu.Lock()
	layerName := e.cfg.Layers[min(e.layer, len(e.cfg.Layers)-1)].Name
	e.mu.Unlock()
	showsOutput := (a.Type == "shell" || a.Type == "ssh") && ((a.ShowOutput != nil && *a.ShowOutput) || (a.Type == "ssh" && a.ShowOutput == nil)) || a.Type == "http"
	res := e.execFn(a, showsOutput)
	ok := res.OK
	e.emit(Event{Kind: "exec", Key: hexKey(u), Gesture: gesture, LayerName: layerName, Label: label, Type: a.Type, OK: &ok, MS: res.MS, Output: res.Output})
	e.count(hexKey(u), layerName, a.Type)
	e.mu.Lock()
	onAction := e.cfg.Settings.HUD.OnAction
	e.mu.Unlock()
	switch {
	case !res.OK:
		msg := res.Output
		if msg == "" {
			msg = label
		}
		e.hudFn(e.tr("error"), clip(msg, 60), 2.2, true)
	case showsOutput:
		first := ""
		if res.Output != "" {
			first = clip(strings.SplitN(res.Output, "\n", 2)[0], 60)
		}
		e.hudFn(label, first, 2.4, true)
	case onAction && a.Type != "layer" && a.Type != "hud":
		e.hudFn(label, clip(res.Output, 40), 0, false)
	}
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func (e *Engine) Test(a config.Action) actions.Result {
	res := e.execFn(a, true)
	ok := res.OK
	e.emit(Event{Kind: "test", Type: a.Type, OK: &ok, MS: res.MS, Output: res.Output})
	return res
}

func (e *Engine) loadStats() Stats {
	s := newStats()
	if b, err := os.ReadFile(e.paths.Stats()); err == nil {
		var in Stats
		if json.Unmarshal(b, &in) == nil && in.Keys != nil {
			s = in
			for _, m := range []*map[string]int{&s.Layers, &s.Types, &s.Days} {
				if *m == nil {
					*m = map[string]int{}
				}
			}
		}
	}
	return s
}

func (e *Engine) count(key, layer, atype string) {
	e.mu.Lock()
	s := &e.stats
	s.Total++
	s.Keys[key]++
	s.Layers[layer]++
	s.Types[atype]++
	day := time.Now().Format("2006-01-02")
	s.Days[day]++
	if len(s.Days) > 60 {
		oldest := ""
		for d := range s.Days {
			if oldest == "" || d < oldest {
				oldest = d
			}
		}
		delete(s.Days, oldest)
	}
	if e.statsTimer == nil {
		e.statsTimer = time.AfterFunc(30*time.Second, e.SaveStats)
	}
	e.mu.Unlock()
}

func (e *Engine) SaveStats() {
	e.mu.Lock()
	e.statsTimer = nil
	data, _ := json.Marshal(e.stats)
	e.mu.Unlock()
	_ = os.MkdirAll(e.paths.Dir, 0o755)
	tmp := e.paths.Stats() + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Rename(tmp, e.paths.Stats())
	}
}

func (e *Engine) Stats() Stats {
	e.mu.Lock()
	defer e.mu.Unlock()
	cp := e.stats
	cp.Keys, cp.Layers, cp.Types, cp.Days = cloneMap(e.stats.Keys), cloneMap(e.stats.Layers), cloneMap(e.stats.Types), cloneMap(e.stats.Days)
	return cp
}

func cloneMap(m map[string]int) map[string]int {
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (e *Engine) ResetStats() {
	e.mu.Lock()
	e.stats = newStats()
	e.mu.Unlock()
	e.SaveStats()
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
	if e.board == nil {
		return fmt.Errorf("board not connected")
	}
	e.mu.Lock()
	e.learning, e.seen = true, map[string]int{}
	e.mu.Unlock()
	e.syncMaskAsync()
	return nil
}

func (e *Engine) StopLearn() {
	e.mu.Lock()
	e.learning = false
	e.mu.Unlock()
	e.syncMaskAsync()
}

func (e *Engine) Seen() map[string]int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return cloneMap(e.seen)
}

func (e *Engine) OnBoardConnected(port string) {
	c := true
	e.emit(Event{Kind: "board", Connected: &c, Port: port})
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
	e.emit(Event{Kind: "board", Connected: &c})
}

func (e *Engine) System() actions.SystemInfo { return actions.GetSystemInfo(e) }
