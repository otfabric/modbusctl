package errs

import (
	"errors"
	"testing"
)

func TestError_Error_nil(t *testing.T) {
	t.Parallel()
	var e *Error
	if got := e.Error(); got != "" {
		t.Fatalf("got %q want empty", got)
	}
}

func TestError_Error_message(t *testing.T) {
	t.Parallel()
	e := New(KindInvalidInput, CodeInvalidInput, "bad", nil)
	if got := e.Error(); got != "bad" {
		t.Fatalf("got %q", got)
	}
}

func TestError_Error_causeOnly(t *testing.T) {
	t.Parallel()
	cause := errors.New("underlying")
	e := New(KindTransport, CodeTransportConnectFailed, "", cause)
	if got := e.Error(); got != "underlying" {
		t.Fatalf("got %q", got)
	}
}

func TestError_Error_kindFallback(t *testing.T) {
	t.Parallel()
	e := New(KindTimeout, CodeRequestTimeout, "", nil)
	if got := e.Error(); got != string(KindTimeout) {
		t.Fatalf("got %q", got)
	}
}

func TestError_Unwrap(t *testing.T) {
	t.Parallel()
	cause := errors.New("x")
	e := New(KindInternal, CodeInternalUnexpected, "m", cause)
	if e.Unwrap() != cause {
		t.Fatal("unwrap")
	}
	var nilE *Error
	if nilE.Unwrap() != nil {
		t.Fatal("nil unwrap")
	}
}

func TestInvalidInput_Output_Internal(t *testing.T) {
	t.Parallel()
	in := InvalidInput(CodeInvalidURL, "msg", errors.New("c"))
	if in.Kind != KindInvalidInput || in.Code != CodeInvalidURL || in.Message != "msg" {
		t.Fatalf("%+v", in)
	}
	out := Output(CodeJSONEncodeFailed, errors.New("enc"))
	if out.Kind != KindOutput || out.Code != CodeJSONEncodeFailed {
		t.Fatalf("%+v", out)
	}
	out2 := Output(CodeJSONEncodeFailed, nil)
	if out2.Message != "output error" {
		t.Fatalf("got %q", out2.Message)
	}
	inte := Internal(errors.New("boom"))
	if inte.Kind != KindInternal || inte.Code != CodeInternalUnexpected {
		t.Fatalf("%+v", inte)
	}
}

func TestAs(t *testing.T) {
	t.Parallel()
	inner := InvalidInput(CodeInvalidInput, "x", nil)
	wrapped := fmtWrap(inner)
	got, ok := As(wrapped)
	if !ok || got.Message != "x" {
		t.Fatalf("ok=%v got=%+v", ok, got)
	}
	got2, ok2 := As(errors.New("plain"))
	if ok2 || got2 != nil {
		t.Fatal("plain should not As")
	}
}

func fmtWrap(e error) error {
	return &wrap{e: e}
}

type wrap struct{ e error }

func (w *wrap) Error() string { return "wrap: " + w.e.Error() }
func (w *wrap) Unwrap() error { return w.e }

func TestFormatInvalid(t *testing.T) {
	t.Parallel()
	cause := errors.New("parse")
	e := FormatInvalid(cause)
	if e.Code != CodeInvalidFormat || e.Cause != cause {
		t.Fatalf("%+v", e)
	}
}

func TestWrapValidation(t *testing.T) {
	t.Parallel()
	if WrapValidation(nil) != nil {
		t.Fatal("nil in")
	}
	typed := InvalidInput(CodeInvalidUnitSelector, "units", nil)
	if got := WrapValidation(typed); got != typed {
		t.Fatal("preserve typed")
	}
	plain := errors.New("plain")
	got := WrapValidation(plain)
	if got.Kind != KindInvalidInput || got.Code != CodeInvalidInput {
		t.Fatalf("%+v", got)
	}
}

func TestExitCodeForKind(t *testing.T) {
	t.Parallel()
	cases := []struct {
		k    Kind
		want int
	}{
		{KindUsage, ExitUsage},
		{KindInvalidInput, ExitInvalidInput},
		{KindTransport, ExitTransport},
		{KindTimeout, ExitTimeout},
		{KindProtocol, ExitProtocol},
		{KindModbus, ExitProtocol},
		{KindOutput, ExitInternal},
		{KindInternal, ExitInternal},
		{Kind("unknown"), ExitInternal},
	}
	for _, tc := range cases {
		if got := ExitCodeForKind(tc.k); got != tc.want {
			t.Fatalf("%s: got %d want %d", tc.k, got, tc.want)
		}
	}
}
