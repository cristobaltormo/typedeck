package board

import (
	"io"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func pipePair(t *testing.T) (*overlappedFile, *overlappedFile) {
	t.Helper()
	name, _ := syscall.UTF16PtrFromString(`\\.\pipe\typedeck-test-` + time.Now().Format("150405.000000"))
	create := kernel32.NewProc("CreateNamedPipeW")
	srv, _, err := create.Call(uintptr(unsafe.Pointer(name)), 3|fileFlagOverlapped, 0, 1, 4096, 4096, 0, 0)
	if syscall.Handle(srv) == syscall.InvalidHandle {
		t.Fatal(err)
	}
	cli, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING, fileFlagOverlapped, 0)
	if err != nil {
		t.Fatal(err)
	}
	a, err := newOverlappedFile(syscall.Handle(srv))
	if err != nil {
		t.Fatal(err)
	}
	b, err := newOverlappedFile(cli)
	if err != nil {
		t.Fatal(err)
	}
	return a, b
}

func TestOverlappedEcho(t *testing.T) {
	a, b := pipePair(t)
	defer a.Close()
	defer b.Close()
	if _, err := a.Write([]byte("WHO\n")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	n, err := b.Read(buf)
	if err != nil || string(buf[:n]) != "WHO\n" {
		t.Fatalf("%q %v", buf[:n], err)
	}
}

func TestOverlappedWriteWhileReadPending(t *testing.T) {
	a, b := pipePair(t)
	defer a.Close()
	defer b.Close()
	go func() { _, _ = a.Read(make([]byte, 16)) }()
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	if _, err := a.Write([]byte("HB\n")); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("a write waited %v behind a pending read", d)
	}
}

func TestOverlappedCloseUnblocksRead(t *testing.T) {
	a, b := pipePair(t)
	defer b.Close()
	done := make(chan error, 1)
	go func() {
		_, err := a.Read(make([]byte, 16))
		done <- err
	}()
	time.Sleep(100 * time.Millisecond)
	a.Close()
	select {
	case err := <-done:
		if err != io.EOF {
			t.Fatalf("expected EOF, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not unblock the read")
	}
	if _, err := a.Write([]byte("x")); err == nil {
		t.Fatal("writing to a closed port must fail")
	}
}

func TestOverlappedConcurrentReadersDoNotShareTheBuffer(t *testing.T) {
	a, b := pipePair(t)
	defer a.Close()
	defer b.Close()
	got := make(chan string, 2)
	for i := 0; i < 2; i++ {
		go func() {
			buf := make([]byte, 16)
			n, err := b.Read(buf)
			if err != nil {
				got <- err.Error()
				return
			}
			got <- string(buf[:n])
		}()
	}
	time.Sleep(100 * time.Millisecond)
	_, _ = a.Write([]byte("AAAA"))
	time.Sleep(50 * time.Millisecond)
	_, _ = a.Write([]byte("BBBB"))
	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case s := <-got:
			seen[s] = true
		case <-time.After(2 * time.Second):
			t.Fatal("a reader never got its data")
		}
	}
	if !seen["AAAA"] || !seen["BBBB"] {
		t.Fatalf("readers saw %v", seen)
	}
}

func TestOverlappedWriteAfterCloseFails(t *testing.T) {
	a, b := pipePair(t)
	defer b.Close()
	a.Close()
	if _, err := a.Write([]byte("X")); err != errPortClosed {
		t.Fatalf("got %v", err)
	}
}
