//go:build windows

package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var pShellExecute = syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW")

type shortcut struct{ Name, Path, Target, AUMID string }

var (
	lnkMu    sync.Mutex
	lnks     []shortcut
	lnkAt    time.Time
	storeMu  sync.Mutex
	store    []shortcut
	storeAt  time.Time
	storeRun bool
)

const storeScript = `[Console]::OutputEncoding=[Text.Encoding]::UTF8;Get-StartApps | ForEach-Object { $_.Name + '|' + $_.AppID }`

func startMenuDirs() []string {
	var dirs []string
	if p := os.Getenv("ProgramData"); p != "" {
		dirs = append(dirs, filepath.Join(p, `Microsoft\Windows\Start Menu\Programs`))
	}
	if p := os.Getenv("APPDATA"); p != "" {
		dirs = append(dirs, filepath.Join(p, `Microsoft\Windows\Start Menu\Programs`))
	}
	return dirs
}

func isUninstaller(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "uninstall") || strings.Contains(n, "desinstalar")
}

func scanShortcuts() []shortcut {
	seen := map[string]bool{}
	var out []shortcut
	for _, d := range startMenuDirs() {
		_ = filepath.WalkDir(d, func(p string, de os.DirEntry, err error) error {
			if err != nil || de.IsDir() || !strings.EqualFold(filepath.Ext(p), ".lnk") {
				return nil
			}
			n := strings.TrimSuffix(de.Name(), filepath.Ext(p))
			if isUninstaller(n) || seen[strings.ToLower(n)] {
				return nil
			}
			seen[strings.ToLower(n)] = true
			data, _ := os.ReadFile(p)
			out = append(out, shortcut{Name: n, Path: p, Target: lnkTarget(data)})
			return nil
		})
	}
	return out
}

func parseStoreApps(out string) []shortcut {
	var apps []shortcut
	for _, line := range strings.Split(out, "\n") {
		name, id, ok := strings.Cut(strings.TrimSpace(line), "|")
		if ok && name != "" && strings.Contains(id, "!") && !isUninstaller(name) {
			apps = append(apps, shortcut{Name: name, AUMID: id})
		}
	}
	return apps
}

func refreshStoreApps() {
	storeMu.Lock()
	if storeRun || (store != nil && time.Since(storeAt) < 10*time.Minute) {
		storeMu.Unlock()
		return
	}
	storeRun = true
	storeMu.Unlock()
	go func() {
		out, err := Run(15*time.Second, "", "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", storeScript)
		storeMu.Lock()
		defer storeMu.Unlock()
		storeRun, storeAt = false, time.Now()
		if err == nil {
			store = parseStoreApps(out)
		} else if store == nil {
			store = []shortcut{}
		}
	}()
}

func storeApps() []shortcut {
	refreshStoreApps()
	storeMu.Lock()
	defer storeMu.Unlock()
	return store
}

func shortcuts() []shortcut {
	lnkMu.Lock()
	if lnks == nil || time.Since(lnkAt) >= 60*time.Second {
		lnks, lnkAt = scanShortcuts(), time.Now()
	}
	base := lnks
	lnkMu.Unlock()
	known := map[string]bool{}
	for _, s := range base {
		known[strings.ToLower(s.Name)] = true
	}
	out := append([]shortcut{}, base...)
	for _, s := range storeApps() {
		if !known[strings.ToLower(s.Name)] {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func findShortcut(name string) (shortcut, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	all := shortcuts()
	for _, s := range all {
		if strings.ToLower(s.Name) == n {
			return s, true
		}
	}
	for _, s := range all {
		if strings.Contains(strings.ToLower(s.Name), n) {
			return s, true
		}
	}
	return shortcut{}, false
}

func shortcutNamesFor(image, base string) []string {
	var names []string
	for _, s := range shortcuts() {
		if s.Target != "" && strings.EqualFold(filepath.Base(s.Target), filepath.Base(image)) {
			names = append(names, s.Name)
		} else if s.Target == "" && s.AUMID == "" && strings.EqualFold(s.Name, base) {
			names = append(names, s.Name)
		}
	}
	return names
}

func imageFor(name string) string {
	if s, ok := findShortcut(name); ok {
		if s.Target != "" {
			return filepath.Base(s.Target)
		}
		if s.AUMID != "" {
			return ""
		}
	}
	if strings.EqualFold(filepath.Ext(name), ".exe") {
		return filepath.Base(name)
	}
	return name + ".exe"
}

func shellExecute(file string) error {
	f, err := syscall.UTF16PtrFromString(file)
	if err != nil {
		return err
	}
	r, _, _ := pShellExecute.Call(0, uintptr(unsafe.Pointer(utf16Ptr("open"))), uintptr(unsafe.Pointer(f)), 0, 0, 1)
	if r <= 32 {
		return fmt.Errorf("could not open %q (code %d)", file, r)
	}
	return nil
}

func (*windows) OpenApp(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("the app name is missing")
	}
	s, found := findShortcut(name)
	if image := imageFor(name); image != "" {
		if ws := windowsOfImage(image); len(ws) > 0 {
			bringToFront(ws[0].hwnd)
			return nil
		}
	}
	switch {
	case found && s.AUMID != "":
		return Detached("explorer.exe", `shell:AppsFolder\`+s.AUMID)
	case found:
		return shellExecute(s.Path)
	}
	return shellExecute(name)
}

func (*windows) QuitApp(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("the app name is missing")
	}
	if s, ok := findShortcut(name); ok && s.AUMID != "" {
		return closeStoreApp(s.Name)
	}
	exe := imageFor(name)
	if ws := windowsOfImage(exe); len(ws) > 0 {
		closeWindows(ws)
		return nil
	}
	if _, err := Run(5*time.Second, "", "taskkill", "/IM", exe); err == nil {
		return nil
	}
	return fmt.Errorf("could not close %q (is it open?)", name)
}

func closeStoreApp(title string) error {
	var ws []topWindow
	for _, w := range topWindows() {
		if w.class == "ApplicationFrameWindow" && strings.EqualFold(w.title, title) {
			ws = append(ws, w)
		}
	}
	if len(ws) == 0 {
		return fmt.Errorf("could not close %q (is it open?)", title)
	}
	closeWindows(ws)
	return nil
}

func (*windows) InstalledApps() []string {
	var out []string
	for _, s := range shortcuts() {
		out = append(out, s.Name)
	}
	return out
}
