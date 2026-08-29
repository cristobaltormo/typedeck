package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateV1(t *testing.T) {
	old := `{"settings":{"hud_on_action":true},"global":{"KPENTER":{"type":"url","url":"http://x","label":"Ed"}},
	"layers":[{"name":"A","keys":{"KP7":{"type":"open","app":"Safari"},"KP/":{"type":"media","cmd":"mute"}}}]}`
	c, err := Parse([]byte(old))
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != 3 || !c.Settings.HUD.OnAction {
		t.Fatalf("version/ajustes: %+v", c.Settings)
	}
	k := c.Layers[0].Keys["5F"]
	if k.Tap == nil || k.Tap.Type != "app" || k.Tap.App != "Safari" {
		t.Fatalf("KP7 -> %+v", k)
	}
	if c.Global["58"].Label != "Ed" {
		t.Fatalf("KPENTER -> %+v", c.Global)
	}
	if c.Global["53"].Tap == nil || c.Global["53"].Tap.Type != "layer" {
		t.Fatal("NumLock debe cambiar de capa por defecto")
	}
	if c.Layers[0].Keys["54"].Tap.Cmd != "mute" {
		t.Fatalf("KP/ -> %+v", c.Layers[0].Keys["54"])
	}
}

func TestMigrateV2KeepsEverything(t *testing.T) {
	v2 := `{"version":2,"settings":{"language":"en","theme":"dark","accent":"#8b5cf6","keysize":100,"hud":{"enabled":false,"position":"top","seconds":3}},
	"global":{"KP.":{"tap":{"type":"system","cmd":"lock","confirm":true},"label":"L"}},
	"layers":[{"name":"Trabajo","color":"#2563eb","auto_apps":["Code"],"keys":{"KP8":{"tap":{"type":"app","app":"Termius","mode":"toggle"},"hold":{"type":"ssh","host":"my-server","cmd":"uptime"},"color":"#16a34a"}}}]}`
	c, err := Parse([]byte(v2))
	if err != nil {
		t.Fatal(err)
	}
	if c.Settings.Language != "en" || c.Settings.Accent != "#8b5cf6" || c.Settings.HUD.Enabled || c.Settings.HUD.Position != "top" {
		t.Fatalf("ajustes: %+v", c.Settings)
	}
	k := c.Layers[0].Keys["60"]
	if k.Tap.Mode != "toggle" || k.Hold.Host != "my-server" || k.Hold.ShowOutput == nil || !*k.Hold.ShowOutput {
		t.Fatalf("KP8 -> %+v %+v", k.Tap, k.Hold)
	}
	if !c.Global["63"].Tap.Confirm {
		t.Fatal("se pierde confirm")
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]func(*Config){
		"sin capas":       func(c *Config) { c.Layers = nil },
		"muchas capas":    func(c *Config) { c.Layers = make([]Layer, 10) },
		"tecla inventada": func(c *Config) { c.Layers[0].Keys["ZZ"] = KeyDef{Label: "x"} },
		"tecla fuera":     func(c *Config) { c.Layers[0].Keys["01"] = KeyDef{Label: "x"} },
		"tipo malo":       func(c *Config) { c.Layers[0].Keys["5F"] = KeyDef{Tap: &Action{Type: "nope"}} },
		"color malo":      func(c *Config) { c.Layers[0].Keys["5F"] = KeyDef{Color: "red"} },
		"capa sin nombre": func(c *Config) { c.Layers[0].Name = "  " },
		"texto enorme": func(c *Config) {
			c.Layers[0].Keys["5F"] = KeyDef{Tap: &Action{Type: "text", Text: strings.Repeat("a", 5000)}}
		},
		"secuencia anidada": func(c *Config) {
			c.Layers[0].Keys["5F"] = KeyDef{Tap: &Action{Type: "sequence", Steps: []Action{{Type: "sequence"}}}}
		},
	}
	for name, mutate := range cases {
		c := Default()
		mutate(&c)
		if _, err := Validate(c); err == nil {
			t.Errorf("%s: debía fallar", name)
		}
	}
}

func TestValidateClampsAndNormalises(t *testing.T) {
	c := Default()
	c.Settings.HoldMS, c.Settings.KeySize, c.Settings.Accent, c.Settings.Input = 99999, 1, "azul", "raro"
	c.Layers[0].Keys["5f"] = KeyDef{Tap: &Action{Type: "timer", Minutes: 99999}}
	out, err := Validate(c)
	if err != nil {
		t.Fatal(err)
	}
	s := out.Settings
	if s.HoldMS != 1500 || s.KeySize != 64 || s.Accent != "#2563eb" || s.Input != "auto" {
		t.Fatalf("ajustes sin acotar: %+v", s)
	}
	if out.Layers[0].Keys["5F"].Tap.Minutes != 600 {
		t.Fatal("la clave 5f debía pasar a 5F y acotar minutos")
	}
}

func TestSaveLoadBackupRestore(t *testing.T) {
	p := Paths{Dir: t.TempDir()}
	c, err := Load(p)
	if err != nil || len(c.Layers) != 2 {
		t.Fatalf("load: %v %+v", err, c)
	}
	c.Layers[0].Name = "Otra"
	if err := Save(p, c, true, true); err != nil {
		t.Fatal(err)
	}
	got, _ := Load(p)
	if got.Layers[0].Name != "Otra" {
		t.Fatal("no se guardó")
	}
	b := ListBackups(p)
	if len(b) != 1 || b[0].Layers != 2 {
		t.Fatalf("copias: %+v", b)
	}
	r, err := RestoreBackup(p, b[0].Name)
	if err != nil || r.Layers[0].Name != "Capa 1" {
		t.Fatalf("restaurar: %v %+v", err, r.Layers[0])
	}
	if _, err := RestoreBackup(p, "../../etc/passwd"); err == nil {
		t.Fatal("ruta maliciosa aceptada")
	}
	if _, err := os.Stat(p.Config() + ".tmp"); err == nil {
		t.Fatal("queda un temporal")
	}
}

func TestLayerTargetsNormalised(t *testing.T) {
	c := Default()
	c.Layers[0].Keys["5F"] = KeyDef{Tap: &Action{Type: "layer", To: float64(1)}}
	c.Layers[0].Keys["60"] = KeyDef{Tap: &Action{Type: "layer", To: "prev"}}
	c.Layers[0].Keys["61"] = KeyDef{Tap: &Action{Type: "layer", To: "basura"}}
	out, err := Validate(c)
	if err != nil {
		t.Fatal(err)
	}
	if out.Layers[0].Keys["5F"].Tap.To != 1 || out.Layers[0].Keys["60"].Tap.To != "prev" || out.Layers[0].Keys["61"].Tap.To != "next" {
		t.Fatalf("destinos: %+v %+v %+v", out.Layers[0].Keys["5F"].Tap.To, out.Layers[0].Keys["60"].Tap.To, out.Layers[0].Keys["61"].Tap.To)
	}
}

func TestCorruptConfigRecoversFromBackup(t *testing.T) {
	p := Paths{Dir: t.TempDir()}
	c := Default()
	c.Layers[0].Name = "Buena"
	if err := Save(p, c, false, false); err != nil {
		t.Fatal(err)
	}
	c.Layers[0].Name = "Segunda"
	if err := Save(p, c, true, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Config(), []byte("{esto no es json"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, note, err := LoadWithNote(p)
	if err != nil || got.Layers[0].Name != "Buena" || note == "" {
		t.Fatalf("no recuperó: %v %q %+v", err, note, got.Layers[0])
	}
	if m, _ := filepath.Glob(filepath.Join(p.Dir, "config.broken-*.json")); len(m) != 1 {
		t.Fatal("el original dañado debe conservarse")
	}
	if again, note, err := LoadWithNote(p); err != nil || note != "" || again.Layers[0].Name != "Buena" {
		t.Fatalf("segunda carga: %v %q", err, note)
	}
}

func TestCorruptConfigWithoutBackupsStartsFresh(t *testing.T) {
	p := Paths{Dir: t.TempDir()}
	_ = os.MkdirAll(p.Dir, 0o755)
	_ = os.WriteFile(p.Config(), []byte(`{"version":3,"layers":[]}`), 0o644)
	got, note, err := LoadWithNote(p)
	if err != nil || note == "" || len(got.Layers) != 2 {
		t.Fatalf("%v %q %d", err, note, len(got.Layers))
	}
}

func TestOBSActionAndSettings(t *testing.T) {
	c := Default()
	c.Settings.OBS = OBS{Host: " bad host/ ", Port: 99999, Password: strings.Repeat("x", 500)}
	c.Layers[0].Keys["04"] = KeyDef{Tap: &Action{Type: "obs", Cmd: "borrar-todo", Target: "Mic"}}
	c.Layers[0].Keys["05"] = KeyDef{Tap: &Action{Type: "obs", Cmd: "scene", Target: "#2"}}
	out, err := Validate(c)
	if err != nil {
		t.Fatal(err)
	}
	if out.Settings.OBS.Host != "127.0.0.1" || out.Settings.OBS.Port != 65535 || len(out.Settings.OBS.Password) != 200 {
		t.Fatalf("ajustes de OBS sin acotar: %+v", out.Settings.OBS)
	}
	if a := out.Layers[0].Keys["04"].Tap; a.Cmd != "record" {
		t.Fatalf("una orden desconocida debe volver a la de por defecto: %+v", a)
	}
	if a := out.Layers[0].Keys["05"].Tap; a.Cmd != "scene" || a.Target != "#2" {
		t.Fatalf("acción válida alterada: %+v", a)
	}
}
