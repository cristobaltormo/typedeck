package server

import (
	"bufio"
	"net"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cristobaltormo/typedeck/internal/board"
	"github.com/cristobaltormo/typedeck/internal/config"
)

func dialWS(t *testing.T, addr, token string) (net.Conn, *bufio.Reader, string) {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	req := "GET /api/ws HTTP/1.1\r\nHost: " + addr + "\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Protocol: typedeck." + token + "\r\n\r\n"
	_, _ = c.Write([]byte(req))
	r := bufio.NewReader(c)
	status, _ := r.ReadString('\n')
	for {
		l, err := r.ReadString('\n')
		if err != nil || l == "\r\n" {
			break
		}
	}
	return c, r, status
}

func sendText(c net.Conn, s string) {
	p := []byte(s)
	mask := []byte{1, 2, 3, 4}
	f := []byte{0x81, 0x80 | byte(len(p))}
	f = append(f, mask...)
	for i, b := range p {
		f = append(f, b^mask[i%4])
	}
	_, _ = c.Write(f)
}

func TestWebSocketHelloAndEditingPause(t *testing.T) {
	s, h := newSrv(t)
	ts := httptest.NewServer(h)
	defer ts.Close()
	addr := strings.TrimPrefix(ts.URL, "http://")
	old := Port
	Port, _ = strconv.Atoi(addr[strings.LastIndex(addr, ":")+1:])
	defer func() { Port = old }()

	cfg := config.Default()
	cfg.Global["04"] = config.KeyDef{Tap: &config.Action{Type: "hud", Text: "x"}}
	if _, err := s.eng.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if m := s.eng.MaskBytes(); m[0]&0x10 == 0 {
		t.Fatal("key A must be captured")
	}

	if _, _, st := dialWS(t, addr, "mal"); !strings.Contains(st, "403") {
		t.Fatalf("a bad token was accepted: %s", st)
	}

	c, r, st := dialWS(t, addr, s.token)
	defer c.Close()
	if !strings.Contains(st, "101") {
		t.Fatalf("no upgrade: %s", st)
	}
	var hdr [2]byte
	_, _ = r.Read(hdr[:])
	body := make([]byte, hdr[1])
	_, _ = r.Read(body)
	if !strings.Contains(string(body), `"t":"hello"`) {
		t.Fatalf("no hello: %s", body)
	}

	sendText(c, `{"t":"editing","on":true}`)
	waitMask(t, s, true)
	c.Close()
	waitMask(t, s, false)
}

func waitMask(t *testing.T, s *Server, zero bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		m := s.eng.MaskBytes()
		if (m[0]&0x10 == 0) == zero {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the mask did not reach the expected state (zero=%v)", zero)
}

func TestPlainKeysAreBroadcastToTheEditor(t *testing.T) {
	s, h := newSrv(t)
	ts := httptest.NewServer(h)
	defer ts.Close()
	addr := strings.TrimPrefix(ts.URL, "http://")
	old := Port
	Port, _ = strconv.Atoi(addr[strings.LastIndex(addr, ":")+1:])
	defer func() { Port = old }()

	c, r, _ := dialWS(t, addr, s.token)
	defer c.Close()
	var hdr [2]byte
	_, _ = r.Read(hdr[:])
	_, _ = r.Read(make([]byte, hdr[1]))

	s.eng.HandleBoardEvent(board.Event{Kind: 'P', Usage: 0x1E})
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _ = r.Read(hdr[:])
	body := make([]byte, hdr[1])
	_, _ = r.Read(body)
	if !strings.Contains(string(body), `"t":"key"`) || !strings.Contains(string(body), `"k":"1E"`) || !strings.Contains(string(body), `"d":true`) {
		t.Fatalf("key message: %s", body)
	}
}
