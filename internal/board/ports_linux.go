package board

import (
	"path/filepath"
	"sort"
)

func systemPorts() []string {
	m, _ := filepath.Glob("/dev/ttyACM*")
	sort.Strings(m)
	return m
}
