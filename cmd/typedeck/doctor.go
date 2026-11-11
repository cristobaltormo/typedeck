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
	"github.com/cristobaltormo/typedeck/internal/capture"
	"github.com/cristobaltormo/typedeck/internal/config"
	"github.com/cristobaltormo/typedeck/internal/hud"
	"github.com/cristobaltormo/typedeck/internal/platform"
)

func doctor() int {
	bad := 0
	check := func(ok bool, okMsg, badMsg string) {
		if ok {
			fmt.Println("  ok      ", okMsg)
		} else if note, found := strings.CutPrefix(badMsg, "optional: "); found {
			fmt.Println("  note    ", note)
		} else {
			fmt.Println("  FAIL    ", badMsg)
			bad++
		}
	}
	fmt.Println("Typedeck", version)
	paths := config.DefaultPaths()
	cfg, err := config.Load(paths)
	check(err == nil, "valid configuration in "+paths.Config(), fmt.Sprint("configuration: ", err))
	if platform.Current.Name() == "darwin" {
		_, err = os.Stat(hud.Binary)
		check(err == nil, "floating popup installed", "the floating popup is missing ("+hud.Binary+"); notifications will be used")
	}
	for _, c := range platform.Current.Doctor() {
		check(c.OK, c.Message, c.Problem)
	}
	if cfg.Settings.Capture == capture.Software {
		softwareChecks(check)
	} else {
		boardChecks(check)
	}
	time.Sleep(100 * time.Millisecond)
	if bad > 0 {
		fmt.Printf("%d checks failed\n", bad)
		return 1
	}
	fmt.Println("all good")
	return 0
}

type checkFn func(ok bool, okMsg, badMsg string)

func boardChecks(check checkFn) {
	if running, connected, fw, kb, _ := askRunningService(); running {
		check(true, "Typedeck is already running (editor at "+editorURL()+")", "")
		check(connected, "board connected, firmware "+fw, "the service cannot see the board (cable or port)")
		check(kb != "", "keyboard detected: "+kb, "the board sees no keyboard on the shield")
	} else {
		rw, port, err := (&board.SerialConnector{}).Open()
		if err == nil {
			rw.Close()
		}
		check(err == nil, "board found at "+port, "board not found (cable, port, or a running program using the port)")
	}
}

func softwareChecks(check checkFn) {
	for _, c := range capture.NewSoftware().Checks() {
		check(c.OK, c.Message, c.Problem)
	}
	running, connected, _, _, mode := askRunningService()
	if !running {
		return
	}
	check(true, "Typedeck is already running (editor at "+editorURL()+")", "")
	if mode != capture.Software {
		check(false, "", "the running program still uses the board; restart it to apply the software mode")
		return
	}
	check(connected, "software capture is active in the running program", "the running program has no keyboard hook")
	for _, w := range runningWarnings() {
		check(false, "", w)
	}
}

func serviceAPI() (get func(path string, v any) bool, running bool) {
	client := &http.Client{Timeout: 2 * time.Second}
	req, _ := http.NewRequest("GET", editorURL(), nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, false
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	m := regexp.MustCompile(`name="token" content="([^"]+)"`).FindSubmatch(page)
	if m == nil {
		return nil, true
	}
	return func(path string, v any) bool {
		r, _ := http.NewRequest("GET", strings.TrimSuffix(editorURL(), "/")+path, nil)
		r.Header.Set("X-Token", string(m[1]))
		rs, err := client.Do(r)
		if err != nil {
			return false
		}
		defer rs.Body.Close()
		return json.NewDecoder(rs.Body).Decode(v) == nil
	}, true
}

func askRunningService() (running, connected bool, firmware, keyboard, mode string) {
	get, running := serviceAPI()
	if get == nil {
		return running, false, "", "", ""
	}
	var kb struct {
		Connected bool `json:"connected"`
		Info      *struct {
			Firmware, Mfr, Prod, VID, PID string
			Present                       bool
		} `json:"info"`
	}
	if !get("/api/keyboard", &kb) {
		return true, false, "", "", ""
	}
	var st struct {
		Capture string `json:"capture"`
	}
	get("/api/status", &st)
	if kb.Info != nil {
		firmware = kb.Info.Firmware
		if kb.Info.Present {
			keyboard = strings.TrimSpace(kb.Info.Mfr+" "+kb.Info.Prod) + " (" + kb.Info.VID + ":" + kb.Info.PID + ")"
		}
	}
	return true, kb.Connected, firmware, keyboard, st.Capture
}

func runningWarnings() []string {
	get, _ := serviceAPI()
	if get == nil {
		return nil
	}
	var su struct {
		Checks []struct{ Level, Code string } `json:"checks"`
	}
	if !get("/api/setup", &su) {
		return nil
	}
	var out []string
	for _, c := range su.Checks {
		if c.Level == "warn" && c.Code == "grab_failed" {
			out = append(out, "the running program could not take one of the keyboards (another program holds it): keys typed there are not captured")
		}
	}
	return out
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
