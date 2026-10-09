//go:build linux

package mbtest

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

func ptySlavePath(master *os.File) (string, error) {
	var unlock int32
	if err := ioctl(master.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); err != nil {
		return "", err
	}
	var n uint32
	if err := ioctl(master.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n))); err != nil {
		return "", err
	}
	return fmt.Sprintf("/dev/pts/%d", n), nil
}

func ioctl(fd uintptr, req uint, arg uintptr) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(req), arg); errno != 0 {
		return errno
	}
	return nil
}
