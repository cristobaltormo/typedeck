package hid

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

type Report struct {
	ID     int    `json:"id"`
	Dir    string `json:"dir"`
	Kind   string `json:"kind"`
	Bits   int    `json:"bits"`
	Detail string `json:"detail"`
}

type field struct {
	id, page, size, count int
	usages                []int
	umin, umax            int
	flags                 int
	dir                   string
}

func ParseReportDescriptor(data []byte) []Report {
	var fields []field
	type globals struct{ page, size, count, id, lmin, lmax int }
	var g globals
	var stack []globals
	var usages []int
	umin, umax := -1, -1
	for i := 0; i < len(data); {
		b := data[i]
		if b == 0xFE {
			if i+1 >= len(data) {
				break
			}
			i += 3 + int(data[i+1])
			continue
		}
		size := int(b & 3)
		if size == 3 {
			size = 4
		}
		typ, tag := int(b>>2)&3, int(b>>4)
		val := 0
		for k := 0; k < size && i+1+k < len(data); k++ {
			val |= int(data[i+1+k]) << (8 * k)
		}
		i += 1 + size
		switch typ {
		case 1:
			switch tag {
			case 0:
				g.page = val
			case 1:
				g.lmin = val
			case 2:
				g.lmax = val
			case 7:
				g.size = val
			case 8:
				g.id = val
			case 9:
				g.count = val
			case 0xA:
				stack = append(stack, g)
			case 0xB:
				if n := len(stack); n > 0 {
					g = stack[n-1]
					stack = stack[:n-1]
				}
			}
		case 2:
			switch tag {
			case 0:
				usages = append(usages, val)
			case 1:
				umin = val
			case 2:
				umax = val
			}
		case 0:
			if tag == 8 || tag == 9 || tag == 0xB {
				dir := map[int]string{8: "input", 9: "output", 0xB: "feature"}[tag]
				fields = append(fields, field{id: g.id, page: g.page, size: g.size, count: g.count,
					usages: append([]int(nil), usages...), umin: umin, umax: umax, flags: val, dir: dir})
			}
			if tag != 0xA && tag != 0xC || tag == 0xA {
				usages, umin, umax = nil, -1, -1
			}
		}
	}
	return summarise(fields)
}

func ParseReportDescriptorHex(s string) ([]Report, error) {
	s = strings.Join(strings.Fields(s), "")
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	return ParseReportDescriptor(b), nil
}

func summarise(fields []field) []Report {
	type key struct {
		id  int
		dir string
	}
	groups := map[key][]field{}
	var order []key
	for _, f := range fields {
		k := key{f.id, f.dir}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], f)
	}
	sort.SliceStable(order, func(a, b int) bool { return order[a].id < order[b].id })
	var out []Report
	for _, k := range order {
		r := Report{ID: k.id, Dir: k.dir, Kind: "other"}
		var detail []string
		for _, f := range groups[k] {
			r.Bits += f.size * f.count
			switch {
			case f.page == 0x07 && f.flags&1 == 0 && f.size == 1 && f.count == 8 && (f.umin == 0xE0 || contains(f.usages, 0xE0)):
				r.Kind = "keyboard"
				detail = append(detail, "modificadores")
			case f.page == 0x07 && f.size == 1 && f.count > 8 && f.umin >= 0:
				r.Kind, detail = "nkro", append(detail, fmt.Sprintf("%d teclas a la vez", f.count))
			case f.page == 0x07 && f.size == 8 && f.count > 1 && f.flags&2 == 0:
				r.Kind, detail = "keyboard", append(detail, fmt.Sprintf("hasta %d teclas a la vez", f.count))
			case f.page == 0x08 && f.dir == "output":
				detail = append(detail, "LED del teclado")
			case f.page == 0x0C && r.Kind != "mouse":
				r.Kind, detail = "consumer", append(detail, "teclas multimedia")
			case f.page == 0x01 && contains(f.usages, 0x80) || f.page == 0x01 && f.umin == 0x81:
				r.Kind, detail = "system", append(detail, "encendido, suspender y despertar")
			case f.page == 0x01 && (contains(f.usages, 0x30) || contains(f.usages, 0x31)):
				r.Kind, detail = "mouse", append(detail, "movimiento del ratón")
			case f.page == 0x09:
				if r.Kind == "other" {
					r.Kind = "mouse"
				}
				detail = append(detail, "botones")
			case f.page >= 0xFF00:
				r.Kind = "vendor"
			}
		}
		if r.Kind == "mouse" && r.Dir == "input" && len(detail) > 0 {
			detail = []string{"ratón integrado"}
		}
		if r.Kind == "vendor" {
			detail = []string{"datos del fabricante"}
		}
		r.Detail = strings.Join(dedupe(detail), ", ")
		out = append(out, r)
	}
	return out
}

func contains(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
