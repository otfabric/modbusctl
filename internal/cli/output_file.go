package cli

import (
	"io"
	"os"
	"strings"
)

// OpenStdoutOrFile returns os.Stdout when path is empty/whitespace; otherwise creates path for write.
// cleanup must be called when non-nil (no-op for stdout).
func OpenStdoutOrFile(path string) (w io.Writer, cleanup func(), err error) {
	if strings.TrimSpace(path) == "" {
		return os.Stdout, func() {}, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { _ = f.Close() }, nil
}
