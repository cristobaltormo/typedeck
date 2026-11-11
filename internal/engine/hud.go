package engine

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/cristobaltormo/typedeck/internal/hud"
	"github.com/cristobaltormo/typedeck/internal/platform"
)

func (e *Engine) accentLocked(layer int) string {
	if layer >= 0 && layer < len(e.cfg.Layers) && e.cfg.Layers[layer].Color != "" {
		return e.cfg.Layers[layer].Color
	}
	return e.cfg.Settings.Accent
}

var strings_es = map[string]map[string]string{
	"es": {"layer_of": "Capa %d de %d", "confirm": "Confirmar", "press_again": "Pulsa otra vez: %s", "error": "Error",
		"caff_on": "Mac despierto", "caff_off": "Mac puede dormir", "timer_start": "Temporizador", "timer_end": "Tiempo cumplido",
		"kbd_lost": "Teclado no detectado", "kbd_lost_sub": "Desenchufa el teclado del shield y vuelve a enchufarlo", "kbd_back": "Teclado conectado", "hist_on": "Historial de tecleo activado", "hist_off": "Historial de tecleo desactivado",
		"capture_failed": "Captura por software no disponible", "capture_failed_sub": "Se usa la placa; revisa los permisos en el editor",
		"capture_consent_lost": "Captura por software desactivada", "capture_consent_lost_sub": "Esta instalación nunca la ha aceptado: actívala desde el editor"},
	"en": {"layer_of": "Layer %d of %d", "confirm": "Confirm", "press_again": "Press again: %s", "error": "Error",
		"caff_on": "Mac awake", "caff_off": "Mac can sleep", "timer_start": "Timer", "timer_end": "Time is up",
		"kbd_lost": "Keyboard not detected", "kbd_lost_sub": "Unplug the keyboard from the shield and plug it back in", "kbd_back": "Keyboard connected", "hist_on": "Typing history on", "hist_off": "Typing history off",
		"capture_failed": "Software capture unavailable", "capture_failed_sub": "Using the board; check the permissions in the editor",
		"capture_consent_lost": "Software capture turned off", "capture_consent_lost_sub": "This installation never agreed to it: turn it on from the editor"},
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
