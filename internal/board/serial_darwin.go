package board

import (
	"syscall"
	"unsafe"
)

func setRaw115200(fd uintptr) error {
	var t syscall.Termios
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCGETA, uintptr(unsafe.Pointer(&t))); e != 0 {
		return e
	}
	t.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP | syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	t.Oflag &^= syscall.OPOST
	t.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	t.Cflag &^= syscall.CSIZE | syscall.PARENB
	t.Cflag |= syscall.CS8 | syscall.CREAD | syscall.CLOCAL
	t.Cc[syscall.VMIN] = 1
	t.Cc[syscall.VTIME] = 0
	t.Ispeed, t.Ospeed = 115200, 115200
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCSETA, uintptr(unsafe.Pointer(&t))); e != 0 {
		return e
	}
	return nil
}
