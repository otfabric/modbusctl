// SPDX-License-Identifier: MIT

package mbtest

import (
	"io"
	"os"
	"syscall"
	"testing"
)

// ServeRTUSerial serves the device with RTU framing on a pseudo-terminal and returns the
// path of the serial device to open (e.g. /dev/ttys004). The test is skipped where no
// pseudo-terminal can be created.
func (d *Device) ServeRTUSerial(t testing.TB) string {
	t.Helper()
	return d.servePTY(t, d.serveRTU)
}

// ServeASCIISerial serves the device with ASCII framing on a pseudo-terminal and returns
// the path of the serial device to open.
func (d *Device) ServeASCIISerial(t testing.TB) string {
	t.Helper()
	return d.servePTY(t, d.serveASCII)
}

func (d *Device) servePTY(t testing.TB, serve func(io.ReadWriter)) string {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("mbtest: no pseudo-terminal available: %v", err)
	}
	path, err := ptySlavePath(master)
	if err != nil {
		_ = master.Close()
		t.Skipf("mbtest: no pseudo-terminal available: %v", err)
	}
	// Hold the slave side open so that the master does not see a hang-up between two
	// clients opening and closing the device.
	slave, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		_ = master.Close()
		t.Skipf("mbtest: cannot open pseudo-terminal %s: %v", path, err)
	}
	t.Cleanup(func() {
		_ = slave.Close()
		_ = master.Close()
	})
	go serve(master)
	return path
}
