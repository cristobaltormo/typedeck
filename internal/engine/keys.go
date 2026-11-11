package engine

import (
	"fmt"
	"strings"
	"time"

	"github.com/cristobaltormo/typedeck/internal/actions"
	"github.com/cristobaltormo/typedeck/internal/config"
	"github.com/cristobaltormo/typedeck/internal/hid"
)

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
	modifier  bool
	used      bool
	downAt    time.Time
	prevLayer int
}

type confirmKey struct {
	key     byte
	gesture string
}

func hexKey(u byte) string { return fmt.Sprintf("%02X", u) }

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

func (e *Engine) pass(u, mods byte) {
	hold := 0
	if u == 0x39 {
		hold = 150
	}
	e.injectFn(hid.Stroke{Usage: u, Mods: mods}, hold)
}

func modifierOf(kd config.KeyDef) *config.Action {
	if isMomentary(kd.Tap) {
		return kd.Tap
	}
	if isMomentary(kd.Hold) {
		return kd.Hold
	}
	return nil
}

func (e *Engine) heldTargetsLocked() []int {
	var out []int
	seen := map[int]bool{}
	visit := func(keys map[string]config.KeyDef) {
		for _, kd := range keys {
			if m := modifierOf(kd); m != nil {
				if i := e.targetLocked(m.To); i != e.layer && !seen[i] {
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
	if e.src != nil {
		go e.src.SyncMask(false)
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
	for k, other := range e.keys {
		if k != u && other.down && other.modifier {
			other.used = true
		}
	}
	if st := e.keys[u]; st != nil && st.down && st.modifier {
		e.mu.Unlock()
		return
	}
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
		e.pass(u, mods)
		return
	}
	if m := modifierOf(kd); m != nil {
		st.modifier, st.momentary, st.prevLayer, st.downAt = true, true, e.layer, time.Now()
		if st.pending != nil {
			st.pending.Stop()
			st.pending = nil
		}
		e.mu.Unlock()
		e.gotoLayer(m.To, false, false)
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
		prev, kd, mods := st.prevLayer, st.kd, st.mods
		alone := st.modifier && !st.used
		st.momentary, st.modifier = false, false
		e.mu.Unlock()
		e.gotoLayer(prev, false, false)
		if alone {
			if kd.Tap != nil && !isMomentary(kd.Tap) {
				e.fire(u, "tap", kd.Tap, kd)
			} else {
				e.pass(u, mods)
			}
		}
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
		e.pass(u, 0)
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
