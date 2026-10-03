package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"syscall"
	"time"

	"github.com/cristobaltormo/typedeck/internal/actions"
	"github.com/cristobaltormo/typedeck/internal/app"
	"github.com/cristobaltormo/typedeck/internal/board"
	"github.com/cristobaltormo/typedeck/internal/config"
	"github.com/cristobaltormo/typedeck/internal/engine"
	"github.com/cristobaltormo/typedeck/internal/platform"
	"github.com/cristobaltormo/typedeck/internal/server"
)

var version = "dev"

func main() {
	server.Version = version
	debug.SetGCPercent(25)
	debug.SetMemoryLimit(24 << 20)
	if len(os.Args) > 1 {
		platform.AttachConsole()
		switch os.Args[1] {
		case "version", "-v", "--version":
			fmt.Println("typedeck", version)
			return
		case "doctor":
			os.Exit(doctor())
		case "install", "uninstall":
			os.Exit(service(os.Args[1]))
		case "help", "-h", "--help":
			fmt.Println(`usage: typedeck [command]
  (no command)  start the program and the editor at http://127.0.0.1:7788
  doctor        check that everything is in place and say what fails
  install       start it with your session (user service)
  uninstall     remove the start with the session (the configuration is kept)
  version       print the version`)
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown command: %s (typedeck help)\n", os.Args[1])
			os.Exit(2)
		}
	}
	logToFile()
	log.SetFlags(log.Ltime)
	paths := config.DefaultPaths()
	brd := board.New(&board.SerialConnector{})
	brd.Log = log.Printf
	eng, err := engine.New(paths, brd)
	if err != nil {
		log.Fatalf("could not load the configuration: %v", err)
	}
	brd.OnEvent = eng.HandleBoardEvent
	brd.Mask = eng.MaskBytes
	brd.OnConnected = func(i *board.Info) {
		eng.OnBoardConnected(i.Firmware)
		eng.KeyboardChanged(i.Present)
	}
	brd.OnDisconnect = eng.OnBoardDisconnected

	srv, err := server.New(eng, brd, paths, log.Printf)
	if err != nil {
		log.Fatal(err)
	}
	ln, err := server.Listen()
	if err != nil {
		log.Fatalf("no free port from %d on (is another copy running?): %v", app.DefaultPort, err)
	}
	actions.EditorURL = server.EditorURL
	actions.EditorFocus = srv.FocusEditor
	_ = os.WriteFile(filepath.Join(paths.Dir, "port"), []byte(strconv.Itoa(server.Port)), 0o644)
	log.Printf("Typedeck %s, editor at %s", version, server.EditorURL())

	go func() {
		time.Sleep(8 * time.Second)
		debug.FreeOSMemory()
	}()
	stop := make(chan struct{})
	go eng.Run(stop)
	go brd.Run()
	go func() {
		hs := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second}
		if err := hs.Serve(ln); err != nil {
			log.Fatal(err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	close(stop)
	brd.Close()
	time.Sleep(200 * time.Millisecond)
}

func logToFile() {
	path := platform.Current.LogFile()
	if path == "" {
		return
	}
	if st, err := os.Stat(path); err == nil && st.Size() > 1<<20 {
		_ = os.Truncate(path, 0)
	}
	if platform.Current.Name() == "windows" {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
			log.SetOutput(f)
		}
	}
}
