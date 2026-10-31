// SPDX-License-Identifier: MIT

package errs

// Process exit codes (single mapping from Kind for fatals).
const (
	ExitOK           = 0
	ExitUsage        = 2
	ExitInvalidInput = 3
	ExitTransport    = 4
	ExitTimeout      = 5
	ExitProtocol     = 6
	ExitPartial      = 7
	ExitInternal     = 10
	// ExitInterrupted is SIGINT or SIGTERM before a result (128 + SIGINT, the shell convention).
	ExitInterrupted = 130
)

// ExitCodeForKind maps Kind to exit code for fatal errors.
func ExitCodeForKind(k Kind) int {
	switch k {
	case KindUsage:
		return ExitUsage
	case KindInvalidInput:
		return ExitInvalidInput
	case KindTransport:
		return ExitTransport
	case KindTimeout:
		return ExitTimeout
	case KindProtocol, KindModbus:
		return ExitProtocol
	case KindInterrupted:
		return ExitInterrupted
	case KindOutput, KindInternal:
		return ExitInternal
	default:
		return ExitInternal
	}
}
