package board

import (
	"bufio"
	"io"
	"strings"
	"sync"
	"time"
)

type SerialConnector struct {
	mu       sync.Mutex
	rejected map[string]time.Time
}

func candidates() []string { return systemPorts() }

func (c *SerialConnector) Open() (io.ReadWriteCloser, string, error) {
	outdated := ""
	c.mu.Lock()
	if c.rejected == nil {
		c.rejected = map[string]time.Time{}
	}
	c.mu.Unlock()
	for _, path := range candidates() {
		c.mu.Lock()
		at, bad := c.rejected[path]
		c.mu.Unlock()
		if bad && time.Since(at) < 30*time.Second {
			continue
		}
		f, err := openPort(path)
		if err != nil {
			continue
		}
		switch probe(f) {
		case probeOK:
			return f, path, nil
		case probeOld:
			outdated = path
		}
		f.Close()
		c.mu.Lock()
		c.rejected[path] = time.Now()
		c.mu.Unlock()
	}
	if outdated != "" {
		return nil, outdated, ErrOutdatedFirmware
	}
	return nil, "", ErrNoBoard
}

const (
	probeNo = iota
	probeOK
	probeOld
)

func probe(f io.ReadWriteCloser) int {
	time.Sleep(300 * time.Millisecond)
	if _, err := io.WriteString(f, "WHO\n"); err != nil {
		return probeNo
	}
	res := make(chan int, 1)
	go func() {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			l := strings.TrimSpace(sc.Text())
			switch {
			case strings.HasPrefix(l, "TYPEDECK-FW"):
				res <- probeOK
				return
			case strings.HasPrefix(l, "NUMDECK-FW"), strings.HasPrefix(l, "LEONARDO-MACROS"):
				res <- probeOld
				return
			}
		}
		res <- probeNo
	}()
	select {
	case v := <-res:
		return v
	case <-time.After(900 * time.Millisecond):
		return probeNo
	}
}
