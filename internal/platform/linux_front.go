//go:build linux

package platform

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type frontInfo struct {
	id    string
	class string
	pid   int
	x11   bool
}

func front() frontInfo {
	if os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != "" && have("hyprctl") {
		out, err := Run(2*time.Second, "", "hyprctl", "activewindow", "-j")
		if err == nil {
			var w struct {
				Address string `json:"address"`
				Class   string `json:"class"`
				PID     int    `json:"pid"`
			}
			if json.Unmarshal([]byte(out), &w) == nil && w.Address != "" {
				return frontInfo{id: w.Address, class: w.Class, pid: w.PID}
			}
		}
		return frontInfo{}
	}
	if os.Getenv("SWAYSOCK") != "" && have("swaymsg") {
		out, err := Run(2*time.Second, "", "swaymsg", "-t", "get_tree")
		if err == nil {
			var root map[string]any
			if json.Unmarshal([]byte(out), &root) == nil {
				if n := focusedNode(root); n != nil {
					id, _ := n["id"].(float64)
					class, _ := n["app_id"].(string)
					if class == "" {
						if wp, ok := n["window_properties"].(map[string]any); ok {
							class, _ = wp["class"].(string)
						}
					}
					pid, _ := n["pid"].(float64)
					return frontInfo{id: strconv.Itoa(int(id)), class: class, pid: int(pid)}
				}
			}
		}
		return frontInfo{}
	}
	if have("xprop") && os.Getenv("DISPLAY") != "" {
		out, _ := Run(2*time.Second, "", "xprop", "-root", "_NET_ACTIVE_WINDOW")
		if i := strings.LastIndex(out, "0x"); i >= 0 {
			if id := strings.TrimSpace(out[i:]); id != "0x0" {
				return frontInfo{id: id, x11: true}
			}
		}
		if have("xdotool") {
			if out, err := Run(2*time.Second, "", "xdotool", "getwindowfocus"); err == nil && strings.TrimSpace(out) != "" {
				return frontInfo{id: strings.TrimSpace(out), x11: true}
			}
		}
	}
	return frontInfo{}
}

func focusedNode(n map[string]any) map[string]any {
	if f, _ := n["focused"].(bool); f {
		return n
	}
	for _, k := range []string{"nodes", "floating_nodes"} {
		if kids, ok := n[k].([]any); ok {
			for _, c := range kids {
				if m, ok := c.(map[string]any); ok {
					if r := focusedNode(m); r != nil {
						return r
					}
				}
			}
		}
	}
	return nil
}

func (*linux) FrontID() string { return front().id }

func (*linux) FrontNames(id string) []string {
	f := front()
	if f.id == "" || (id != "" && f.id != id) {
		return nil
	}
	if f.class == "" && f.pid == 0 && f.x11 {
		out, _ := Run(2*time.Second, "", "xprop", "-id", f.id, "WM_CLASS", "_NET_WM_PID")
		for _, l := range strings.Split(out, "\n") {
			if _, v, ok := strings.Cut(l, "WM_CLASS(STRING) = "); ok {
				parts := strings.Split(v, ",")
				f.class = strings.Trim(strings.TrimSpace(parts[len(parts)-1]), `"`)
				if len(parts) > 1 {
					f.class = strings.Trim(strings.TrimSpace(parts[0]), `"`) + "\x00" + f.class
				}
			}
			if _, v, ok := strings.Cut(l, "_NET_WM_PID(CARDINAL) = "); ok {
				f.pid, _ = strconv.Atoi(strings.TrimSpace(v))
			}
		}
	}
	var names []string
	add := func(n string) {
		n = strings.TrimSpace(n)
		for _, x := range names {
			if strings.EqualFold(x, n) {
				return
			}
		}
		if n != "" {
			names = append(names, n)
		}
	}
	classes := strings.Split(f.class, "\x00")
	comm := ""
	if f.pid > 0 {
		if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", f.pid)); err == nil {
			comm = strings.TrimSpace(string(b))
		}
	}
	for _, c := range classes {
		if e, ok := entryFor(c, comm); ok {
			add(e.Name)
			break
		}
	}
	for _, c := range classes {
		add(c)
	}
	add(comm)
	return names
}

var browserClassRe = regexp.MustCompile(`(?i)firefox|navigator|chromium|chrome|brave|vivaldi|opera|librewolf|epiphany`)

func windowIsBrowser(id string) bool {
	if !have("xprop") {
		return true
	}
	class, err := Run(2*time.Second, "", "xprop", "-id", id, "WM_CLASS")
	return err == nil && browserClassRe.MatchString(class)
}

func (*linux) FocusEditor(string) bool {
	switch {
	case os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != "" && have("hyprctl"):
		out, err := Run(2*time.Second, "", "hyprctl", "dispatch", "focuswindow", "title:Typedeck")
		return err == nil && strings.Contains(out, "ok")
	case os.Getenv("SWAYSOCK") != "" && have("swaymsg"):
		out, err := Run(2*time.Second, "", "swaymsg", `[title="Typedeck" app_id="(?i)(firefox|chromium|chrome|brave|vivaldi|opera|librewolf|epiphany)"] focus`)
		return err == nil && strings.Contains(out, `"success": true`)
	case !wayland() && have("xdotool"):
		ids, err := Run(3*time.Second, "", "xdotool", "search", "--onlyvisible", "--name", "Typedeck")
		if err != nil {
			return false
		}
		for _, id := range strings.Fields(ids) {
			if windowIsBrowser(id) {
				_, err = Run(3*time.Second, "", "xdotool", "windowactivate", id)
				return err == nil
			}
		}
		return false
	case !wayland() && have("wmctrl"):
		_, err := Run(3*time.Second, "", "wmctrl", "-a", "Typedeck")
		return err == nil
	}
	return false
}
