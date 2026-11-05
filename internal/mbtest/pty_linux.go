//go:build linux

// SPDX-License-Identifier: MIT

package mbtest

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func ptySlavePath(master *os.File) (string, error) {
	var n uint32
	err := control(master, func(fd int) error {
		if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
			return err
		}
		var err error
		n, err = unix.IoctlGetUint32(fd, unix.TIOCGPTN)
		return err
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("/dev/pts/%d", n), nil
}
