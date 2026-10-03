package obs

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

type wsConn struct {
	c  net.Conn
	br *bufio.Reader
}

func wsDial(addr, subprotocol string, timeout time.Duration) (*wsConn, error) {
	c, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, err
	}
	_ = c.SetDeadline(time.Now().Add(timeout))
	keyRaw := make([]byte, 16)
	_, _ = rand.Read(keyRaw)
	key := base64.StdEncoding.EncodeToString(keyRaw)
	req := fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Protocol: %s\r\n\r\n", addr, key, subprotocol)
	if _, err := io.WriteString(c, req); err != nil {
		c.Close()
		return nil, err
	}
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		c.Close()
		return nil, err
	}
	resp.Body.Close()
	sum := sha1.Sum([]byte(key + wsGUID))
	if resp.StatusCode != http.StatusSwitchingProtocols || resp.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(sum[:]) {
		c.Close()
		return nil, fmt.Errorf("the server did not accept the WebSocket (HTTP %d)", resp.StatusCode)
	}
	return &wsConn{c: c, br: br}, nil
}

func (w *wsConn) Close() error { return w.c.Close() }

func (w *wsConn) deadline(d time.Duration) { _ = w.c.SetDeadline(time.Now().Add(d)) }

func (w *wsConn) writeFrame(opcode byte, payload []byte) error {
	hdr := []byte{0x80 | opcode}
	switch n := len(payload); {
	case n < 126:
		hdr = append(hdr, 0x80|byte(n))
	case n < 1<<16:
		hdr = append(hdr, 0x80|126, byte(n>>8), byte(n))
	default:
		hdr = append(hdr, 0x80|127)
		hdr = binary.BigEndian.AppendUint64(hdr, uint64(n))
	}
	mask := make([]byte, 4)
	_, _ = rand.Read(mask)
	hdr = append(hdr, mask...)
	body := make([]byte, len(payload))
	for i, b := range payload {
		body[i] = b ^ mask[i%4]
	}
	_, err := w.c.Write(append(hdr, body...))
	return err
}

func (w *wsConn) WriteText(s []byte) error { return w.writeFrame(0x1, s) }

func (w *wsConn) ReadText() ([]byte, error) {
	var msg []byte
	for {
		var h [2]byte
		if _, err := io.ReadFull(w.br, h[:]); err != nil {
			return nil, err
		}
		fin, op, masked := h[0]&0x80 != 0, h[0]&0x0F, h[1]&0x80 != 0
		n := uint64(h[1] & 0x7F)
		switch n {
		case 126:
			var b [2]byte
			if _, err := io.ReadFull(w.br, b[:]); err != nil {
				return nil, err
			}
			n = uint64(binary.BigEndian.Uint16(b[:]))
		case 127:
			var b [8]byte
			if _, err := io.ReadFull(w.br, b[:]); err != nil {
				return nil, err
			}
			n = binary.BigEndian.Uint64(b[:])
		}
		if n > 1<<20 {
			return nil, errors.New("message too large")
		}
		var mask [4]byte
		if masked {
			if _, err := io.ReadFull(w.br, mask[:]); err != nil {
				return nil, err
			}
		}
		p := make([]byte, n)
		if _, err := io.ReadFull(w.br, p); err != nil {
			return nil, err
		}
		if masked {
			for i := range p {
				p[i] ^= mask[i%4]
			}
		}
		switch op {
		case 0x8:
			return nil, io.EOF
		case 0x9:
			_ = w.writeFrame(0xA, p)
		case 0xA:
		case 0x1, 0x0:
			msg = append(msg, p...)
			if fin {
				return msg, nil
			}
		default:
			if !fin || op != 0x2 {
				return nil, fmt.Errorf("unexpected WebSocket frame (%d)", op)
			}
		}
	}
}

func hostPort(host string, port int) string {
	if host == "" {
		host = "127.0.0.1"
	}
	if port == 0 {
		port = 4455
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("%s:%d", host, port)
}
