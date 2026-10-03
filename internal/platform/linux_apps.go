//go:build linux

package platform

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type desktopEntry struct {
	ID      string
	Name    string
	Exec    string
	WMClass string
	Path    string
}

var (
	entriesMu sync.Mutex
	entries   []desktopEntry
	entriesAt time.Time
)

var sandboxedAppDirs = func(home string) []string {
	return []string{"/var/lib/flatpak/exports/share", filepath.Join(home, ".local/share/flatpak/exports/share"), "/var/lib/snapd/desktop"}
}

func dataDirs() []string {
	home, _ := os.UserHomeDir()
	dirs := []string{filepath.Join(home, ".local/share")}
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		dirs[0] = x
	}
	xdg := os.Getenv("XDG_DATA_DIRS")
	if xdg == "" {
		xdg = "/usr/local/share:/usr/share"
	}
	dirs = append(dirs, strings.Split(xdg, ":")...)
	dirs = append(dirs, sandboxedAppDirs(home)...)
	return dirs
}

func parseDesktop(path string) (desktopEntry, bool) {
	f, err := os.Open(path)
	if err != nil {
		return desktopEntry{}, false
	}
	defer f.Close()
	e := desktopEntry{ID: strings.TrimSuffix(filepath.Base(path), ".desktop"), Path: path}
	inMain, typ, hidden := false, "", false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(l, "[") {
			inMain = l == "[Desktop Entry]"
			continue
		}
		if !inMain || l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			continue
		}
		switch k {
		case "Name":
			e.Name = v
		case "Exec":
			e.Exec = v
		case "StartupWMClass":
			e.WMClass = v
		case "Type":
			typ = v
		case "NoDisplay", "Hidden":
			if strings.EqualFold(v, "true") {
				hidden = true
			}
		}
	}
	if typ != "Application" || hidden || e.Name == "" || e.Exec == "" {
		return e, false
	}
	return e, true
}

func loadEntries() []desktopEntry {
	entriesMu.Lock()
	defer entriesMu.Unlock()
	if entries != nil && time.Since(entriesAt) < 60*time.Second {
		return entries
	}
	seen := map[string]bool{}
	var out []desktopEntry
	for _, d := range dataDirs() {
		_ = filepath.WalkDir(filepath.Join(d, "applications"), func(p string, de os.DirEntry, err error) error {
			if err != nil || de.IsDir() || !strings.HasSuffix(p, ".desktop") {
				return nil
			}
			e, ok := parseDesktop(p)
			if ok && !seen[e.ID] {
				seen[e.ID] = true
				out = append(out, e)
			}
			return nil
		})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	entries, entriesAt = out, time.Now()
	return out
}

func findEntry(name string) (desktopEntry, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return desktopEntry{}, false
	}
	list := loadEntries()
	for _, e := range list {
		if strings.ToLower(e.Name) == n || strings.ToLower(e.ID) == n || (e.WMClass != "" && strings.ToLower(e.WMClass) == n) {
			return e, true
		}
	}
	for _, e := range list {
		if strings.Contains(strings.ToLower(e.Name), n) || strings.HasSuffix(strings.ToLower(e.ID), "."+n) {
			return e, true
		}
	}
	return desktopEntry{}, false
}

func entryFor(class, comm string) (desktopEntry, bool) {
	for _, e := range loadEntries() {
		if class != "" && (strings.EqualFold(e.WMClass, class) || strings.EqualFold(e.ID, class)) {
			return e, true
		}
	}
	for _, e := range loadEntries() {
		if comm != "" && strings.EqualFold(execBase(e.Exec), comm) {
			return e, true
		}
	}
	return desktopEntry{}, false
}

func execBase(exec string) string {
	f := strings.Fields(exec)
	for len(f) > 0 && (f[0] == "env" || strings.Contains(f[0], "=")) {
		f = f[1:]
	}
	if len(f) == 0 {
		return ""
	}
	return filepath.Base(strings.Trim(f[0], `"`))
}

func expandExec(exec string) []string {
	var out []string
	for _, w := range strings.Fields(exec) {
		if len(w) == 2 && w[0] == '%' {
			continue
		}
		out = append(out, strings.Trim(w, `"`))
	}
	return out
}
