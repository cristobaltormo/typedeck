package capture

import (
	"bufio"
	"io"
	"os/exec"
	"sync"
	"time"
)

type helperProc struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	wmu   sync.Mutex
	done  chan struct{}
}

func startHelper(path string, args []string, onLine func(string), onExit func()) (*helperProc, error) {
	cmd := exec.Command(path, args...)
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	h := &helperProc{cmd: cmd, stdin: in, done: make(chan struct{})}
	go func() {
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			onLine(sc.Text())
		}
		_ = cmd.Wait()
		onExit()
		close(h.done)
	}()
	return h, nil
}

func (h *helperProc) send(lines ...string) error {
	h.wmu.Lock()
	defer h.wmu.Unlock()
	for _, l := range lines {
		if _, err := io.WriteString(h.stdin, l+"\n"); err != nil {
			return err
		}
	}
	return nil
}

func (h *helperProc) alive() bool {
	select {
	case <-h.done:
		return false
	default:
		return true
	}
}

func (h *helperProc) stop() {
	_ = h.send("QUIT")
	_ = h.stdin.Close()
	select {
	case <-h.done:
	case <-time.After(2 * time.Second):
		_ = h.cmd.Process.Kill()
		<-h.done
	}
}
