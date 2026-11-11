package engine

import (
	"strings"

	"github.com/cristobaltormo/typedeck/internal/actions"
	"github.com/cristobaltormo/typedeck/internal/capture"
)

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
	if l, ok := e.src.(capture.LayerSetter); ok {
		go l.SetLayer(i)
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
