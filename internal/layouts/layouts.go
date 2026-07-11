package layouts

import (
	"sort"
)

type Key struct {
	Usage byte    `json:"u"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	W     float64 `json:"w"`
	H     float64 `json:"h"`
	Mod   bool    `json:"mod,omitempty"`
}

type Layout struct {
	ID       string  `json:"id"`
	Family   string  `json:"family"`
	Percent  int     `json:"percent"`
	Standard string  `json:"standard"`
	Width    float64 `json:"width"`
	Height   float64 `json:"height"`
	Keys     []Key   `json:"keys"`
}

func IsModifier(u byte) bool { return u >= 0xE0 && u <= 0xE7 }

type item struct {
	u    byte
	w, h float64
}

func k(u byte, w float64) item    { return item{u, w, 1} }
func tall(u byte, w float64) item { return item{u, w, 2} }
func gap(w float64) item          { return item{0, w, 1} }
func keys(us ...byte) []item {
	out := make([]item, len(us))
	for i, u := range us {
		out[i] = k(u, 1)
	}
	return out
}
func cat(parts ...[]item) []item {
	var out []item
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}
func one(i item) []item { return []item{i} }

type row struct {
	y     float64
	items []item
}

func build(id, family string, percent int, standard string, rows []row) Layout {
	l := Layout{ID: id, Family: family, Percent: percent, Standard: standard}
	for _, r := range rows {
		x := 0.0
		for _, it := range r.items {
			if it.u != 0 {
				l.Keys = append(l.Keys, Key{Usage: it.u, X: x, Y: r.y, W: it.w, H: it.h, Mod: IsModifier(it.u)})
			}
			x += it.w
			if x > l.Width {
				l.Width = x
			}
			if it.u != 0 && r.y+it.h > l.Height {
				l.Height = r.y + it.h
			}
		}
	}
	return l
}

var (
	fRow1    = keys(0x3A, 0x3B, 0x3C, 0x3D)
	fRow2    = keys(0x3E, 0x3F, 0x40, 0x41)
	fRow3    = keys(0x42, 0x43, 0x44, 0x45)
	numbers  = keys(0x35, 0x1E, 0x1F, 0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x2D, 0x2E)
	qLetters = keys(0x14, 0x1A, 0x08, 0x15, 0x17, 0x1C, 0x18, 0x0C, 0x12, 0x13, 0x2F, 0x30)
	aLetters = keys(0x04, 0x16, 0x07, 0x09, 0x0A, 0x0B, 0x0D, 0x0E, 0x0F, 0x33, 0x34)
	zLetters = keys(0x1D, 0x1B, 0x06, 0x19, 0x05, 0x11, 0x10, 0x36, 0x37, 0x38)
	space    = []item{k(0xE0, 1.25), k(0xE3, 1.25), k(0xE2, 1.25), k(0x2C, 6.25), k(0xE6, 1.25), k(0xE7, 1.25), k(0x65, 1.25), k(0xE4, 1.25)}
)

func letterRows(iso bool, tabW float64, capsW float64, rshiftW float64, backslashW float64) (tab, caps, shift []item) {
	if iso {
		tab = cat(one(k(0x2B, 1.5)), qLetters, one(gap(0.25)), one(tall(0x28, 1.25)))
		caps = cat(one(k(0x39, 1.75)), aLetters, one(k(0x32, 1)), one(gap(1.25)))
		shift = cat(one(k(0xE1, 1.25)), one(k(0x64, 1)), zLetters, one(k(0xE5, rshiftW)))
		return
	}
	tab = cat(one(k(0x2B, tabW)), qLetters, one(k(0x31, backslashW)))
	caps = cat(one(k(0x39, capsW)), aLetters, one(k(0x28, 2.25)))
	shift = cat(one(k(0xE1, 2.25)), zLetters, one(k(0xE5, rshiftW)))
	return
}

func std(iso bool) string {
	if iso {
		return "iso"
	}
	return "ansi"
}

func full(iso bool) Layout {
	tab, caps, shift := letterRows(iso, 1.5, 1.75, 2.75, 1.5)
	nav := func(a, b, c byte) []item { return cat(one(gap(0.25)), keys(a, b, c)) }
	np := func(items ...item) []item { return cat(one(gap(0.25)), items) }
	rows := []row{
		{0, cat(one(k(0x29, 1)), one(gap(1)), fRow1, one(gap(0.5)), fRow2, one(gap(0.5)), fRow3, nav(0x46, 0x47, 0x48))},
		{1.5, cat(numbers, one(k(0x2A, 2)), nav(0x49, 0x4A, 0x4B), np(k(0x53, 1), k(0x54, 1), k(0x55, 1), k(0x56, 1)))},
		{2.5, cat(tab, nav(0x4C, 0x4D, 0x4E), np(k(0x5F, 1), k(0x60, 1), k(0x61, 1), tall(0x57, 1)))},
		{3.5, cat(caps, one(gap(3.25)), np(k(0x5C, 1), k(0x5D, 1), k(0x5E, 1)))},
		{4.5, cat(shift, cat(one(gap(0.25)), one(gap(1)), one(k(0x52, 1)), one(gap(1))), np(k(0x59, 1), k(0x5A, 1), k(0x5B, 1), tall(0x58, 1)))},
		{5.5, cat(space, nav(0x50, 0x51, 0x4F), np(k(0x62, 2), k(0x63, 1)))},
	}
	id := "full-" + std(iso)
	return build(id, "full", 100, std(iso), rows)
}

func tkl(iso bool) Layout {
	tab, caps, shift := letterRows(iso, 1.5, 1.75, 2.75, 1.5)
	nav := func(a, b, c byte) []item { return cat(one(gap(0.25)), keys(a, b, c)) }
	rows := []row{
		{0, cat(one(k(0x29, 1)), one(gap(1)), fRow1, one(gap(0.5)), fRow2, one(gap(0.5)), fRow3, nav(0x46, 0x47, 0x48))},
		{1.5, cat(numbers, one(k(0x2A, 2)), nav(0x49, 0x4A, 0x4B))},
		{2.5, cat(tab, nav(0x4C, 0x4D, 0x4E))},
		{3.5, caps},
		{4.5, cat(shift, one(gap(0.25)), one(gap(1)), one(k(0x52, 1)))},
		{5.5, cat(space, nav(0x50, 0x51, 0x4F))},
	}
	return build("tkl-"+std(iso), "tkl", 80, std(iso), rows)
}

func compact(iso bool, seventyFive bool) Layout {
	tab, caps, shift := letterRows(iso, 1.5, 1.75, 1.75, 1.5)
	right := func(u byte) []item { return one(k(u, 1)) }
	bottom := cat(one(k(0xE0, 1.25)), one(k(0xE3, 1.25)), one(k(0xE2, 1.25)), one(k(0x2C, 6.25)), one(k(0xE6, 1)), one(k(0x65, 1)), one(k(0xE4, 1)), keys(0x50, 0x51, 0x4F))
	if iso {
		caps = caps[:len(caps)-1]
		caps = cat(caps, one(gap(1.25)))
	}
	var rows []row
	if seventyFive {
		rows = []row{
			{0, cat(one(k(0x29, 1)), fRow1, fRow2, fRow3, one(k(0x4C, 1)))},
			{1.25, cat(numbers, one(k(0x2A, 2)), right(0x4A))},
			{2.25, cat(tab, right(0x4B))},
			{3.25, cat(caps, right(0x4E))},
			{4.25, cat(shift, one(k(0x52, 1)), right(0x4D))},
			{5.25, bottom},
		}
		return build("75-"+std(iso), "75", 75, std(iso), rows)
	}
	rows = []row{
		{0, cat(numbers, one(k(0x2A, 2)), right(0x4C))},
		{1, cat(tab, right(0x4B))},
		{2, cat(caps, right(0x4E))},
		{3, cat(shift, one(k(0x52, 1)), right(0x4D))},
		{4, bottom},
	}
	return build("65-"+std(iso), "65", 65, std(iso), rows)
}

func sixty(iso bool) Layout {
	tab, caps, shift := letterRows(iso, 1.5, 1.75, 2.75, 1.5)
	if iso {
		caps = caps[:len(caps)-1]
	}
	rows := []row{
		{0, cat(numbers, one(k(0x2A, 2)))},
		{1, tab},
		{2, caps},
		{3, shift},
		{4, space},
	}
	return build("60-"+std(iso), "60", 60, std(iso), rows)
}

func numpad() Layout {
	rows := []row{
		{0, keys(0x53, 0x54, 0x55, 0x56)},
		{1, cat(keys(0x5F, 0x60, 0x61), one(tall(0x57, 1)))},
		{2, keys(0x5C, 0x5D, 0x5E)},
		{3, cat(keys(0x59, 0x5A, 0x5B), one(tall(0x58, 1)))},
		{4, cat(one(k(0x62, 2)), one(k(0x63, 1)))},
	}
	return build("numpad", "numpad", 0, "", rows)
}

var all = func() []Layout {
	var out []Layout
	for _, iso := range []bool{true, false} {
		out = append(out, full(iso), tkl(iso), compact(iso, true), compact(iso, false), sixty(iso))
	}
	out = append(out, numpad())
	return out
}()

func All() []Layout { return all }

func Get(id string) (Layout, bool) {
	id = Canonical(id)
	for _, l := range all {
		if l.ID == id {
			return l, true
		}
	}
	return Layout{}, false
}

func Canonical(id string) string {
	switch id {
	case "iso-full":
		return "full-iso"
	case "ansi-full":
		return "full-ansi"
	case "iso-tkl":
		return "tkl-iso"
	case "ansi-tkl":
		return "tkl-ansi"
	}
	return id
}

func (l Layout) KeySet() map[byte]bool {
	s := map[byte]bool{}
	for _, key := range l.Keys {
		if !key.Mod {
			s[key.Usage] = true
		}
	}
	return s
}

type Suggestion struct {
	ID        string  `json:"id"`
	Score     float64 `json:"score"`
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	Extra     []int   `json:"extra,omitempty"`
	Missing   []int   `json:"missing,omitempty"`
}

func Match(seenRaw map[byte]bool) []Suggestion {
	var out []Suggestion
	for _, l := range all {
		set := l.KeySet()
		seen := seenRaw
		if l.Standard == "iso" && seenRaw[0x31] && seenRaw[0x64] && !seenRaw[0x32] {
			seen = map[byte]bool{}
			for u := range seenRaw {
				if u == 0x31 {
					seen[0x32] = true
				} else {
					seen[u] = true
				}
			}
		}
		var inter float64
		var extra, missing []int
		for u := range seen {
			if IsModifier(u) || u < 4 {
				continue
			}
			if set[u] {
				inter++
			} else {
				extra = append(extra, int(u))
			}
		}
		for u := range set {
			if !seen[u] {
				missing = append(missing, int(u))
			}
		}
		sort.Ints(extra)
		sort.Ints(missing)
		n := 0.0
		for u := range seen {
			if !IsModifier(u) && u >= 4 {
				n++
			}
		}
		if n == 0 {
			continue
		}
		p, r := inter/n, inter/float64(len(set))
		s := 0.0
		if p+r > 0 {
			s = 2 * p * r / (p + r)
		}
		out = append(out, Suggestion{ID: l.ID, Score: s, Precision: p, Recall: r, Extra: extra, Missing: missing})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

func Ambiguity(r []Suggestion) (bool, []int) {
	if len(r) < 2 {
		return false, nil
	}
	a, _ := Get(r[0].ID)
	b, _ := Get(r[1].ID)
	if a.Family != b.Family || a.Standard == b.Standard || a.Standard == "" || r[0].Score-r[1].Score > 0.06 || len(r[0].Extra) > 0 || len(r[1].Extra) > 0 {
		return false, nil
	}
	var hints []int
	for _, u := range []int{0x64, 0x32, 0x31} {
		if !contains(r[0].Missing, u) && !contains(r[1].Missing, u) {
			continue
		}
		hints = append(hints, u)
	}
	return len(hints) > 0, hints
}

func contains(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
