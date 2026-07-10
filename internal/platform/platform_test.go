package platform

import (
	"reflect"
	"testing"
)

func TestSplitHotkey(t *testing.T) {
	mods, key, err := splitHotkey("Cmd+Shift+4")
	if err != nil || !reflect.DeepEqual(mods, []string{"gui", "shift"}) || key != "4" {
		t.Fatalf("%v %q %v", mods, key, err)
	}
	if mods, key, _ = splitHotkey("ctrl+alt+plus"); !reflect.DeepEqual(mods, []string{"ctrl", "alt"}) || key != "+" {
		t.Fatalf("%v %q", mods, key)
	}
	for _, bad := range []string{"", "cmd+", "hyper+x"} {
		if _, _, err := splitHotkey(bad); err == nil {
			t.Errorf("%q debe fallar", bad)
		}
	}
}

func TestRunTimeoutAndOutput(t *testing.T) {
	if CurrentName := Current.Name(); CurrentName == "windows" {
		t.Skip("usa utilidades de Unix")
	}
	out, err := Run(2e9, "", "echo", "hola")
	if err != nil || out != "hola" {
		t.Fatalf("%q %v", out, err)
	}
	if _, err := Run(100e6, "", "sleep", "2"); err == nil {
		t.Fatal("debe agotar el tiempo")
	}
}
