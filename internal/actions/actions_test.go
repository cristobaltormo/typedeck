package actions

import (
	"runtime"
	"testing"
	"time"

	"github.com/cristobaltormo/typedeck/internal/config"
	"github.com/cristobaltormo/typedeck/internal/platform"
)

func TestSubstituteAndDescribe(t *testing.T) {
	if got := Substitute("sin llaves", false); got != "sin llaves" {
		t.Fatal(got)
	}
	if got := Substitute("{date}", false); len(got) != 10 {
		t.Fatalf("fecha: %q", got)
	}
	want := `'a'\''b'`
	if runtime.GOOS == "windows" {
		want = `"a'b"`
	}
	if got := shellQuote("a'b"); got != want {
		t.Fatal(got)
	}
}

type fakeEnv struct{ layer int }

func (f fakeEnv) Settings() config.Settings    { return config.DefaultSettings() }
func (fakeEnv) HUD(string, string, bool)       {}
func (fakeEnv) Goto(any)                       {}
func (fakeEnv) ToggleLayer(any)                {}
func (fakeEnv) ToggleCaffeinate() bool         { return false }
func (fakeEnv) StartTimer(float64, string)     {}
func (fakeEnv) Hardware() Hardware             { return nil }
func (f fakeEnv) Layer() int                   { return f.layer }
func (fakeEnv) SetKeyHistory(mode string) bool { return mode == "on" }
func (f fakeEnv) LayerName() string            { return []string{"Uno", "Dos"}[f.layer] }

func TestInTimeRange(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 10, 3, h, m, 0, 0, time.UTC) }
	for _, c := range []struct {
		spec string
		now  time.Time
		want bool
	}{
		{"09:00-18:00", at(12, 0), true},
		{"09:00-18:00", at(18, 0), false},
		{"22:00-06:00", at(23, 30), true},
		{"22:00-06:00", at(5, 59), true},
		{"22:00-06:00", at(12, 0), false},
		{"basura", at(12, 0), false},
	} {
		if got := inTimeRange(c.spec, c.now); got != c.want {
			t.Errorf("%s a las %s: %v", c.spec, c.now.Format("15:04"), got)
		}
	}
}

func TestConditionalRunsTheRightBranch(t *testing.T) {
	hud := func(s string) config.Action { return config.Action{Type: "hud", Text: s} }
	a := config.Action{Type: "if", Cond: "layer", Target: "2", Steps: []config.Action{hud("then")}, Else: []config.Action{{Type: "wait", Ms: 1}, {Type: "app"}}}
	if r := Execute(a, fakeEnv{layer: 1}, false); !r.OK {
		t.Fatalf("the branch for layer 2 should have run: %+v", r)
	}
	if r := Execute(a, fakeEnv{layer: 0}, false); r.OK {
		t.Fatal("the other branch should have run and failed on the empty app")
	}
	byName := config.Action{Type: "if", Cond: "layer", Target: "dos", Steps: []config.Action{hud("x")}}
	if ok, _ := condition(byName, fakeEnv{layer: 1}); !ok {
		t.Fatal("a layer must be selectable by name")
	}
	not := config.Action{Type: "if", Cond: "os", Target: platform.Current.Name(), Not: true}
	if ok, _ := condition(not, fakeEnv{}); ok {
		t.Fatal("not must invert the condition")
	}
}

func TestHistoryActionReportsTheNewState(t *testing.T) {
	if r := Execute(config.Action{Type: "history", Cmd: "on"}, fakeEnv{}, false); !r.OK || r.Output != "on" {
		t.Fatalf("on: %+v", r)
	}
	if r := Execute(config.Action{Type: "history", Cmd: "off"}, fakeEnv{}, false); !r.OK || r.Output != "off" {
		t.Fatalf("off: %+v", r)
	}
}
