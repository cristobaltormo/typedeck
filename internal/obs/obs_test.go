package obs

import (
	"bufio"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

type fakeOBS struct {
	ln       net.Listener
	password string
	calls    []string
	scene    string
	muted    bool
}

func startFake(t *testing.T, password string) *fakeOBS {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeOBS{ln: ln, password: password, scene: "Juego"}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(t, c)
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return f
}

func (f *fakeOBS) port() int { return f.ln.Addr().(*net.TCPAddr).Port }

func writeServer(c net.Conn, v any) {
	b, _ := json.Marshal(v)
	hdr := []byte{0x81}
	if len(b) < 126 {
		hdr = append(hdr, byte(len(b)))
	} else {
		hdr = append(hdr, 126, byte(len(b)>>8), byte(len(b)))
	}
	c.Write(append(hdr, b...))
}

func readClient(br *bufio.Reader) (map[string]any, error) {
	var h [2]byte
	if _, err := io.ReadFull(br, h[:]); err != nil {
		return nil, err
	}
	n := int(h[1] & 0x7F)
	if n == 126 {
		var b [2]byte
		io.ReadFull(br, b[:])
		n = int(binary.BigEndian.Uint16(b[:]))
	}
	var mask [4]byte
	io.ReadFull(br, mask[:])
	p := make([]byte, n)
	io.ReadFull(br, p)
	for i := range p {
		p[i] ^= mask[i%4]
	}
	var m map[string]any
	return m, json.Unmarshal(p, &m)
}

func (f *fakeOBS) serve(t *testing.T, c net.Conn) {
	defer c.Close()
	br := bufio.NewReader(c)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	sum := sha1.Sum([]byte(req.Header.Get("Sec-WebSocket-Key") + wsGUID))
	io.WriteString(c, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Protocol: obswebsocket.json\r\nSec-WebSocket-Accept: "+base64.StdEncoding.EncodeToString(sum[:])+"\r\n\r\n")
	hello := map[string]any{"rpcVersion": 1, "obsWebSocketVersion": "5.5.0"}
	const challenge, salt = "chal", "salt"
	if f.password != "" {
		hello["authentication"] = map[string]string{"challenge": challenge, "salt": salt}
	}
	writeServer(c, map[string]any{"op": 0, "d": hello})
	id, err := readClient(br)
	if err != nil {
		return
	}
	if f.password != "" {
		sec := sha256.Sum256([]byte(f.password + salt))
		want := sha256.Sum256([]byte(base64.StdEncoding.EncodeToString(sec[:]) + challenge))
		got, _ := id["d"].(map[string]any)["authentication"].(string)
		if got != base64.StdEncoding.EncodeToString(want[:]) {
			return
		}
	}
	writeServer(c, map[string]any{"op": 2, "d": map[string]any{"negotiatedRpcVersion": 1}})
	for {
		m, err := readClient(br)
		if err != nil {
			return
		}
		d := m["d"].(map[string]any)
		typ, _ := d["requestType"].(string)
		data, _ := d["requestData"].(map[string]any)
		f.calls = append(f.calls, typ)
		resp := map[string]any{}
		switch typ {
		case "GetSceneList":
			resp = map[string]any{"currentProgramSceneName": f.scene, "scenes": []any{map[string]any{"sceneName": "Final"}, map[string]any{"sceneName": "Charla"}, map[string]any{"sceneName": "Juego"}}}
		case "SetCurrentProgramScene":
			f.scene = data["sceneName"].(string)
		case "ToggleStream", "ToggleRecord":
			resp = map[string]any{"outputActive": true}
		case "ToggleInputMute":
			f.muted = !f.muted
			resp = map[string]any{"inputMuted": f.muted}
		case "GetSpecialInputs":
			resp = map[string]any{"mic1": "Mic/Aux", "desktop1": "Escritorio"}
		case "GetVersion":
			resp = map[string]any{"obsVersion": "32.2.2"}
		}
		writeServer(c, map[string]any{"op": 7, "d": map[string]any{"requestType": typ, "requestId": typ, "requestStatus": map[string]any{"result": true, "code": 100}, "responseData": resp}})
	}
}

func TestRunWithoutPassword(t *testing.T) {
	f := startFake(t, "")
	cfg := Config{Port: f.port()}
	if out, err := Run(cfg, "stream", ""); err != nil || out != "live" {
		t.Fatalf("stream: %q %v", out, err)
	}
	if out, err := Run(cfg, "scene", "Charla"); err != nil || out != "Charla" || f.scene != "Charla" {
		t.Fatalf("scene: %q %v %q", out, err, f.scene)
	}
	if out, err := Run(cfg, "mute", "Mic"); err != nil || !strings.Contains(out, "muted") {
		t.Fatalf("mute: %q %v", out, err)
	}
	if out, err := Run(cfg, "mute", "Mic"); err != nil || !strings.Contains(out, "unmuted") {
		t.Fatalf("unmute: %q %v", out, err)
	}
}

func TestSceneStepWraps(t *testing.T) {
	f := startFake(t, "")
	cfg := Config{Port: f.port()}
	for _, want := range []string{"Charla", "Final", "Juego"} {
		if out, err := Run(cfg, "scene_next", ""); err != nil || out != want {
			t.Fatalf("next: %q %v, wanted %q", out, err, want)
		}
	}
	if out, _ := Run(cfg, "scene_prev", ""); out != "Final" {
		t.Fatalf("prev: %q", out)
	}
}

func TestPassword(t *testing.T) {
	f := startFake(t, "secreto")
	if _, err := Run(Config{Port: f.port(), Password: "secreto"}, "record", ""); err != nil {
		t.Fatalf("with the right password: %v", err)
	}
	if _, err := Run(Config{Port: f.port(), Password: "mala"}, "record", ""); err == nil {
		t.Fatal("a wrong password must fail")
	}
	if _, err := Run(Config{Port: f.port()}, "record", ""); err == nil || !strings.Contains(err.Error(), "password") {
		t.Fatalf("without a password it must ask for it: %v", err)
	}
}

func TestProbeAndErrors(t *testing.T) {
	f := startFake(t, "")
	info, err := Probe(Config{Port: f.port()})
	if err != nil || info.Version != "32.2.2" || len(info.Scenes) != 3 || info.Scenes[0] != "Juego" {
		t.Fatalf("probe: %+v %v", info, err)
	}
	if _, err := Run(Config{Port: f.port()}, "scene", ""); err == nil {
		t.Fatal("scene without a name must fail")
	}
	if out, err := Run(Config{Port: f.port()}, "scene", "#2"); err != nil || out != "Charla" {
		t.Fatalf("scene by number: %q %v", out, err)
	}
	if _, err := Run(Config{Port: f.port()}, "scene", "#9"); err == nil {
		t.Fatal("a scene out of range must fail")
	}
	if out, err := Run(Config{Port: f.port()}, "mute", "@desktop"); err != nil || !strings.HasPrefix(out, "Escritorio") {
		t.Fatalf("mute @desktop: %q %v", out, err)
	}
	if out, err := Run(Config{Port: f.port()}, "mute", ""); err != nil || !strings.HasPrefix(out, "Mic/Aux") {
		t.Fatalf("default mute: %q %v", out, err)
	}
	if _, err := Run(Config{Port: 1}, "stream", ""); err == nil || !strings.Contains(err.Error(), "WebSocket") {
		t.Fatalf("without OBS it must explain how to turn it on: %v", err)
	}
}
