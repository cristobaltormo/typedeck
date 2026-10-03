package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cristobaltormo/typedeck/internal/board"
	"github.com/cristobaltormo/typedeck/internal/config"
	"github.com/cristobaltormo/typedeck/internal/hud"
	"github.com/cristobaltormo/typedeck/internal/platform"
)

func doctor() int {
	bad := 0
	check := func(ok bool, okMsg, badMsg string) {
		if ok {
			fmt.Println("  ok      ", okMsg)
		} else {
			fmt.Println("  FAIL    ", badMsg)
			bad++
		}
	}
	fmt.Println("Typedeck", version)
	paths := config.DefaultPaths()
	_, err := config.Load(paths)
	check(err == nil, "valid configuration in "+paths.Config(), fmt.Sprint("configuration: ", err))
	if platform.Current.Name() == "darwin" {
		_, err = os.Stat(hud.Binary)
		check(err == nil, "floating popup installed", "the floating popup is missing ("+hud.Binary+"); notifications will be used")
	}
	for _, c := range platform.Current.Doctor() {
		check(c.OK, c.Message, c.Problem)
	}
	if running, connected, fw, kb := askRunningService(); running {
		check(true, "Typedeck is already running (editor at http://127.0.0.1:7788)", "")
		check(connected, "board connected, firmware "+fw, "the service cannot see the board (cable or port)")
		check(kb != "", "keyboard detected: "+kb, "the board sees no keyboard on the shield")
	} else {
		rw, port, err := (&board.SerialConnector{}).Open()
		if err == nil {
			rw.Close()
		}
		check(err == nil, "board found at "+port, "board not found (cable, port, or a running program using the port)")
	}
	time.Sleep(100 * time.Millisecond)
	if bad > 0 {
		fmt.Printf("%d checks failed\n", bad)
		return 1
	}
	fmt.Println("all good")
	return 0
}

func askRunningService() (running, connected bool, firmware, keyboard string) {
	client := &http.Client{Timeout: 2 * time.Second}
	req, _ := http.NewRequest("GET", editorURL(), nil)
	resp, err := client.Do(req)
	if err != nil {
		return false, false, "", ""
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	m := regexp.MustCompile(`name="token" content="([^"]+)"`).FindSubmatch(page)
	if m == nil {
		return true, false, "", ""
	}
	get := func(path string, v any) bool {
		r, _ := http.NewRequest("GET", strings.TrimSuffix(editorURL(), "/")+path, nil)
		r.Header.Set("X-Token", string(m[1]))
		rs, err := client.Do(r)
		if err != nil {
			return false
		}
		defer rs.Body.Close()
		return json.NewDecoder(rs.Body).Decode(v) == nil
	}
	var kb struct {
		Connected bool `json:"connected"`
		Info      *struct {
			Firmware, Mfr, Prod, VID, PID string
			Present                       bool
		} `json:"info"`
	}
	if !get("/api/keyboard", &kb) {
		return true, false, "", ""
	}
	if kb.Info != nil {
		firmware = kb.Info.Firmware
		if kb.Info.Present {
			keyboard = strings.TrimSpace(kb.Info.Mfr+" "+kb.Info.Prod) + " (" + kb.Info.VID + ":" + kb.Info.PID + ")"
		}
	}
	return true, kb.Connected, firmware, keyboard
}

func editorURL() string {
	port := "7788"
	if b, err := os.ReadFile(filepath.Join(config.DefaultPaths().Dir, "port")); err == nil {
		if p := strings.TrimSpace(string(b)); p != "" {
			port = p
		}
	}
	return "http://127.0.0.1:" + port + "/"
}
