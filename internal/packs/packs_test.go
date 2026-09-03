package packs

import (
	"testing"

	"github.com/cristobaltormo/typedeck/internal/config"
	"github.com/cristobaltormo/typedeck/internal/layouts"
)

func TestCatalogIsConsistent(t *testing.T) {
	cats := map[string]bool{}
	for _, c := range Categories() {
		if c.Name["es"] == "" || c.Name["en"] == "" {
			t.Errorf("categoría %s sin traducir", c.ID)
		}
		cats[c.ID] = true
	}
	ids := map[string]bool{}
	for _, p := range All() {
		if ids[p.ID] {
			t.Errorf("paquete repetido: %s", p.ID)
		}
		ids[p.ID] = true
		if !cats[p.Category] {
			t.Errorf("%s: categoría inexistente %q", p.ID, p.Category)
		}
		for _, f := range []Text{p.Name, p.Description} {
			if f["es"] == "" || f["en"] == "" {
				t.Errorf("%s: falta un idioma", p.ID)
			}
		}
		if p.Notes != nil && (p.Notes["es"] == "" || p.Notes["en"] == "") {
			t.Errorf("%s: nota sin los dos idiomas", p.ID)
		}
		if n := len(p.Macros); n < 3 || n > 15 {
			t.Errorf("%s: %d macros (3 a 15)", p.ID, n)
		}
		if regions[p.Region] == nil {
			t.Errorf("%s: región %q", p.ID, p.Region)
		}
		for i, m := range p.Macros {
			if m.Label["es"] == "" || m.Label["en"] == "" {
				t.Errorf("%s macro %d sin etiqueta en los dos idiomas", p.ID, i)
			}
			if m.Tap == nil && m.Hold == nil && m.Double == nil {
				t.Errorf("%s macro %d sin acción", p.ID, i)
			}
		}
	}
	if len(All()) < 30 {
		t.Errorf("la galería debía tener muchos paquetes, hay %d", len(All()))
	}
}

func TestEveryPackFitsEveryKeyboard(t *testing.T) {
	reserved := map[string]bool{"53": true, "58": true}
	for _, p := range All() {
		for _, l := range layouts.All() {
			r, err := Resolve(p, l.ID, "es", reserved)
			if err != nil {
				t.Fatal(err)
			}
			placed := len(r.Layer.Keys)
			if placed == 0 {
				t.Errorf("%s en %s: no colocó nada", p.ID, l.ID)
			}
			if placed+r.Dropped != len(p.Macros) {
				t.Errorf("%s en %s: %d colocadas + %d descartadas != %d", p.ID, l.ID, placed, r.Dropped, len(p.Macros))
			}
			if r.Dropped > len(p.Macros)/2 && l.ID != "numpad" {
				t.Errorf("%s en %s descarta demasiadas (%d de %d)", p.ID, l.ID, r.Dropped, len(p.Macros))
			}
			for id := range r.Layer.Keys {
				if reserved[id] {
					t.Errorf("%s en %s: ocupó la tecla reservada %s", p.ID, l.ID, id)
				}
			}
			cfg := config.Default()
			cfg.Layers = append(cfg.Layers, r.Layer)
			for k, kd := range r.Global {
				cfg.Global[k] = kd
			}
			if _, err := config.Validate(cfg); err != nil {
				t.Errorf("%s en %s: configuración inválida: %v", p.ID, l.ID, err)
			}
		}
	}
}

func TestPlacementAdaptsToTheKeyboard(t *testing.T) {
	p, _ := Get("zoom")
	r, _ := Resolve(p, "full-iso", "es", nil)
	if r.Region != "numpad" {
		t.Errorf("con pad numérico debía usar el pad: %s", r.Region)
	}
	r, _ = Resolve(p, "60-ansi", "es", nil)
	if r.Region != "numrow" && r.Region != "letters" {
		t.Errorf("un 60%% no tiene pad ni fila F: %s", r.Region)
	}
	r, _ = Resolve(p, "tkl-ansi", "en", nil)
	if r.Region != "fn" {
		t.Errorf("un TKL sin pad debe usar la fila de funciones: %s", r.Region)
	}
	if r.Layer.Name != "Zoom" {
		t.Errorf("nombre: %q", r.Layer.Name)
	}
}

func TestMnemonicKeysAreRespected(t *testing.T) {
	p, _ := Get("launcher")
	r, _ := Resolve(p, "full-iso", "es", nil)
	if kd, ok := r.Layer.Keys["06"]; !ok || kd.Tap.App != "Google Chrome" {
		t.Errorf("Chrome debe estar en la C: %+v", r.Layer.Keys["06"])
	}
	if _, ok := r.Global["39"]; !ok {
		t.Error("el lanzador necesita la tecla Bloq Mayús global")
	}
}

func TestSpecificAppsAreCovered(t *testing.T) {
	for _, id := range []string{"obs", "discord", "obs-discord", "zoom", "slack", "vscode", "photoshop", "premiere"} {
		if _, ok := Get(id); !ok {
			t.Errorf("falta el paquete %s", id)
		}
	}
}

func TestPCVariantsAndOSFilter(t *testing.T) {
	zoom, _ := Get("zoom")
	mac, _ := ResolveFor("darwin", zoom, "full-iso", "es", nil)
	pc, _ := ResolveFor("windows", zoom, "full-iso", "es", nil)
	find := func(r Resolved, label string) string {
		for _, k := range r.Layer.Keys {
			if k.Label == label && k.Tap != nil {
				return k.Tap.Keys
			}
		}
		return ""
	}
	if find(mac, "Micro") != "cmd+shift+a" || find(pc, "Micro") != "alt+a" {
		t.Fatalf("Zoom: mac %q, pc %q", find(mac, "Micro"), find(pc, "Micro"))
	}
	disc, _ := Get("discord")
	pc, _ = ResolveFor("linux", disc, "full-iso", "es", nil)
	if got := find(pc, "Silenciar"); got != "ctrl+shift+m" {
		t.Fatalf("Discord en Linux: %q", got)
	}
	if got := find(pc, "Servidor arriba"); got != "ctrl+alt+up" {
		t.Fatalf("Discord servidor: %q", got)
	}
	for _, id := range []string{"xcode", "finalcut", "mac-productivity"} {
		p, _ := Get(id)
		if p.ForOS("linux") || p.ForOS("windows") || !p.ForOS("darwin") {
			t.Errorf("%s debe ser solo de macOS", id)
		}
	}
	if len(ForOS("linux")) >= len(All()) || len(ForOS("darwin")) != len(All()) {
		t.Fatalf("filtro por sistema: linux %d, darwin %d de %d", len(ForOS("linux")), len(ForOS("darwin")), len(All()))
	}
}
