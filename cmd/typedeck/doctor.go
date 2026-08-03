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
			fmt.Println("  FALLA   ", badMsg)
			bad++
		}
	}
	fmt.Println("Typedeck", version)
	paths := config.DefaultPaths()
	_, err := config.Load(paths)
	check(err == nil, "configuración válida en "+paths.Config(), fmt.Sprint("configuración: ", err))
	if platform.Current.Name() == "darwin" {
		_, err = os.Stat(hud.Binary)
		check(err == nil, "cartel flotante instalado", "falta el cartel flotante ("+hud.Binary+"); se usarán notificaciones")
	}
	for _, c := range platform.Current.Doctor() {
		check(c.OK, c.Message, c.Problem)
	}
	if running, connected, fw, kb := askRunningService(); running {
		check(true, "Typedeck ya está en marcha (editor en http://127.0.0.1:7788)", "")
		check(connected, "placa conectada, firmware "+fw, "el servicio no ve la placa (cable o puerto)")
		check(kb != "", "teclado detectado: "+kb, "la placa no ve ningún teclado en el shield")
	} else {
		rw, port, err := (&board.SerialConnector{}).Open()
		if err == nil {
			rw.Close()
		}
		check(err == nil, "placa encontrada en "+port, "no se encuentra la placa (cable, puerto o programa en marcha usando el puerto)")
	}
	time.Sleep(100 * time.Millisecond)
	if bad > 0 {
		fmt.Printf("%d comprobaciones fallan\n", bad)
		return 1
	}
	fmt.Println("todo en orden")
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
