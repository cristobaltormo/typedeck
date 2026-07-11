package layouts

import (
	"testing"
)

func TestEveryLayoutIsConsistent(t *testing.T) {
	wantKeys := map[string]int{"full-ansi": 104, "full-iso": 105, "tkl-ansi": 87, "tkl-iso": 88, "75-ansi": 82, "75-iso": 83, "60-ansi": 61, "60-iso": 62, "numpad": 17}
	for _, l := range All() {
		seen := map[byte]bool{}
		for _, k := range l.Keys {
			if seen[k.Usage] {
				t.Errorf("%s: uso 0x%02X repetido", l.ID, k.Usage)
			}
			seen[k.Usage] = true
			if k.X < -0.001 || k.Y < -0.001 || k.X+k.W > l.Width+0.001 || k.Y+k.H > l.Height+0.001 {
				t.Errorf("%s: la tecla 0x%02X se sale (%v,%v %vx%v de %vx%v)", l.ID, k.Usage, k.X, k.Y, k.W, k.H, l.Width, l.Height)
			}
		}
		for i, a := range l.Keys {
			for _, b := range l.Keys[i+1:] {
				if a.X < b.X+b.W-0.01 && b.X < a.X+a.W-0.01 && a.Y < b.Y+b.H-0.01 && b.Y < a.Y+a.H-0.01 {
					t.Errorf("%s: solape entre 0x%02X y 0x%02X", l.ID, a.Usage, b.Usage)
				}
			}
		}
		if want, ok := wantKeys[l.ID]; ok && len(l.Keys) != want {
			t.Errorf("%s: %d teclas, esperaba %d", l.ID, len(l.Keys), want)
		}
	}
}

func TestPercentAndStandard(t *testing.T) {
	for id, p := range map[string]int{"full-iso": 100, "tkl-ansi": 80, "75-iso": 75, "65-ansi": 65, "60-ansi": 60} {
		l, ok := Get(id)
		if !ok || l.Percent != p {
			t.Errorf("%s: %+v %v", id, l.Percent, ok)
		}
	}
	if l, _ := Get("iso-full"); l.ID != "full-iso" {
		t.Error("el nombre antiguo iso-full debe traducirse")
	}
	if l, _ := Get("full-iso"); l.Standard != "iso" {
		t.Error("estandar")
	}
}

func TestWidthsAreRealisticForEachFormFactor(t *testing.T) {
	want := map[string]float64{"full-ansi": 22.5, "tkl-ansi": 18.25, "75-ansi": 16, "65-ansi": 16, "60-ansi": 15}
	for id, w := range want {
		l, _ := Get(id)
		if l.Width < w-0.01 || l.Width > w+0.01 {
			t.Errorf("%s: ancho %v, esperaba %v", id, l.Width, w)
		}
	}
}

func TestMatchRecognisesEachLayout(t *testing.T) {
	for _, l := range All() {
		set := l.KeySet()
		all := Match(set)
		if all[0].ID != l.ID {
			t.Errorf("%s con todas sus teclas: gana %s (%.2f) y no %s", l.ID, all[0].ID, all[0].Score, l.ID)
		}
		part := map[byte]bool{}
		i := 0
		for u := range set {
			if i%7 != 0 {
				part[u] = true
			}
			i++
		}
		if r := Match(part); len(r) > 0 && r[0].ID != l.ID && r[0].Score-scoreOf(r, l.ID) > 0.05 {
			t.Errorf("%s con el 86%% de las teclas: gana %s (%.2f frente a %.2f)", l.ID, r[0].ID, r[0].Score, scoreOf(r, l.ID))
		}
	}
}

func scoreOf(r []Suggestion, id string) float64 {
	for _, s := range r {
		if s.ID == id {
			return s.Score
		}
	}
	return 0
}

func TestISOvsANSIIsDecidedByTheExtraKeys(t *testing.T) {
	iso, _ := Get("full-iso")
	if r := Match(iso.KeySet()); r[0].ID != "full-iso" {
		t.Fatalf("un teclado ISO completo debe ganar como ISO: %s", r[0].ID)
	}
	ansi, _ := Get("tkl-ansi")
	if r := Match(ansi.KeySet()); r[0].ID != "tkl-ansi" {
		t.Fatalf("un TKL ANSI debe ganar como ANSI: %s", r[0].ID)
	}
	if got := Match(map[byte]bool{}); len(got) != 0 {
		t.Error("sin teclas no hay sugerencias")
	}
}

func TestAmbiguityAsksForTheDecidingKeys(t *testing.T) {
	iso, _ := Get("full-iso")
	seen := iso.KeySet()
	delete(seen, 0x64)
	delete(seen, 0x32)
	r := Match(seen)
	amb, hints := Ambiguity(r)
	if !amb || len(hints) == 0 {
		t.Fatalf("debía ser ambiguo: %v %v (%s %.3f / %s %.3f)", amb, hints, r[0].ID, r[0].Score, r[1].ID, r[1].Score)
	}
	if amb, _ := Ambiguity(Match(iso.KeySet())); amb {
		t.Error("con todas las teclas no hay ambiguedad")
	}
}

func TestMissingKeysAreReported(t *testing.T) {
	l, _ := Get("tkl-ansi")
	seen := l.KeySet()
	delete(seen, 0x2B)
	r := Match(seen)
	var tkl Suggestion
	for _, s := range r {
		if s.ID == "tkl-ansi" {
			tkl = s
		}
	}
	if len(tkl.Missing) != 1 || tkl.Missing[0] != 0x2B {
		t.Errorf("faltantes: %v", tkl.Missing)
	}
}

func TestISOWithBackslashAliasStillWinsAsISO(t *testing.T) {
	iso, _ := Get("full-iso")
	seen := iso.KeySet()
	delete(seen, 0x32)
	seen[0x31] = true
	r := Match(seen)
	if r[0].ID != "full-iso" {
		t.Fatalf("ISO con alias 0x31: gana %s (%.3f)", r[0].ID, r[0].Score)
	}
	if len(r[0].Extra) != 0 {
		t.Errorf("0x31 no debe contar como extra en ISO: %v", r[0].Extra)
	}
	ansi, _ := Get("full-ansi")
	if r := Match(ansi.KeySet()); r[0].ID != "full-ansi" {
		t.Fatalf("ANSI: %s", r[0].ID)
	}
}
