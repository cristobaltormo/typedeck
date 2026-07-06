package board

import (
	"path/filepath"
	"sort"
)

func systemPorts() []string {
	m, _ := filepath.Glob("/dev/cu.usbmodem*")
	sort.Strings(m)
	return m
}
