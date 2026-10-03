package server

import (
	"bufio"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	wsMagic       = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	wsMaxClients  = 8
	wsMaxPayload  = 8 << 10
	wsReadTimeout = 70 * time.Second
	wsProtoPrefix = "typedeck."
)

type wsClient struct {
	conn    net.Conn
	out     chan []byte
	closed  chan struct{}
	once    sync.Once
	editing atomic.Bool
}

func (c *wsClient) close() {
	c.once.Do(func() {
		close(c.closed)
		_ = c.conn.Close()
	})
}

func (c *wsClient) send(msg []byte) {
	select {
	case c.out <- msg:
	default:
	}
}

type hub struct {
	mu      sync.Mutex
	clients map[*wsClient]struct{}
}

func (h *hub) add(c *wsClient) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients == nil {
		h.clients = map[*wsClient]struct{}{}
	}
	if len(h.clients) >= wsMaxClients {
		return false
	}
	h.clients[c] = struct{}{}
	return true
}

func (h *hub) remove(c *wsClient) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

func (h *hub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

func (h *hub) editing() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		if c.editing.Load() {
			return true
		}
	}
	return false
}

func (h *hub) broadcast(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.mu.Lock()
	for c := range h.clients {
		c.send(b)
	}
	h.mu.Unlock()
}

func (s *Server) originOK(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	return o == fmt.Sprintf("http://127.0.0.1:%d", Port) || o == fmt.Sprintf("http://localhost:%d", Port)
}

func (s *Server) wsToken(r *http.Request) bool {
	for _, p := range strings.Split(r.Header.Get("Sec-WebSocket-Protocol"), ",") {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, wsProtoPrefix) && subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(p, wsProtoPrefix)), []byte(s.token)) == 1 {
			return true
		}
	}
	return false
}

func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	if !s.hostOK(r) || !s.originOK(r) || !s.wsToken(r) {
		fail(w, 403, "token")
		return
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") || key == "" {
		fail(w, 400, "websocket")
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		fail(w, 500, "websocket")
		return
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return
	}
	sum := sha1.Sum([]byte(key + wsMagic))
	proto := wsProtoPrefix + s.token
	fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\nSec-WebSocket-Protocol: %s\r\n\r\n",
		base64.StdEncoding.EncodeToString(sum[:]), proto)
	if rw.Flush() != nil {
		_ = conn.Close()
		return
	}
	c := &wsClient{conn: conn, out: make(chan []byte, 64), closed: make(chan struct{})}
	if !s.hub.add(c) {
		_ = writeFrame(conn, 8, []byte{0x03, 0xf1})
		_ = conn.Close()
		return
	}
	go s.wsWrite(c)
	s.wsRead(c, rw.Reader)
	c.close()
	s.hub.remove(c)
	if c.editing.Load() {
		s.eng.SetEditing(s.hub.editing())
	}
}

func (s *Server) wsWrite(c *wsClient) {
	tick := time.NewTicker(25 * time.Second)
	defer tick.Stop()
	_, last := s.eng.EventsSince(0, 0, nil)
	hello, _ := json.Marshal(map[string]any{"t": "hello", "last_id": last})
	if writeFrame(c.conn, 1, hello) != nil {
		c.close()
		return
	}
	for {
		select {
		case m := <-c.out:
			_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if writeFrame(c.conn, 1, m) != nil {
				c.close()
				return
			}
		case <-tick.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if writeFrame(c.conn, 9, nil) != nil {
				c.close()
				return
			}
		case <-c.closed:
			return
		}
	}
}

func (s *Server) wsRead(c *wsClient, r *bufio.Reader) {
	for {
		_ = c.conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
		op, payload, err := readFrame(r)
		if err != nil {
			return
		}
		switch op {
		case 8:
			return
		case 1:
			var m struct {
				T  string `json:"t"`
				On bool   `json:"on"`
			}
			if json.Unmarshal(payload, &m) != nil {
				continue
			}
			if m.T == "editing" {
				c.editing.Store(m.On)
				s.eng.SetEditing(s.hub.editing())
			}
		}
	}
}

func writeFrame(w io.Writer, op byte, payload []byte) error {
	n := len(payload)
	hdr := []byte{0x80 | op}
	switch {
	case n < 126:
		hdr = append(hdr, byte(n))
	case n < 1<<16:
		hdr = append(hdr, 126, byte(n>>8), byte(n))
	default:
		hdr = append(hdr, 127)
		hdr = binary.BigEndian.AppendUint64(hdr, uint64(n))
	}
	_, err := w.Write(append(hdr, payload...))
	return err
}

func readFrame(r *bufio.Reader) (op byte, payload []byte, err error) {
	var h [2]byte
	if _, err = io.ReadFull(r, h[:]); err != nil {
		return
	}
	if h[0]&0x80 == 0 || h[0]&0x70 != 0 || h[1]&0x80 == 0 {
		return 0, nil, errors.New("trama no admitida")
	}
	op = h[0] & 0x0f
	n := uint64(h[1] & 0x7f)
	switch n {
	case 126:
		var b [2]byte
		if _, err = io.ReadFull(r, b[:]); err != nil {
			return
		}
		n = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err = io.ReadFull(r, b[:]); err != nil {
			return
		}
		n = binary.BigEndian.Uint64(b[:])
	}
	if n > wsMaxPayload {
		return 0, nil, errors.New("trama demasiado grande")
	}
	var mask [4]byte
	if _, err = io.ReadFull(r, mask[:]); err != nil {
		return
	}
	payload = make([]byte, n)
	if _, err = io.ReadFull(r, payload); err != nil {
		return
	}
	for i := range payload {
		payload[i] ^= mask[i%4]
	}
	return op, payload, nil
}

func (s *Server) pumpEvents() {
	_, last := s.eng.EventsSince(0, 0, nil)
	for {
		evs, l := s.eng.EventsSince(last, 25*time.Second, nil)
		last = l
		for _, ev := range evs {
			s.hub.broadcast(map[string]any{"t": "event", "ev": ev})
		}
	}
}
