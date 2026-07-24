package engine

import (
	"sync"
	"testing"
	"time"

	"github.com/cristobaltormo/typedeck/internal/actions"
	"github.com/cristobaltormo/typedeck/internal/config"
	"github.com/cristobaltormo/typedeck/internal/hid"
)

type rec struct {
	mu      sync.Mutex
	fired   []string
	huds    []string
	injects []byte
}

func (r *rec) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.fired...)
}

func newTest(t *testing.T, mutate func(*config.Config)) (*Engine, *rec) {
	t.Helper()
	c := config.Default()
	c.Settings.HoldMS, c.Settings.DoubleMS = 200, 120
	c.Layers[0].Keys = map[string]config.KeyDef{}
	if mutate != nil {
		mutate(&c)
	}
	p := config.Paths{Dir: t.TempDir()}
	if err := config.Save(p, c, false, false); err != nil {
		t.Fatal(err)
	}
	e, err := New(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := &rec{}
	e.execFn = func(a config.Action, _ bool) actions.Result {
		if a.Type == "layer" {
			e.Goto(a.To)
			return actions.Result{OK: true}
		}
		r.mu.Lock()
		r.fired = append(r.fired, a.Type+":"+a.Text+a.Cmd)
		r.mu.Unlock()
		return actions.Result{OK: true}
	}
	e.hudFn = func(title, sub string, _ float64, _ bool) { r.mu.Lock(); r.huds = append(r.huds, title); r.mu.Unlock() }
	e.injectFn = func(s hid.Stroke) { r.mu.Lock(); r.injects = append(r.injects, s.Usage); r.mu.Unlock() }
	return e, r
}

func act(text string) *config.Action { return &config.Action{Type: "hud", Text: text} }

func settle() { time.Sleep(60 * time.Millisecond) }

func TestPlainTapFiresOnDown(t *testing.T) {
	e, r := newTest(t, func(c *config.Config) { c.Layers[0].Keys["04"] = config.KeyDef{Tap: act("a")} })
	e.KeyDown(0x04, 0)
	settle()
	if got := r.snapshot(); len(got) != 1 || got[0] != "hud:a" {
		t.Fatalf("disparos: %v", got)
	}
}

func TestHoldVersusTap(t *testing.T) {
	e, r := newTest(t, func(c *config.Config) {
		c.Layers[0].Keys["04"] = config.KeyDef{Tap: act("tap"), Hold: act("hold")}
	})
	e.KeyDown(0x04, 0)
	time.Sleep(30 * time.Millisecond)
	e.KeyUp(0x04)
	settle()
	if got := r.snapshot(); len(got) != 1 || got[0] != "hud:tap" {
		t.Fatalf("pulsación corta: %v", got)
	}
	e.KeyDown(0x04, 0)
	time.Sleep(350 * time.Millisecond)
	e.KeyUp(0x04)
	settle()
	if got := r.snapshot(); len(got) != 2 || got[1] != "hud:hold" {
		t.Fatalf("pulsación larga: %v", got)
	}
}

func TestDoubleVersusSingle(t *testing.T) {
	e, r := newTest(t, func(c *config.Config) {
		c.Layers[0].Keys["05"] = config.KeyDef{Tap: act("tap"), Double: act("double")}
	})
	e.KeyDown(0x05, 0)
	time.Sleep(20 * time.Millisecond)
	e.KeyUp(0x05)
	time.Sleep(40 * time.Millisecond)
	e.KeyDown(0x05, 0)
	time.Sleep(20 * time.Millisecond)
	e.KeyUp(0x05)
	time.Sleep(300 * time.Millisecond)
	if got := r.snapshot(); len(got) != 1 || got[0] != "hud:double" {
		t.Fatalf("doble: %v", got)
	}
	e.KeyDown(0x05, 0)
	time.Sleep(20 * time.Millisecond)
	e.KeyUp(0x05)
	time.Sleep(300 * time.Millisecond)
	if got := r.snapshot(); len(got) != 2 || got[1] != "hud:tap" {
		t.Fatalf("simple: %v", got)
	}
}

func TestGlobalFallbackAndLayerPriority(t *testing.T) {
	e, r := newTest(t, func(c *config.Config) {
		c.Layers[0].Keys["06"] = config.KeyDef{Tap: act("capa")}
		c.Global["06"] = config.KeyDef{Tap: act("global")}
		c.Global["07"] = config.KeyDef{Tap: act("global7")}
	})
	e.KeyDown(0x06, 0)
	e.KeyDown(0x07, 0)
	settle()
	got := r.snapshot()
	if len(got) != 2 {
		t.Fatalf("disparos: %v", got)
	}
	seen := map[string]bool{got[0]: true, got[1]: true}
	if !seen["hud:capa"] || !seen["hud:global7"] || seen["hud:global"] {
		t.Fatalf("prioridad: %v", got)
	}
}

func TestLayerCycleKeyAndMask(t *testing.T) {
	e, _ := newTest(t, func(c *config.Config) {
		c.Layers[1].Keys["08"] = config.KeyDef{Tap: act("solo capa 2")}
	})
	m := e.MaskBytes()
	if m[0x53>>3]&(1<<(0x53&7)) == 0 {
		t.Fatal("la tecla 53 (capa) debe estar capturada")
	}
	if m[0x08>>3]&(1<<(0x08&7)) != 0 {
		t.Fatal("la tecla 08 solo existe en la capa 2")
	}
	e.KeyDown(0x53, 0)
	settle()
	if e.Status().Layer != 1 {
		t.Fatalf("capa: %d", e.Status().Layer)
	}
	m = e.MaskBytes()
	if m[0x08>>3]&(1<<(0x08&7)) == 0 {
		t.Fatal("en la capa 2 la tecla 08 debe capturarse")
	}
	e.KeyUp(0x53)
	e.KeyDown(0x53, 0)
	settle()
	if e.Status().Layer != 0 {
		t.Fatalf("debía volver a la capa 1: %d", e.Status().Layer)
	}
}

func TestConfirmNeedsTwoPresses(t *testing.T) {
	e, r := newTest(t, func(c *config.Config) {
		c.Layers[0].Keys["09"] = config.KeyDef{Tap: &config.Action{Type: "hud", Text: "peligro", Confirm: true}, Label: "Borrar"}
	})
	e.KeyDown(0x09, 0)
	e.KeyUp(0x09)
	settle()
	if len(r.snapshot()) != 0 {
		t.Fatal("la primera pulsación no debe ejecutar")
	}
	e.KeyDown(0x09, 0)
	settle()
	if got := r.snapshot(); len(got) != 1 {
		t.Fatalf("la segunda debe ejecutar: %v", got)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.huds) == 0 || r.huds[0] != "Confirmar" {
		t.Fatalf("falta el aviso de confirmación: %v", r.huds)
	}
}

func TestMomentaryLayerOnHold(t *testing.T) {
	e, r := newTest(t, func(c *config.Config) {
		c.Layers[0].Keys["39"] = config.KeyDef{Hold: &config.Action{Type: "layer", To: 1, Momentary: true}}
		c.Layers[1].Keys["04"] = config.KeyDef{Tap: act("capa2-a")}
	})
	e.KeyDown(0x39, 0)
	time.Sleep(350 * time.Millisecond)
	if e.Status().Layer != 1 {
		t.Fatalf("mientras se mantiene debe estar la capa 2: %d", e.Status().Layer)
	}
	e.KeyDown(0x04, 0)
	e.KeyUp(0x04)
	e.KeyUp(0x39)
	settle()
	if e.Status().Layer != 0 {
		t.Fatalf("al soltar vuelve a la capa 1: %d", e.Status().Layer)
	}
	if got := r.snapshot(); len(got) != 1 || got[0] != "hud:capa2-a" {
		t.Fatalf("disparos: %v", got)
	}
}

func TestHoldOnlyKeyReinjectsOnShortTap(t *testing.T) {
	e, r := newTest(t, func(c *config.Config) {
		c.Layers[0].Keys["39"] = config.KeyDef{Hold: act("mantener")}
	})
	e.KeyDown(0x39, 0)
	time.Sleep(30 * time.Millisecond)
	e.KeyUp(0x39)
	settle()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.injects) != 1 || r.injects[0] != 0x39 {
		t.Fatalf("la tecla 39 debía reenviarse: %v", r.injects)
	}
	if len(r.fired) != 0 {
		t.Fatalf("no debía ejecutar nada: %v", r.fired)
	}
}

func TestUnmappedKeyIsIgnored(t *testing.T) {
	e, r := newTest(t, nil)
	e.KeyDown(0x1A, 0)
	e.KeyUp(0x1A)
	settle()
	if len(r.snapshot()) != 0 {
		t.Fatal("una tecla sin acción no debe ejecutar nada")
	}
}

func TestEventsLongPollAndStats(t *testing.T) {
	e, _ := newTest(t, func(c *config.Config) { c.Layers[0].Keys["0A"] = config.KeyDef{Tap: act("x")} })
	got := make(chan []Event, 1)
	go func() { evs, _ := e.EventsSince(0, 2*time.Second, nil); got <- evs }()
	time.Sleep(50 * time.Millisecond)
	e.KeyDown(0x0A, 0)
	select {
	case evs := <-got:
		if len(evs) == 0 || evs[0].Kind != "down" {
			t.Fatalf("eventos: %+v", evs)
		}
	case <-time.After(time.Second):
		t.Fatal("la conexión larga no despertó")
	}
	settle()
	s := e.Stats()
	if s.Total != 1 || s.Keys["0A"] != 1 {
		t.Fatalf("estadísticas: %+v", s)
	}
	e.ResetStats()
	if e.Stats().Total != 0 {
		t.Fatal("no se puso a cero")
	}
}

func TestApplyConfigClampsLayer(t *testing.T) {
	e, _ := newTest(t, nil)
	e.Goto(1)
	c := e.Config()
	c.Layers = c.Layers[:1]
	if _, err := e.SetConfig(c); err != nil {
		t.Fatal(err)
	}
	if e.Status().Layer != 0 {
		t.Fatal("la capa activa debía acotarse")
	}
}

func TestBounceIsIgnored(t *testing.T) {
	e, r := newTest(t, func(c *config.Config) { c.Layers[0].Keys["04"] = config.KeyDef{Tap: act("a")} })
	e.KeyDown(0x04, 0)
	e.KeyUp(0x04)
	e.KeyDown(0x04, 0)
	settle()
	if got := r.snapshot(); len(got) != 1 {
		t.Fatalf("el rebote no debe ejecutar dos veces: %v", got)
	}
	time.Sleep(80 * time.Millisecond)
	e.KeyDown(0x04, 0)
	settle()
	if got := r.snapshot(); len(got) != 2 {
		t.Fatalf("una pulsación posterior sí: %v", got)
	}
}
