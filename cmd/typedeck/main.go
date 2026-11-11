package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/cristobaltormo/typedeck/internal/actions"
	"github.com/cristobaltormo/typedeck/internal/app"
	"github.com/cristobaltormo/typedeck/internal/board"
	"github.com/cristobaltormo/typedeck/internal/capture"
	"github.com/cristobaltormo/typedeck/internal/config"
	"github.com/cristobaltormo/typedeck/internal/engine"
	"github.com/cristobaltormo/typedeck/internal/platform"
	"github.com/cristobaltormo/typedeck/internal/server"
)

var version = "dev"

const helpText = `usage: typedeck [command]
  (no command)  start the program and the editor at http://127.0.0.1:7788
  doctor        check that everything is in place and say what fails
  install       start it with your session (user service)
  uninstall     remove the start with the session (the configuration is kept)
  version       print the version`

func main() {
	server.Version = version
	debug.SetGCPercent(25)
	debug.SetMemoryLimit(24 << 20)
	if len(os.Args) > 1 && !backgroundStart() {
		platform.AttachConsole()
		os.Exit(runCommand(os.Args[1]))
	}
	runProgram()
}

func runCommand(name string) int {
	switch name {
	case "version", "-v", "--version":
		fmt.Println("typedeck", version)
	case "doctor":
		return doctor()
	case "install", "uninstall":
		return service(name)
	case "help", "-h", "--help":
		fmt.Println(helpText)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s (typedeck help)\n", name)
		return 2
	}
	return 0
}

func serveEditor(srv *server.Server, ln net.Listener) {
	hs := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second}
	if err := hs.Serve(ln); err != nil {
		fatalf("the editor stopped: %v", err)
	}
}

func runProgram() {
	claimInstance()
	logToFile()
	log.SetFlags(log.Ltime)
	paths := config.DefaultPaths()
	brd := board.New(&board.SerialConnector{})
	brd.Log = log.Printf
	src := capture.NewSelector(brd, capture.NewSoftware())
	eng, err := engine.New(paths, src)
	if err != nil {
		fatalf("could not load the configuration: %v", err)
	}
	srv, err := server.New(eng, src, paths, log.Printf)
	if err != nil {
		fatalf("could not start the editor: %v", err)
	}
	ln, err := server.Listen()
	if err != nil {
		fatalf("no free port from %d on: %v", app.DefaultPort, err)
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
	go src.Run()
	go serveEditor(srv, ln)

	var once sync.Once
	shutdown := func() {
		once.Do(func() {
			log.Print("shutting down")
			close(stop)
			src.Close()
			time.Sleep(200 * time.Millisecond)
			log.Print("stopped")
		})
	}
	startShell(src, shutdown)
	if manualStart() {
		go func() { _ = platform.Current.OpenURL(server.EditorURL()) }()
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	shutdown()
}

func logToFile() {
	path := platform.Current.LogFile()
	if path == "" {
		return
	}
	if st, err := os.Stat(path); err == nil && st.Size() > 1<<20 {
		rotateLog(path)
	}
	if platform.Current.Name() == "windows" {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
			log.SetOutput(f)
		}
	}
}
