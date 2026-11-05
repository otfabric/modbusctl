//go:build darwin || linux

// SPDX-License-Identifier: MIT

package mbtest

import "os"

// control runs f with the file descriptor of file.
func control(file *os.File, f func(fd int) error) error {
	rc, err := file.SyscallConn()
	if err != nil {
		return err
	}
	var ferr error
	if err := rc.Control(func(fd uintptr) { ferr = f(int(fd)) }); err != nil {
		return err
	}
	return ferr
}
