package runner

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/spf13/cobra"
)

func TestInvocation_contextRoundTrip(t *testing.T) {
	t.Parallel()
	inv := &Invocation{}
	inv.SetOutputFormat(format.FormatJSON)
	if inv.OutputFormat != format.FormatJSON || !inv.HasOutputFormat {
		t.Fatal("SetOutputFormat")
	}
	inv.SetSuccessExit(7)
	if inv.SuccessExitCode != 7 || !inv.SuccessExitSet {
		t.Fatal("SetSuccessExit")
	}
	ctx := WithInvocation(context.Background(), inv)
	got := InvocationFrom(ctx)
	if got != inv {
		t.Fatal("round trip")
	}
	if InvocationFrom(context.Background()) != nil {
		t.Fatal("missing inv")
	}
}

func TestAttachOutputFormat(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	got, err := AttachOutputFormat(cmd, "json")
	if err != nil || got != format.FormatJSON {
		t.Fatalf("%v %q", err, got)
	}
	_, err = AttachOutputFormat(cmd, "not-a-format")
	if err == nil {
		t.Fatal("expected format error")
	}
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeInvalidFormat {
		t.Fatalf("want FormatInvalid: %v", err)
	}

	inv := &Invocation{}
	cmd2 := &cobra.Command{}
	cmd2.SetContext(WithInvocation(context.Background(), inv))
	if _, err := AttachOutputFormat(cmd2, "table"); err != nil {
		t.Fatal(err)
	}
	if !inv.HasOutputFormat || inv.OutputFormat != format.FormatTable {
		t.Fatal("inv not updated")
	}
}

func TestRunFormattedWithOutputFormat_jsonSuccess(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)
	inv := &Invocation{}
	cmd.SetContext(WithInvocation(context.Background(), inv))
	rr, err := RunFormattedWithOutputFormat(cmd, format.FormatJSON, func(context.Context) (any, error) {
		return map[string]int{"n": 1}, nil
	})
	if err != nil || rr.ExitCode != errs.ExitOK {
		t.Fatalf("err=%v rr=%+v", err, rr)
	}
	if !inv.SuccessExitSet || inv.SuccessExitCode != errs.ExitOK {
		t.Fatal("success exit not set")
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"n"`)) {
		t.Fatalf("out=%q", buf.String())
	}
}

func TestRunFormattedWithOutputFormat_WithSuccessExit_partial(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)
	inv := &Invocation{}
	cmd.SetContext(WithInvocation(context.Background(), inv))
	payload := &types.IdentifyResult{Summary: &types.ResultSummary{Failed: 1}}
	rr, err := RunFormattedWithOutputFormat(cmd, format.FormatJSON, func(context.Context) (any, error) {
		return payload, nil
	}, WithSuccessExit(types.SuccessExitForPayload))
	if err != nil || rr.ExitCode != errs.ExitPartial {
		t.Fatalf("err=%v rr=%+v", err, rr)
	}
	if inv.SuccessExitCode != errs.ExitPartial {
		t.Fatalf("got exit %d", inv.SuccessExitCode)
	}
}

func TestRunFormattedWithOutputFormat_jsonEncodeError(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)
	cmd.SetContext(context.Background())
	_, err := RunFormattedWithOutputFormat(cmd, format.FormatJSON, func(context.Context) (any, error) {
		return make(chan int), nil
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeJSONEncodeFailed {
		t.Fatalf("got %v", err)
	}
}

type mismatchTable struct{}

func (mismatchTable) TableHeaders() []string { return []string{"a", "b"} }
func (mismatchTable) TableRows() [][]string {
	return [][]string{{"only-one"}}
}

func TestRunFormattedWithOutputFormat_tableRenderError(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)
	cmd.SetContext(context.Background())
	_, err := RunFormattedWithOutputFormat(cmd, format.FormatTable, func(context.Context) (any, error) {
		return mismatchTable{}, nil
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeTableRenderFailed {
		t.Fatalf("got %v", err)
	}
}

func TestRunClientFormatted_validateError(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetOut(&bytes.Buffer{})
	err := RunClientFormatted(cmd, "json", func() error {
		return errors.New("bad flags")
	}, func(context.Context) (any, error) { return nil, nil })
	if err == nil {
		t.Fatal("expected error")
	}
	var e *errs.Error
	if !errors.As(err, &e) || e.Kind != errs.KindInvalidInput {
		t.Fatalf("got %v", err)
	}
}

func TestRunClientFormattedWithDebug_propagatesDebug(t *testing.T) {
	t.Parallel()
	root := &cobra.Command{}
	root.PersistentFlags().Bool("debug", false, "")
	sub := &cobra.Command{}
	sub.SetContext(context.Background())
	sub.SetOut(&bytes.Buffer{})
	root.AddCommand(sub)
	var seen bool
	_ = root.PersistentFlags().Set("debug", "true")
	err := RunClientFormattedWithDebug(sub, func(b bool) { seen = b }, "json", func() error { return errors.New("v") },
		func(context.Context) (any, error) { return nil, nil })
	if err == nil {
		t.Fatal("expected validate error")
	}
	if !seen {
		t.Fatal("setDebug not called with true")
	}
}

func TestRegisterStdoutFormatCompletion(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	cmd.Flags().String("format", "", "")
	RegisterStdoutFormatCompletion(cmd)
	fn, ok := cmd.GetFlagCompletionFunc("format")
	if !ok || fn == nil {
		t.Fatal("missing completion")
	}
	out, _ := fn(cmd, nil, "js")
	if len(out) == 0 {
		t.Fatalf("completions %v", out)
	}
}

func TestWireClientCommand(t *testing.T) {
	t.Parallel()
	parent := &cobra.Command{Use: "client"}
	child := &cobra.Command{Use: "probe"}
	var cfg struct{}
	WireClientCommand(parent, child, &cfg)
	if parent.Commands()[0] != child {
		t.Fatal("child not added")
	}
}
