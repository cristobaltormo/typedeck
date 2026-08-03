package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cristobaltormo/typedeck/internal/platform"
)

func service(what string) int {
	var msg string
	var err error
	if what == "install" {
		exe, e := os.Executable()
		if e == nil {
			exe, e = filepath.EvalSymlinks(exe)
		}
		if e != nil {
			fmt.Fprintln(os.Stderr, "no se pudo localizar el ejecutable:", e)
			return 1
		}
		msg, err = platform.Current.InstallService(exe)
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
