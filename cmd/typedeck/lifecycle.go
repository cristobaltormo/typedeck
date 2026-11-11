package main

import (
	"fmt"
	"log"
	"os"

	"github.com/cristobaltormo/typedeck/internal/capture"
	"github.com/cristobaltormo/typedeck/internal/platform"
	"github.com/cristobaltormo/typedeck/internal/server"
)

func backgroundStart() bool { return len(os.Args) > 1 && os.Args[1] == "--background" }

func manualStart() bool { return !backgroundStart() && platform.StartedByUser() }

func claimInstance() {
	if platform.AcquireInstance() {
		return
	}
	platform.AttachConsole()
	url := editorURL()
	fmt.Fprintln(os.Stderr, "Typedeck is already running, its editor is at "+url)
	if manualStart() {
		_ = platform.Current.OpenURL(url)
	}
	os.Exit(0)
}

func fatalf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Print(msg)
	platform.ShowError("Typedeck", msg)
	os.Exit(1)
}

func rotateLog(path string) {
	if platform.Current.Name() != "windows" {
		_ = os.Truncate(path, 0)
		return
	}
	_ = os.Remove(path + ".1")
	_ = os.Rename(path, path+".1")
}

func captureStatus(src capture.Source) string {
	connected, port, _, _ := src.Status()
	switch {
	case src.Kind() == capture.Software && connected:
		return "software capture active"
	case src.Kind() == capture.Software:
		return "software capture not running"
	case !connected:
		return "board not connected"
	}
	return "board connected on " + port
}

func startShell(src capture.Source, shutdown func()) {
	platform.StartShell(platform.ShellHooks{
		Status:     func() string { return captureStatus(src) },
		OpenEditor: func() { _ = platform.Current.OpenURL(server.EditorURL()) },
		Shutdown:   shutdown,
		Release:    func() { log.Print("session locked or suspended, releasing the keys"); src.Pause() },
		Reclaim:    func() { log.Print("session back, capturing again"); src.Resume() },
	})
}
