//go:build darwin

package mbtest

import (
	"bytes"
	"os"
	"syscall"
	"unsafe"
)

func ptySlavePath(master *os.File) (string, error) {
	if err := ioctl(master.Fd(), syscall.TIOCPTYGRANT, 0); err != nil {
		return "", err
	}
	if err := ioctl(master.Fd(), syscall.TIOCPTYUNLK, 0); err != nil {
		return "", err
	}
	var name [256]byte
	if err := ioctl(master.Fd(), syscall.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0]))); err != nil {
		return "", err
	}
	if i := bytes.IndexByte(name[:], 0); i >= 0 {
		return string(name[:i]), nil
	}
	return string(name[:]), nil
}

func ioctl(fd uintptr, req uint, arg uintptr) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(req), arg); errno != 0 {
		return errno
	}
	return nil
}
