//go:build linux

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	sessionMu        sync.Mutex
	sessionCheckedAt time.Time
	runtimeDir       = func() string { return fmt.Sprintf("/run/user/%d", os.Getuid()) }
)

func discoverSession() {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	if time.Since(sessionCheckedAt) < 3*time.Second {
		return
	}
	sessionCheckedAt = time.Now()
	dir := runtimeDir()
	if os.Getenv("SWAYSOCK") == "" {
		if m, _ := filepath.Glob(filepath.Join(dir, "sway-ipc.*.sock")); len(m) > 0 {
			_ = os.Setenv("SWAYSOCK", m[0])
		}
	}
	if os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") == "" {
		if m, _ := filepath.Glob(filepath.Join(dir, "hypr", "*")); len(m) > 0 && dirExists(m[0]) {
			_ = os.Setenv("HYPRLAND_INSTANCE_SIGNATURE", filepath.Base(m[0]))
		}
	}
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		if m, _ := filepath.Glob(filepath.Join(dir, "wayland-[0-9]*")); len(m) > 0 {
			for _, p := range m {
				if filepath.Ext(p) == "" {
					_ = os.Setenv("WAYLAND_DISPLAY", filepath.Base(p))
					break
				}
			}
		}
	}
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" && fileExists("/tmp/.X11-unix/X0") {
		_ = os.Setenv("DISPLAY", ":0")
	}
}
