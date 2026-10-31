// SPDX-License-Identifier: MIT

package capture

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/runner"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// testRoot stands in for the modbusctl root command: it owns the persistent --debug flag.
var testRoot = func() *cobra.Command {
	root := &cobra.Command{Use: "modbusctl", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().Bool("debug", false, "Print debug information")
	root.AddCommand(CaptureCmd)
	return root
}()

// syncBuffer is an output buffer that is safe for concurrent writers: some commands write
// progress from a background goroutine.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// cmdResult is the outcome of one command invocation.
type cmdResult struct {
	stdout string
	stderr string
	err    error
	inv    *runner.Invocation
}

// exit returns the process exit code modbusctl would use for this result.
func (r cmdResult) exit() int {
	if r.err != nil {
		if e, ok := errs.As(r.err); ok {
			return errs.ExitCodeForKind(e.Kind)
		}
		return errs.ExitInternal
	}
	if r.inv.SuccessExitSet {
		return r.inv.SuccessExitCode
	}
	return errs.ExitOK
}

// walkCommands calls fn for c and every command below it.
func walkCommands(c *cobra.Command, fn func(*cobra.Command)) {
	fn(c)
	for _, sub := range c.Commands() {
		walkCommands(sub, fn)
	}
}

// resetFlag restores a flag to its default value and clears its Changed state.
func resetFlag(f *pflag.Flag) {
	if sv, ok := f.Value.(pflag.SliceValue); ok {
		def := strings.Trim(f.DefValue, "[]")
		var vals []string
		if def != "" {
			vals = strings.Split(def, ",")
		}
		_ = sv.Replace(vals)
	} else {
		_ = f.Value.Set(f.DefValue)
	}
	f.Changed = false
}

// resetFlags restores every flag in the command tree to its default.
func resetFlags(root *cobra.Command) {
	walkCommands(root, func(c *cobra.Command) {
		c.Flags().VisitAll(resetFlag)
		c.PersistentFlags().VisitAll(resetFlag)
	})
}

// runTo runs the test root with args and ctx, writing stdout to out and capturing stderr.
func runTo(t *testing.T, ctx context.Context, out io.Writer, args ...string) cmdResult {
	t.Helper()
	var stderr syncBuffer
	inv := &runner.Invocation{}
	ctx = runner.WithInvocation(ctx, inv)
	resetFlags(testRoot)
	defer resetFlags(testRoot)
	walkCommands(testRoot, func(c *cobra.Command) {
		c.SetContext(ctx)
		c.SetOut(out)
		c.SetErr(&stderr)
	})
	testRoot.SetArgs(args)
	err := testRoot.ExecuteContext(ctx)
	return cmdResult{stderr: stderr.String(), err: err, inv: inv}
}

// runCtx runs the test root with args and ctx, capturing stdout and stderr.
func runCtx(t *testing.T, ctx context.Context, args ...string) cmdResult {
	t.Helper()
	var stdout syncBuffer
	r := runTo(t, ctx, &stdout, args...)
	r.stdout = stdout.String()
	return r
}

// run runs the test root with args.
func run(t *testing.T, args ...string) cmdResult {
	t.Helper()
	return runCtx(t, context.Background(), args...)
}

// wantExit fails the test unless the result maps to the given process exit code.
func wantExit(t *testing.T, r cmdResult, want int) {
	t.Helper()
	if got := r.exit(); got != want {
		t.Fatalf("exit code %d, want %d (err: %v)\nstdout:\n%s\nstderr:\n%s", got, want, r.err, r.stdout, r.stderr)
	}
}

// wantKind fails the test unless the command failed with a typed error of the given kind.
func wantKind(t *testing.T, r cmdResult, want errs.Kind) {
	t.Helper()
	if r.err == nil {
		t.Fatalf("expected a %s error, got success\nstdout:\n%s", want, r.stdout)
	}
	e, ok := errs.As(r.err)
	if !ok {
		t.Fatalf("expected a typed %s error, got %T: %v", want, r.err, r.err)
	}
	if e.Kind != want {
		t.Fatalf("error kind %q, want %q (err: %v)", e.Kind, want, r.err)
	}
	if r.stdout != "" {
		t.Fatalf("stdout must stay empty on a fatal error, got:\n%s", r.stdout)
	}
}

// wantContains fails the test unless s contains every one of subs.
func wantContains(t *testing.T, s string, subs ...string) {
	t.Helper()
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			t.Fatalf("output does not contain %q:\n%s", sub, s)
		}
	}
}

// decodeJSON unmarshals s into v and fails the test when s is not valid JSON.
func decodeJSON(t *testing.T, s string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(s), v); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, s)
	}
}
