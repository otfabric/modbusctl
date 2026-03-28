package runner

import (
	"context"

	"github.com/otfabric/modbusctl/internal/format"
)

type invocationKey struct{}

// Invocation holds per-invocation output format and success exit code (mutated through command run).
type Invocation struct {
	OutputFormat    format.OutputFormat
	HasOutputFormat bool
	SuccessExitCode int
	SuccessExitSet  bool
}

// SetOutputFormat records the resolved stdout format after format.Parse.
func (inv *Invocation) SetOutputFormat(f format.OutputFormat) {
	inv.OutputFormat = f
	inv.HasOutputFormat = true
}

// SetSuccessExit records the process exit code for a successful command (Err() == nil).
func (inv *Invocation) SetSuccessExit(code int) {
	inv.SuccessExitCode = code
	inv.SuccessExitSet = true
}

// WithInvocation attaches inv to ctx (typically root ExecuteContext).
func WithInvocation(ctx context.Context, inv *Invocation) context.Context {
	return context.WithValue(ctx, invocationKey{}, inv)
}

// InvocationFrom returns the invocation state from ctx, or nil.
func InvocationFrom(ctx context.Context) *Invocation {
	v, _ := ctx.Value(invocationKey{}).(*Invocation)
	return v
}
