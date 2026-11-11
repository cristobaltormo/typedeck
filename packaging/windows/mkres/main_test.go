package main

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"os"
	"testing"
)

func readTree(t *testing.T, raw []byte, off uint32, path []uint16, out map[[3]uint16][]byte, rvaBase func(uint32) uint32) {
	t.Helper()
	count := int(binary.LittleEndian.Uint16(raw[off+12:])) + int(binary.LittleEndian.Uint16(raw[off+14:]))
	for i := 0; i < count; i++ {
		e := raw[int(off)+16+8*i:]
		id, target := binary.LittleEndian.Uint32(e), binary.LittleEndian.Uint32(e[4:])
		next := append(append([]uint16{}, path...), uint16(id))
		if target&0x80000000 != 0 {
			readTree(t, raw, target&0x7FFFFFFF, next, out, rvaBase)
			continue
		}
		rva := binary.LittleEndian.Uint32(raw[target:])
		size := binary.LittleEndian.Uint32(raw[target+4:])
		out[[3]uint16{next[0], next[1], next[2]}] = raw[rvaBase(rva) : rvaBase(rva)+size]
	}
}

func TestObjectRoundTrip(t *testing.T) {
	manifest, err := os.ReadFile("../typedeck.manifest")
	if err != nil {
		t.Fatal(err)
	}
	ico, err := os.ReadFile("../typedeck.ico")
	if err != nil {
		t.Fatal(err)
	}
	for arch, m := range machines {
		res, err := buildResources(manifest, ico, "v1.2.3")
		if err != nil {
			t.Fatal(err)
		}
		obj := buildObject(m, res)
		f, err := pe.NewFile(bytes.NewReader(obj))
		if err != nil {
			t.Fatalf("%s: %v", arch, err)
		}
		sec := f.Section(".rsrc")
		if sec == nil || f.Machine != m.id {
			t.Fatalf("%s: wrong object", arch)
		}
		raw, _ := sec.Data()
		found := map[[3]uint16][]byte{}
		readTree(t, raw, 0, nil, found, func(v uint32) uint32 { return v })
		if !bytes.Equal(found[[3]uint16{rtManifest, 1, langNeutral}], manifest) {
			t.Errorf("%s: the manifest does not survive", arch)
		}
		if len(found[[3]uint16{rtVersion, 1, langUS}]) == 0 || len(found[[3]uint16{rtGroupIcon, 1, langNeutral}]) == 0 {
			t.Errorf("%s: missing version or icon group", arch)
		}
		if int(sec.NumberOfRelocations) != len(found) {
			t.Errorf("%s: %d relocations for %d resources", arch, sec.NumberOfRelocations, len(found))
		}
	}
}

func TestNumericVersion(t *testing.T) {
	cases := map[string][4]uint16{"v1.2.3": {1, 2, 3, 0}, "0.8.9-4-gabc": {0, 8, 9, 0}, "dev": {}}
	for in, want := range cases {
		if got := numericVersion(in); got != want {
			t.Errorf("%s: %v, expected %v", in, got, want)
		}
	}
}

func TestVersionInfoIsWellFormed(t *testing.T) {
	v := versionInfo("v1.2.3")
	if int(binary.LittleEndian.Uint16(v)) != len(v) {
		t.Fatal("the length of the root block does not match")
	}
	if !bytes.Contains(v, utf16z("Typedeck")) || !bytes.Contains(v, utf16z("v1.2.3")) {
		t.Fatal("missing strings")
	}
	if binary.LittleEndian.Uint32(v[40:]) != 0xFEEF04BD {
		t.Fatal("the fixed file info signature is missing")
	}
}

func TestRejectsTruncatedIcon(t *testing.T) {
	if _, _, err := splitIcon([]byte{0, 0, 1, 0, 1, 0}); err == nil {
		t.Fatal("a truncated icon must fail")
	}
}
