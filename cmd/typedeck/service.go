package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/cristobaltormo/typedeck/internal/capture"
	"github.com/cristobaltormo/typedeck/internal/platform"
)

func captureNote(exe string) string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	helper := filepath.Join(filepath.Dir(exe), capture.HelperName)
	if _, err := os.Stat(helper); err != nil {
		return capture.HelperName + " is not next to typedeck, so using Typedeck without a board will not work; the macOS archive includes it"
	}
	return "Using Typedeck without a board runs " + helper + "; macOS asks you to allow it under Accessibility and Input Monitoring the first time you turn that on"
}

func service(what string) int {
	var msg string
	var err error
	if what == "install" {
		exe, e := os.Executable()
		if e == nil {
			exe, e = filepath.EvalSymlinks(exe)
		}
		if e != nil {
			fmt.Fprintln(os.Stderr, "could not locate the executable:", e)
			return 1
		}
		msg, err = platform.Current.InstallService(exe)
		if note := captureNote(exe); err == nil && note != "" {
			msg += "\n" + note
		}
	} else {
		msg, err = platform.Current.UninstallService()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	fmt.Println(msg)
	return 0
}
