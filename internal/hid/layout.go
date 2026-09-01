package hid

import "strings"

type Layout struct {
	Name  string
	chars map[rune][]Stroke
}

func (l *Layout) Strokes(r rune) ([]Stroke, bool) {
	s, ok := l.chars[r]
	return s, ok
}

func (l *Layout) Typeable(text string) bool {
	for _, r := range text {
		if _, ok := l.chars[r]; !ok {
			return false
		}
	}
	return true
}

func (l *Layout) add(r rune, usage, mods byte) { l.chars[r] = []Stroke{{usage, mods}} }

func base(name string) *Layout {
	l := &Layout{Name: name, chars: map[rune][]Stroke{}}
	for i := 0; i < 26; i++ {
		l.add(rune('a'+i), byte(0x04+i), 0)
		l.add(rune('A'+i), byte(0x04+i), ModShift)
	}
	for i := 1; i <= 9; i++ {
		l.add(rune('0'+i), byte(0x1D+i), 0)
	}
	l.add('0', 0x27, 0)
	l.add(' ', 0x2C, 0)
	l.add('\n', 0x28, 0)
	l.add('\t', 0x2B, 0)
	return l
}

var layoutUS = func() *Layout {
	l := base("us")
	for i, c := range "!@#$%^&*()" {
		u := byte(0x1E + i)
		l.add(c, u, ModShift)
	}
	pairs := []struct {
		u    byte
		a, b rune
	}{{0x2D, '-', '_'}, {0x2E, '=', '+'}, {0x2F, '[', '{'}, {0x30, ']', '}'}, {0x31, '\\', '|'},
		{0x33, ';', ':'}, {0x34, '\'', '"'}, {0x35, '`', '~'}, {0x36, ',', '<'}, {0x37, '.', '>'}, {0x38, '/', '?'}}
	for _, p := range pairs {
		l.add(p.a, p.u, 0)
		l.add(p.b, p.u, ModShift)
	}
	return l
}()

var layoutESISO = func() *Layout {
	l := base("es-iso")
	nums := []struct {
		u          byte
		shift, alt rune
	}{{0x1E, '!', '|'}, {0x1F, '"', '@'}, {0x20, '·', '#'}, {0x21, '$', '¢'}, {0x22, '%', '∞'},
		{0x23, '&', '¬'}, {0x24, '/', '÷'}, {0x25, '(', '“'}, {0x26, ')', '”'}, {0x27, '=', '≠'}}
	for _, n := range nums {
		l.add(n.shift, n.u, ModShift)
		l.add(n.alt, n.u, ModAlt)
	}
	l.add('º', 0x35, 0)
	l.add('ª', 0x35, ModShift)
	l.add('\\', 0x35, ModAlt)
	l.add('\'', 0x2D, 0)
	l.add('?', 0x2D, ModShift)
	l.add('¡', 0x2E, 0)
	l.add('¿', 0x2E, ModShift)
	l.add('[', 0x2F, ModAlt)
	l.add('+', 0x30, 0)
	l.add('*', 0x30, ModShift)
	l.add(']', 0x30, ModAlt)
	l.add('ñ', 0x33, 0)
	l.add('Ñ', 0x33, ModShift)
	l.chars['~'] = []Stroke{{0x33, ModAlt}, {0x2C, 0}}
	l.chars['`'] = []Stroke{{0x2F, 0}, {0x2C, 0}}
	l.chars['^'] = []Stroke{{0x2F, ModShift}, {0x2C, 0}}
	l.chars['´'] = []Stroke{{0x34, 0}, {0x2C, 0}}
	l.chars['¨'] = []Stroke{{0x34, ModShift}, {0x2C, 0}}
	l.add('{', 0x34, ModAlt)
	l.add('ç', 0x32, 0)
	l.add('Ç', 0x32, ModShift)
	l.add('}', 0x32, ModAlt)
	l.add('<', 0x64, 0)
	l.add('>', 0x64, ModShift)
	l.add(',', 0x36, 0)
	l.add(';', 0x36, ModShift)
	l.add('.', 0x37, 0)
	l.add(':', 0x37, ModShift)
	l.add('-', 0x38, 0)
	l.add('_', 0x38, ModShift)
	for _, v := range []struct {
		plain, acute, diaer rune
		key                 byte
	}{{'a', 'á', 'ä', 0x04}, {'e', 'é', 'ë', 0x08}, {'i', 'í', 'ï', 0x0C}, {'o', 'ó', 'ö', 0x12}, {'u', 'ú', 'ü', 0x18}} {
		l.chars[v.acute] = []Stroke{{0x34, 0}, {v.key, 0}}
		l.chars[v.acute-0x20] = []Stroke{{0x34, 0}, {v.key, ModShift}}
		l.chars[v.diaer] = []Stroke{{0x34, ModShift}, {v.key, 0}}
		l.chars[v.diaer-0x20] = []Stroke{{0x34, ModShift}, {v.key, ModShift}}
	}
	return l
}()

var layoutESPC = func() *Layout {
	l := base("es-pc")
	altgr := ModAltGr
	nums := []struct {
		u          byte
		shift, alt rune
	}{{0x1E, '!', '|'}, {0x1F, '"', '@'}, {0x20, '·', '#'}, {0x21, '$', '~'}, {0x22, '%', '€'},
		{0x23, '&', '¬'}, {0x24, '/', 0}, {0x25, '(', 0}, {0x26, ')', 0}, {0x27, '=', 0}}
	for _, n := range nums {
		l.add(n.shift, n.u, ModShift)
		if n.alt != 0 {
			l.add(n.alt, n.u, altgr)
		}
	}
	l.add('º', 0x35, 0)
	l.add('ª', 0x35, ModShift)
	l.add('\\', 0x35, altgr)
	l.add('\'', 0x2D, 0)
	l.add('?', 0x2D, ModShift)
	l.add('¡', 0x2E, 0)
	l.add('¿', 0x2E, ModShift)
	l.add('[', 0x2F, altgr)
	l.add('+', 0x30, 0)
	l.add('*', 0x30, ModShift)
	l.add(']', 0x30, altgr)
	l.add('ñ', 0x33, 0)
	l.add('Ñ', 0x33, ModShift)
	l.chars['`'] = []Stroke{{0x2F, 0}, {0x2C, 0}}
	l.chars['^'] = []Stroke{{0x2F, ModShift}, {0x2C, 0}}
	l.chars['´'] = []Stroke{{0x34, 0}, {0x2C, 0}}
	l.chars['¨'] = []Stroke{{0x34, ModShift}, {0x2C, 0}}
	l.add('{', 0x34, altgr)
	l.add('ç', 0x32, 0)
	l.add('Ç', 0x32, ModShift)
	l.add('}', 0x32, altgr)
	l.add('<', 0x64, 0)
	l.add('>', 0x64, ModShift)
	l.add(',', 0x36, 0)
	l.add(';', 0x36, ModShift)
	l.add('.', 0x37, 0)
	l.add(':', 0x37, ModShift)
	l.add('-', 0x38, 0)
	l.add('_', 0x38, ModShift)
	for _, v := range []struct {
		plain, acute, diaer rune
		key                 byte
	}{{'a', 'á', 'ä', 0x04}, {'e', 'é', 'ë', 0x08}, {'i', 'í', 'ï', 0x0C}, {'o', 'ó', 'ö', 0x12}, {'u', 'ú', 'ü', 0x18}} {
		l.chars[v.acute] = []Stroke{{0x34, 0}, {v.key, 0}}
		l.chars[v.acute-0x20] = []Stroke{{0x34, 0}, {v.key, ModShift}}
		l.chars[v.diaer] = []Stroke{{0x34, ModShift}, {v.key, 0}}
		l.chars[v.diaer-0x20] = []Stroke{{0x34, ModShift}, {v.key, ModShift}}
	}
	return l
}()

// layoutESWin is the Windows Spanish layout: AltGr+4 is a dead key there (checked on a real Windows 11), so ~ needs a space after it.
var layoutQWERTY = base("qwerty")

var layoutNone = &Layout{Name: "none", chars: map[rune][]Stroke{}}

func LayoutByName(name string) *Layout {
	switch {
	case name == "es-pc":
		return layoutESPC
	case strings.HasPrefix(name, "es"):
		return layoutESISO
	case name == "us":
		return layoutUS
	case name == "qwerty":
		return layoutQWERTY
	}
	return layoutNone
}

func (l *Layout) SafeForKeys() bool { return l.Name != "none" }

var qwertyFamily = []string{"British", "Australian", "Canadian", "Irish", "LatinAmerican", "Portuguese", "Brazilian", "Italian",
	"Dutch", "Swedish", "Norwegian", "Danish", "Finnish", "USExtended", "Spanish", "Hawaiian", "Icelandic", "Polish", "Turkish-QWERTY", "Czech"}

func LayoutFromInputSource(id string) string {
	switch {
	case strings.Contains(id, "Spanish-ISO"):
		return "es-iso"
	case strings.HasSuffix(id, ".US"), strings.HasSuffix(id, ".ABC"):
		return "us"
	}
	name := strings.TrimPrefix(id, "com.apple.keylayout.")
	for _, f := range qwertyFamily {
		if name == f || strings.HasPrefix(name, f+"-") {
			return "qwerty"
		}
	}
	return ""
}
