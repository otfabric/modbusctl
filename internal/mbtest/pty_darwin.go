//go:build darwin

// SPDX-License-Identifier: MIT

package mbtest

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// ptySlavePath grants and unlocks the slave of a /dev/ptmx master and returns
// its path. The slave of master minor n is /dev/ttys<n> (three digits at
// least), which is what TIOCPTYGNAME reports; reading the minor from fstat
// avoids passing a buffer to ioctl.
func ptySlavePath(master *os.File) (string, error) {
	var st unix.Stat_t
	err := control(master, func(fd int) error {
		if err := unix.IoctlSetInt(fd, unix.TIOCPTYGRANT, 0); err != nil {
			return err
		}
		if err := unix.IoctlSetInt(fd, unix.TIOCPTYUNLK, 0); err != nil {
			return err
		}
		return unix.Fstat(fd, &st)
	})
	if err != nil {
		return "", err
	}
	if st.Rdev < 0 {
		return "", fmt.Errorf("pseudo-terminal master has device number %d", st.Rdev)
	}
	return fmt.Sprintf("/dev/ttys%03d", unix.Minor(uint64(st.Rdev))), nil
}
