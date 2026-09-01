package hid

import "testing"

const rdesc0 = "05 01 09 06 A1 01 05 07 19 E0 29 E7 15 00 25 01 95 08 75 01 81 02 95 01 75 08 81 03 95 06 75 08 15 00 26 FF 00 05 07 19 00 2A FF 00 81 00 25 01 95 05 75 01 05 08 19 01 29 05 91 02 95 01 75 03 91 03 C0"

const rdesc1 = "06 01 00 09 80 A1 01 85 01 19 81 29 83 15 00 25 01 95 03 75 01 81 02 95 01 75 05 81 01 C0 05 0C 09 01 A1 01 85 02 19 00 2A FF 02 15 00 26 FF 7F 95 01 75 10 81 00 C0 06 00 FF 09 01 A1 01 85 03 15 00 26 FF 00 09 2F 75 08 95 03 81 02 C0 05 01 09 06 A1 01 85 04 05 07 19 04 29 70 15 00 25 01 75 01 95 78 81 02 C0 06 00 FF 09 01 A1 01 85 05 15 00 26 FF 00 19 01 29 02 75 08 95 05 B1 02 C0 06 00 FF 09 01 A1 01 85 06 15 00 26 FF 00 19 01 29 02 75 08 96 07 04 B1 02 C0 05 01 09 02 A1 01 85 07 09 01 A1 00 05 09 15 00 25 01 19 01 29 05 75 01 95 05 81 02 95 03 81 01 05 01 16 00 80 26 FF 7F 09 30 09 31 75 10 95 02 81 06 15 81 25 7F 09 38 75 08 95 01 81 06 05 0C 0A 38 02 95 01 81 06 C0 C0"

func kinds(rs []Report) map[int]string {
	m := map[int]string{}
	for _, r := range rs {
		if r.Dir == "input" {
			m[r.ID] = r.Kind
		}
	}
	return m
}

func TestReportDescriptorRealKeyboard(t *testing.T) {
	r0, err := ParseReportDescriptorHex(rdesc0)
	if err != nil {
		t.Fatal(err)
	}
	if k := kinds(r0)[0]; k != "keyboard" {
		t.Fatalf("interfaz 0: esperaba keyboard, salio %q (%+v)", k, r0)
	}
	r1, _ := ParseReportDescriptorHex(rdesc1)
	want := map[int]string{1: "system", 2: "consumer", 3: "vendor", 4: "nkro", 7: "mouse"}
	got := kinds(r1)
	for id, k := range want {
		if got[id] != k {
			t.Errorf("informe %d: esperaba %s, salio %q", id, k, got[id])
		}
	}
}

func TestHotkeyAndLayouts(t *testing.T) {
	es := LayoutByName("es-iso")
	st, err := ParseHotkey("cmd+shift+4", es)
	if err != nil || st.Usage != 0x21 || st.Mods != ModGui|ModShift {
		t.Fatalf("cmd+shift+4 -> %+v %v", st, err)
	}
	st, err = ParseHotkey("ctrl+alt+f13", es)
	if err != nil || st.Usage != 0x68 || st.Mods != ModCtrl|ModAlt {
		t.Fatalf("f13 -> %+v %v", st, err)
	}
	if _, err = ParseHotkey("cmd+nada", es); err == nil {
		t.Fatal("tecla inexistente debia fallar")
	}
	if _, err = ParseHotkey("hyper+a", es); err == nil {
		t.Fatal("modificador inexistente debia fallar")
	}
	if s, _ := es.Strokes('ñ'); len(s) != 1 || s[0].Usage != 0x33 {
		t.Fatalf("ñ -> %+v", s)
	}
	if s, _ := es.Strokes('á'); len(s) != 2 || s[0].Usage != 0x34 || s[1].Usage != 0x04 {
		t.Fatalf("á -> %+v", s)
	}
	if s, _ := es.Strokes('~'); len(s) != 2 || s[1].Usage != 0x2C {
		t.Fatalf("~ es una tecla muerta + espacio: %+v", s)
	}
	if !es.Typeable("Hola, ¿qué tal? @#{}") {
		t.Fatal("el texto debia poder teclearse")
	}
	if LayoutByName("us").Typeable("ñ") {
		t.Fatal("la ñ no existe en US")
	}
	if LayoutFromInputSource("com.apple.keylayout.Spanish-ISO") != "es-iso" || LayoutFromInputSource("com.apple.keylayout.US") != "us" {
		t.Fatal("deteccion de disposicion")
	}
	for id, want := range map[string]string{"com.apple.keylayout.British": "qwerty", "com.apple.keylayout.French": "", "com.apple.keylayout.German": "",
		"com.apple.keylayout.Dvorak": "", "com.apple.keylayout.Colemak": "", "com.apple.keylayout.Spanish": "qwerty", "": ""} {
		if got := LayoutFromInputSource(id); got != want {
			t.Errorf("%q -> %q, esperaba %q", id, got, want)
		}
	}
	if LayoutByName("").SafeForKeys() || !LayoutByName("qwerty").SafeForKeys() || LayoutByName("qwerty").Typeable("a,b") {
		t.Fatal("none no es seguro; qwerty solo teclea letras y cifras")
	}
	if !LayoutByName("qwerty").Typeable("Hola 123\n") {
		t.Fatal("qwerty debe poder teclear letras, cifras, espacio y salto")
	}
}

func TestSpanishPCLayout(t *testing.T) {
	l := LayoutByName("es-pc")
	if l.Name != "es-pc" || LayoutByName("es-iso").Name != "es-iso" {
		t.Fatal("las dos disposiciones españolas deben ser distintas")
	}
	cases := map[rune][]Stroke{
		'@': {{0x1F, ModAltGr}}, '#': {{0x20, ModAltGr}}, '~': {{0x21, ModAltGr}}, '€': {{0x22, ModAltGr}}, '\\': {{0x35, ModAltGr}},
		'|': {{0x1E, ModAltGr}}, '[': {{0x2F, ModAltGr}}, ']': {{0x30, ModAltGr}}, '{': {{0x34, ModAltGr}}, '}': {{0x32, ModAltGr}},
		'ñ': {{0x33, 0}}, 'Ñ': {{0x33, ModShift}}, 'ç': {{0x32, 0}}, '<': {{0x64, 0}}, '>': {{0x64, ModShift}},
		'á': {{0x34, 0}, {0x04, 0}}, '^': {{0x2F, ModShift}, {0x2C, 0}}, '`': {{0x2F, 0}, {0x2C, 0}}, '/': {{0x24, ModShift}}, '=': {{0x27, ModShift}},
	}
	for r, want := range cases {
		got, ok := l.Strokes(r)
		if !ok || len(got) != len(want) {
			t.Errorf("%q: %v %v", r, got, ok)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%q: %v, esperaba %v", r, got, want)
			}
		}
	}
	if _, ok := l.Strokes('∞'); ok {
		t.Error("∞ no existe en Español (PC)")
	}
	if _, ok := LayoutByName("es-iso").Strokes('∞'); !ok {
		t.Error("∞ si existe en el ISO de macOS")
	}
}
