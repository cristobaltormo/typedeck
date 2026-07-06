//go:build darwin || linux

package board

import (
	"io"
	"os"
	"syscall"
)

func openPort(path string) (io.ReadWriteCloser, error) {
	f, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, err
	}
	if err := setRaw115200(f.Fd()); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
