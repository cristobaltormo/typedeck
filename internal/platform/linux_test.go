//go:build linux

package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func writeDesktop(t *testing.T, dir, id, body string) string {
	t.Helper()
	p := filepath.Join(dir, "applications", id+".desktop")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDesktopEntries(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_DATA_DIRS", dir)
	writeDesktop(t, dir, "code", "[Desktop Entry]\nType=Application\nName=Visual Studio Code\nName[es]=Código\nExec=/usr/share/code/code --unity-launch %F\nStartupWMClass=Code\n")
	writeDesktop(t, dir, "hidden", "[Desktop Entry]\nType=Application\nName=Oculta\nExec=oculta\nNoDisplay=true\n")
	writeDesktop(t, dir, "org.mozilla.firefox", "[Desktop Entry]\nType=Application\nName=Firefox\nExec=env MOZ_X=1 firefox %u\n[Desktop Action new-window]\nName=Nueva ventana\nExec=firefox --new-window\n")
	writeDesktop(t, dir, "link", "[Desktop Entry]\nType=Link\nName=Enlace\nURL=x\n")
	entriesMu.Lock()
	entries = nil
	entriesMu.Unlock()

	names := (&linux{}).InstalledApps()
	if len(names) != 2 || names[0] != "Firefox" || names[1] != "Visual Studio Code" {
		t.Fatalf("aplicaciones: %v", names)
	}
	if e, ok := findEntry("code"); !ok || e.Name != "Visual Studio Code" {
		t.Fatalf("by id: %+v %v", e, ok)
	}
	if e, ok := findEntry("visual studio"); !ok || e.ID != "code" {
		t.Fatalf("parcial: %+v %v", e, ok)
	}
	if e, ok := entryFor("Code", ""); !ok || e.ID != "code" {
		t.Fatalf("by window class: %+v %v", e, ok)
	}
	if e, ok := entryFor("", "firefox"); !ok || e.Name != "Firefox" {
		t.Fatalf("by process: %+v %v", e, ok)
	}
	if _, ok := findEntry("oculta"); ok {
		t.Fatal("an app with NoDisplay must not be offered")
	}
	if got := execBase("env MOZ_X=1 firefox %u"); got != "firefox" {
		t.Fatalf("execBase: %q", got)
	}
	if got := expandExec(`/usr/share/code/code --unity-launch %F`); len(got) != 2 || got[1] != "--unity-launch" {
		t.Fatalf("expandExec: %v", got)
	}
}

func TestXKBToHID(t *testing.T) {
	cases := []struct{ l, v, want string }{{"us", "", "us"}, {"es", "", "es-pc"}, {"es", "winkeys", "es-pc"}, {"us", "intl", "qwerty"}, {"gb", "", "qwerty"},
		{"es", "cat", "qwerty"}, {"fr", "", ""}, {"de", "", ""}, {"us", "dvorak", "qwerty"}}
	for _, c := range cases {
		want := c.want
		if c.v == "dvorak" {
			want = ""
		}
		if got := xkbToHID(c.l, c.v); got != want {
			t.Errorf("%s/%s -> %q, expected %q", c.l, c.v, got, want)
		}
	}
}

func TestXKeyNames(t *testing.T) {
	cases := map[byte]string{0x04: "a", 0x27: "0", 0x3A: "F1", 0x45: "F12", 0x28: "Return", 0x4F: "Right", 0x2C: "space", 0x68: "F13"}
	for u, want := range cases {
		if got := xkey(u); got != want {
			t.Errorf("%02X -> %q, expected %q", u, got, want)
		}
	}
}

func TestFocusedNode(t *testing.T) {
	tree := map[string]any{"nodes": []any{map[string]any{"nodes": []any{map[string]any{"id": 7.0, "focused": true, "app_id": "firefox"}}}}}
	if n := focusedNode(tree); n == nil || n["app_id"] != "firefox" {
		t.Fatalf("%v", n)
	}
	if focusedNode(map[string]any{"nodes": []any{}}) != nil {
		t.Fatal("without focus it must return nil")
	}
}
