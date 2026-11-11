package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"unicode/utf16"
)

const (
	rtIcon      = 3
	rtGroupIcon = 14
	rtVersion   = 16
	rtManifest  = 24

	langNeutral = 0
	langUS      = 0x0409
)

type resource struct {
	kind, id, lang uint16
	data           []byte
}

type machine struct {
	id        uint16
	addr32NB  uint16
	archLabel string
}

var machines = map[string]machine{
	"amd64": {0x8664, 3, "amd64"},
	"arm64": {0xAA64, 2, "arm64"},
}

func main() {
	arch := flag.String("arch", "amd64", "amd64 or arm64")
	version := flag.String("version", "dev", "release version, for example v1.2.3")
	manifest := flag.String("manifest", "", "path to the manifest")
	icon := flag.String("icon", "", "path to the .ico file")
	out := flag.String("o", "", "output .syso file")
	flag.Parse()
	m, ok := machines[*arch]
	if !ok || *manifest == "" || *icon == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	man, err := os.ReadFile(*manifest)
	check(err)
	ico, err := os.ReadFile(*icon)
	check(err)
	res, err := buildResources(man, ico, *version)
	check(err)
	check(os.WriteFile(*out, buildObject(m, res), 0o644))
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "mkres:", err)
		os.Exit(1)
	}
}

func buildResources(manifest, ico []byte, version string) ([]resource, error) {
	icons, group, err := splitIcon(ico)
	if err != nil {
		return nil, err
	}
	res := []resource{{rtManifest, 1, langNeutral, manifest}, {rtGroupIcon, 1, langNeutral, group}, {rtVersion, 1, langUS, versionInfo(version)}}
	return append(res, icons...), nil
}

func splitIcon(ico []byte) (icons []resource, group []byte, err error) {
	if len(ico) < 6 || binary.LittleEndian.Uint16(ico[2:]) != 1 {
		return nil, nil, fmt.Errorf("not an .ico file")
	}
	n := int(binary.LittleEndian.Uint16(ico[4:]))
	var g bytes.Buffer
	binary.Write(&g, binary.LittleEndian, [3]uint16{0, 1, uint16(n)})
	for i := 0; i < n; i++ {
		e := ico[6+16*i:]
		if len(ico) < 6+16*(i+1) {
			return nil, nil, fmt.Errorf("truncated .ico file")
		}
		size, off := binary.LittleEndian.Uint32(e[8:]), binary.LittleEndian.Uint32(e[12:])
		if int(off+size) > len(ico) {
			return nil, nil, fmt.Errorf("truncated .ico file")
		}
		id := uint16(i + 1)
		icons = append(icons, resource{rtIcon, id, langNeutral, ico[off : off+size]})
		g.Write(e[:4])
		binary.Write(&g, binary.LittleEndian, [2]uint16{1, binary.LittleEndian.Uint16(e[6:])})
		binary.Write(&g, binary.LittleEndian, size)
		binary.Write(&g, binary.LittleEndian, id)
	}
	return icons, g.Bytes(), nil
}

var versionParts = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)`)

func numericVersion(v string) [4]uint16 {
	var out [4]uint16
	if m := versionParts.FindStringSubmatch(v); m != nil {
		for i := 0; i < 3; i++ {
			n, _ := strconv.Atoi(m[i+1])
			out[i] = uint16(n)
		}
	}
	return out
}

func pad4(b *bytes.Buffer) {
	for b.Len()%4 != 0 {
		b.WriteByte(0)
	}
}

func utf16z(s string) []byte {
	u := utf16.Encode([]rune(s + "\x00"))
	out := make([]byte, len(u)*2)
	for i, c := range u {
		binary.LittleEndian.PutUint16(out[i*2:], c)
	}
	return out
}

func block(key string, valueLen, kind uint16, value []byte, children ...[]byte) []byte {
	var b bytes.Buffer
	b.Write([]byte{0, 0})
	binary.Write(&b, binary.LittleEndian, [2]uint16{valueLen, kind})
	b.Write(utf16z(key))
	pad4(&b)
	b.Write(value)
	for _, c := range children {
		pad4(&b)
		b.Write(c)
	}
	out := b.Bytes()
	binary.LittleEndian.PutUint16(out, uint16(len(out)))
	return out
}

func fixedFileInfo(v [4]uint16) []byte {
	ms, ls := uint32(v[0])<<16|uint32(v[1]), uint32(v[2])<<16|uint32(v[3])
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, []uint32{0xFEEF04BD, 0x00010000, ms, ls, ms, ls, 0x3F, 0, 0x40004, 1, 0, 0, 0})
	return b.Bytes()
}

func versionInfo(version string) []byte {
	v := numericVersion(version)
	text := fmt.Sprintf("%d.%d.%d.%d", v[0], v[1], v[2], v[3])
	if version != "" && version != text {
		text = version
	}
	fields := [][2]string{
		{"CompanyName", "Cristobal Tormo"},
		{"FileDescription", "Typedeck, a macro keyboard companion"},
		{"FileVersion", text},
		{"InternalName", "typedeck"},
		{"LegalCopyright", "Copyright 2026 Cristobal Tormo. MIT License."},
		{"OriginalFilename", "typedeck.exe"},
		{"ProductName", "Typedeck"},
		{"ProductVersion", text},
	}
	var strs [][]byte
	for _, f := range fields {
		val := utf16z(f[1])
		strs = append(strs, block(f[0], uint16(len(val)/2), 1, val))
	}
	var padded [][]byte
	for _, s := range strs {
		var b bytes.Buffer
		b.Write(s)
		pad4(&b)
		padded = append(padded, b.Bytes())
	}
	table := block("040904B0", 0, 1, nil, padded...)
	stringInfo := block("StringFileInfo", 0, 1, nil, table)
	translation := make([]byte, 4)
	binary.LittleEndian.PutUint32(translation, 0x04B00409)
	varInfo := block("VarFileInfo", 0, 1, nil, block("Translation", 4, 0, translation))
	return block("VS_VERSION_INFO", 52, 0, fixedFileInfo(v), stringInfo, varInfo)
}

type dirNode struct {
	id       uint16
	children []*dirNode
	leaf     *resource
}

func buildTree(res []resource) *dirNode {
	root := &dirNode{}
	types := map[uint16]*dirNode{}
	names := map[[2]uint16]*dirNode{}
	for i := range res {
		r := &res[i]
		t := types[r.kind]
		if t == nil {
			t = &dirNode{id: r.kind}
			types[r.kind] = t
			root.children = append(root.children, t)
		}
		key := [2]uint16{r.kind, r.id}
		n := names[key]
		if n == nil {
			n = &dirNode{id: r.id}
			names[key] = n
			t.children = append(t.children, n)
		}
		n.children = append(n.children, &dirNode{id: r.lang, leaf: r})
	}
	sortTree(root)
	return root
}

func sortTree(n *dirNode) {
	sort.Slice(n.children, func(i, j int) bool { return n.children[i].id < n.children[j].id })
	for _, c := range n.children {
		sortTree(c)
	}
}

type section struct {
	buf    bytes.Buffer
	relocs []uint32
}

func dirSize(n *dirNode) int { return 16 + 8*len(n.children) }

func layoutDirs(n *dirNode, offsets map[*dirNode]int, next *int) {
	offsets[n] = *next
	*next += dirSize(n)
	for _, c := range n.children {
		if c.leaf == nil {
			layoutDirs(c, offsets, next)
		}
	}
}

func writeDirs(s *section, n *dirNode, offsets, entries map[*dirNode]int) {
	binary.Write(&s.buf, binary.LittleEndian, [4]uint32{0, 0, 0, uint32(len(n.children)) << 16})
	for _, c := range n.children {
		target := offsets[c]
		if c.leaf != nil {
			target = entries[c]
		} else {
			target |= 0x80000000
		}
		binary.Write(&s.buf, binary.LittleEndian, [2]uint32{uint32(c.id), uint32(target)})
	}
	for _, c := range n.children {
		if c.leaf == nil {
			writeDirs(s, c, offsets, entries)
		}
	}
}

func collectLeaves(n *dirNode, out *[]*dirNode) {
	for _, c := range n.children {
		if c.leaf != nil {
			*out = append(*out, c)
		} else {
			collectLeaves(c, out)
		}
	}
}

func buildSection(res []resource) *section {
	root := buildTree(res)
	offsets := map[*dirNode]int{}
	next := 0
	layoutDirs(root, offsets, &next)
	var leaves []*dirNode
	collectLeaves(root, &leaves)
	entries := map[*dirNode]int{}
	dataAt := next + 16*len(leaves)
	s := &section{}
	for i, l := range leaves {
		entries[l] = next + 16*i
	}
	writeDirs(s, root, offsets, entries)
	pos := dataAt
	for _, l := range leaves {
		s.relocs = append(s.relocs, uint32(s.buf.Len()))
		binary.Write(&s.buf, binary.LittleEndian, [4]uint32{uint32(pos), uint32(len(l.leaf.data)), 0, 0})
		pos += (len(l.leaf.data) + 3) &^ 3
	}
	for _, l := range leaves {
		s.buf.Write(l.leaf.data)
		pad4(&s.buf)
	}
	return s
}

func buildObject(m machine, res []resource) []byte {
	sec := buildSection(res)
	raw := sec.buf.Bytes()
	const headerSize, sectionHeader = 20, 40
	rawPtr := headerSize + sectionHeader
	relocPtr := rawPtr + len(raw)
	symPtr := relocPtr + 10*len(sec.relocs)
	var o bytes.Buffer
	binary.Write(&o, binary.LittleEndian, struct {
		Machine, Sections uint16
		Time, SymPtr, Sym uint32
		Opt, Chars        uint16
	}{m.id, 1, 0, uint32(symPtr), 2, 0, 0})
	name := [8]byte{'.', 'r', 's', 'r', 'c'}
	binary.Write(&o, binary.LittleEndian, struct {
		Name                    [8]byte
		VSize, VAddr, Size, Ptr uint32
		RelocPtr, LinePtr       uint32
		Relocs, Lines           uint16
		Chars                   uint32
	}{name, 0, 0, uint32(len(raw)), uint32(rawPtr), uint32(relocPtr), 0, uint16(len(sec.relocs)), 0, 0x40000040})
	o.Write(raw)
	for _, r := range sec.relocs {
		binary.Write(&o, binary.LittleEndian, struct {
			Addr, Sym uint32
			Type      uint16
		}{r, 0, m.addr32NB})
	}
	binary.Write(&o, binary.LittleEndian, struct {
		Name    [8]byte
		Value   uint32
		Section int16
		Type    uint16
		Class   uint8
		Aux     uint8
	}{name, 0, 1, 0, 3, 1})
	binary.Write(&o, binary.LittleEndian, struct {
		Length    uint32
		Relocs    uint16
		Lines     uint16
		Checksum  uint32
		Number    uint16
		Selection uint8
		Pad       [3]byte
	}{uint32(len(raw)), uint16(len(sec.relocs)), 0, 0, 1, 0, [3]byte{}})
	binary.Write(&o, binary.LittleEndian, uint32(4))
	return o.Bytes()
}
