package runner

// RunResult is the non-fatal completion from RunFormatted (fatal uses the error return).
type RunResult struct {
	ExitCode int
}
