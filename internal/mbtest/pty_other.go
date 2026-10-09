//go:build !darwin && !linux

package mbtest

import (
	"errors"
	"os"
)

func ptySlavePath(*os.File) (string, error) {
	return "", errors.New("pseudo-terminals are not supported on this platform")
}
