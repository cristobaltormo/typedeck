//go:build linux

package platform

import (
	"os"
	"strings"
	"time"

	"github.com/cristobaltormo/typedeck/internal/hid"
)

func xkbToHID(layout, variant string) string {
	layout, variant = strings.ToLower(strings.TrimSpace(layout)), strings.ToLower(strings.TrimSpace(variant))
	for _, v := range []string{"dvorak", "colemak", "workman", "norman", "dvp", "neo"} {
		if strings.HasPrefix(variant, v) {
			return ""
		}
	}
	switch {
	case layout == "us" && variant == "":
		return "us"
	case layout == "es" && (variant == "" || variant == "winkeys" || variant == "nodeadkeys"):
		return "es-pc"
	case layout == "us" || layout == "gb" || layout == "es" || layout == "pt" || layout == "br" || layout == "it" || layout == "latam" || layout == "ie" || layout == "ca":
		return "qwerty"
	}
	return ""
}

func (*linux) SystemLayout() string {
	if have("gsettings") {
		if out, err := Run(2*time.Second, "", "gsettings", "get", "org.gnome.desktop.input-sources", "mru-sources"); err == nil {
			if i := strings.Index(out, "'xkb', '"); i >= 0 {
				v := out[i+len("'xkb', '"):]
				if j := strings.Index(v, "'"); j > 0 {
					l, variant, _ := strings.Cut(v[:j], "+")
					return xkbToHID(l, variant)
				}
			}
		}
	}
	if have("setxkbmap") && os.Getenv("DISPLAY") != "" {
		out, _ := Run(2*time.Second, "", "setxkbmap", "-query")
		var l, v string
		for _, line := range strings.Split(out, "\n") {
			if k, val, ok := strings.Cut(line, ":"); ok {
				switch strings.TrimSpace(k) {
				case "layout":
					l, _, _ = strings.Cut(strings.TrimSpace(val), ",")
				case "variant":
					v, _, _ = strings.Cut(strings.TrimSpace(val), ",")
				}
			}
		}
		if l != "" {
			return xkbToHID(l, v)
		}
	}
	if have("localectl") {
		out, _ := Run(2*time.Second, "", "localectl", "status")
		var l, v string
		for _, line := range strings.Split(out, "\n") {
			if k, val, ok := strings.Cut(line, ":"); ok {
				switch strings.TrimSpace(k) {
				case "X11 Layout":
					l, _, _ = strings.Cut(strings.TrimSpace(val), ",")
				case "X11 Variant":
					v, _, _ = strings.Cut(strings.TrimSpace(val), ",")
				}
			}
		}
		if l != "" {
			return xkbToHID(l, v)
		}
	}
	return ""
}

func (*linux) AdaptStroke(st hid.Stroke, _ *hid.Layout) hid.Stroke { return st }
