package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

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

// resetRoot restores every flag of the real command tree and detaches captured state.
func resetRoot() {
	walkCommands(rootCmd, func(c *cobra.Command) {
		c.Flags().VisitAll(resetFlag)
		c.PersistentFlags().VisitAll(resetFlag)
		// Drop the context of the previous invocation: cobra only hands a command the
		// context of the new run when it has none.
		var none context.Context
		c.SetContext(none)
		c.SetOut(nil)
		c.SetErr(nil)
	})
}

// execute runs the real modbusctl command line through Execute, as main does, and returns
// the exit code with everything written to the process stdout and stderr.
func execute(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	dir := t.TempDir()
	outFile, err := os.Create(filepath.Join(dir, "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	errFile, err := os.Create(filepath.Join(dir, "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outFile, errFile
	restore := func() { os.Stdout, os.Stderr = origOut, origErr }
	defer restore()

	resetRoot()
	defer resetRoot()
	code = run(args)
	restore()

	read := func(f *os.File) string {
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(f.Name())
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	return code, read(outFile), read(errFile)
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

// refusedURL is a Modbus TCP URL on which nothing listens.
func refusedURL(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return "tcp://" + addr
}

func TestRootCommandTree(t *testing.T) {
	var got []string
	for _, c := range rootCmd.Commands() {
		// cobra adds help and __complete to the root on its first Execute; with
		// -count > 1 an earlier run in this process has already done so.
		if c.Name() == "help" || strings.HasPrefix(c.Name(), "__") {
			continue
		}
		got = append(got, c.Name())
	}
	want := []string{"capture", "client", "completion", "discover", "server", "version"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("root subcommands %v, want %v", got, want)
	}
	if f := rootCmd.PersistentFlags().Lookup("debug"); f == nil || f.DefValue != "false" {
		t.Fatalf("root --debug flag: %+v", f)
	}
	if !rootCmd.SilenceUsage || !rootCmd.SilenceErrors {
		t.Fatal("root must silence cobra usage and errors: fatals are rendered by Execute")
	}
}

func TestVersion(t *testing.T) {
	defer SetBuildMeta(buildMeta.version, buildMeta.tag, buildMeta.commit, buildMeta.buildDate)

	SetBuildMeta(" v9.8.7 ", "v9.8.7", "abc1234", "2026-01-02T03:04:05Z")
	code, stdout, stderr := execute(t, "version")
	if code != errs.ExitOK || stderr != "" {
		t.Fatalf("version: exit %d, stderr %q", code, stderr)
	}
	want := "modbusctl version: v9.8.7\ntag:         v9.8.7\ncommit:      abc1234\nbuild date:  2026-01-02T03:04:05Z\n"
	if stdout != want {
		t.Fatalf("version output:\n%q\nwant:\n%q", stdout, want)
	}

	// Without link-time metadata only the version line is printed.
	SetBuildMeta("", "", " ", "")
	code, stdout, _ = execute(t, "version")
	if code != errs.ExitOK || stdout != "modbusctl version: (unknown)\n" {
		t.Fatalf("version without metadata: exit %d, output %q", code, stdout)
	}
}

func TestCompletion(t *testing.T) {
	markers := map[string]string{
		completionBash:       "__start_modbusctl",
		completionZsh:        "#compdef modbusctl",
		completionFish:       "complete -c modbusctl",
		completionPowerShell: "Register-ArgumentCompleter",
	}
	if len(markers) != len(completionShells) {
		t.Fatalf("test covers %d shells, command offers %v", len(markers), completionShells)
	}
	for _, shell := range completionShells {
		code, stdout, stderr := execute(t, "completion", shell)
		if code != errs.ExitOK || stderr != "" {
			t.Fatalf("completion %s: exit %d, stderr %q", shell, code, stderr)
		}
		wantContains(t, stdout, markers[shell], "modbusctl")
	}

	for _, args := range [][]string{{"completion"}, {"completion", "tcsh"}, {"completion", "bash", "zsh"}} {
		code, stdout, stderr := execute(t, args...)
		if code == errs.ExitOK {
			t.Fatalf("%v: expected a failure", args)
		}
		if stdout != "" {
			t.Fatalf("%v: stdout must stay empty, got %q", args, stdout)
		}
		wantContains(t, stderr, "error:")
	}

	// The shell argument itself completes to the supported shells.
	code, stdout, _ := execute(t, cobra.ShellCompNoDescRequestCmd, "completion", "")
	if code != errs.ExitOK {
		t.Fatalf("shell completion request: exit %d", code)
	}
	wantContains(t, stdout, completionShells...)
}

func TestExecute_endToEnd(t *testing.T) {
	d := mbtest.NewDevice().SetHolding(100, 1, 2, 3)
	url := "tcp://" + d.ServeTCP(t)
	tmp := t.TempDir()

	code, stdout, stderr := execute(t, "client", "write", "--yes", "--url", url, "--start", "100", "--values", "10,20,30", "--format", "json")
	if code != errs.ExitOK {
		t.Fatalf("write: exit %d\n%s\n%s", code, stdout, stderr)
	}
	var wr types.WriteResult
	decodeJSON(t, stdout, &wr)
	if wr.Function != 16 || wr.Count != 3 {
		t.Fatalf("unexpected write result: %+v", wr)
	}
	if got := d.Holding(100, 3); !reflect.DeepEqual(got, []uint16{10, 20, 30}) {
		t.Fatalf("holding = %v", got)
	}

	capture := filepath.Join(tmp, "read.mbcap")
	code, stdout, stderr = execute(t, "client", "read", "--url", url, "--start", "100", "--count", "3", "--output", capture, "--format", "json")
	if code != errs.ExitOK {
		t.Fatalf("read: exit %d\n%s\n%s", code, stdout, stderr)
	}
	var rd types.ReadResult
	decodeJSON(t, stdout, &rd)
	if rd.RawDataHex != "00 0A 00 14 00 1E" || rd.CaptureOutputPath != capture {
		t.Fatalf("unexpected read result: %+v", rd)
	}

	// The capture written by "client read" is readable by the capture commands.
	code, stdout, stderr = execute(t, "capture", "extract", "--input", capture)
	if code != errs.ExitOK {
		t.Fatalf("capture extract: exit %d\n%s\n%s", code, stdout, stderr)
	}
	wantContains(t, stdout, `"start_address": 100`, `"register_count": 3`)

	code, stdout, stderr = execute(t, "client", "poll", "--url", url, "--start", "100", "--polls", "2", "--interval", "1", "--format", "json")
	if code != errs.ExitOK {
		t.Fatalf("poll: exit %d\n%s\n%s", code, stdout, stderr)
	}
	if lines := strings.Split(strings.TrimSpace(stdout), "\n"); len(lines) != 2 {
		t.Fatalf("expected 2 JSON lines, got:\n%s", stdout)
	}
	wantContains(t, stderr, "2 succeeded, 0 failed")

	code, stdout, _ = execute(t, "discover", "--subnets", "127.0.0.1/32", "--port", strings.TrimPrefix(url, "tcp://127.0.0.1:"))
	if code != errs.ExitOK {
		t.Fatalf("discover: exit %d", code)
	}
	wantContains(t, stdout, "Modbus device found at 127.0.0.1:")
}

func TestExecute_partialExit(t *testing.T) {
	d := mbtest.NewDevice().SetHolding(100, 1)
	d.Identification = map[uint8]string{0: "Acme", 1: "P1", 2: "1.0"}
	d.Units = []uint8{1}
	url := "tcp://" + d.ServeTCP(t)

	code, stdout, stderr := execute(t, "client", "identify", "--url", url, "--unit", "1-2", "--format", "json")
	if code != errs.ExitPartial {
		t.Fatalf("identify with a failing unit: exit %d, want %d\n%s", code, errs.ExitPartial, stderr)
	}
	var res types.IdentifyResult
	decodeJSON(t, stdout, &res)
	if len(res.Units) != 2 || res.Units[0].Error != nil || res.Units[1].Error == nil {
		t.Fatalf("unexpected identify result: %+v", res.Units)
	}

	// A poll with failed samples exits 7 as well, with the samples on stdout.
	code, stdout, stderr = execute(t, "client", "poll", "--url", url, "--start", "9000", "--polls", "2", "--interval", "1")
	if code != errs.ExitPartial {
		t.Fatalf("poll with failed samples: exit %d, want %d", code, errs.ExitPartial)
	}
	wantContains(t, stdout, "#1", "#2", "error:")
	wantContains(t, stderr, "0 succeeded, 2 failed")
}

func TestExecute_fatal(t *testing.T) {
	d := mbtest.NewDevice().SetHolding(100, 1)
	url := "tcp://" + d.ServeTCP(t)
	out := filepath.Join(t.TempDir(), "x.mbcap")

	// Text mode: the error goes to stderr, stdout stays empty.
	code, stdout, stderr := execute(t, "client", "read", "--start", "100", "--output", out)
	if code != errs.ExitInvalidInput || stdout != "" {
		t.Fatalf("invalid input: exit %d, stdout %q", code, stdout)
	}
	if !strings.HasPrefix(stderr, "error: ") || strings.Contains(stderr, "Usage:") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	// JSON mode: one error envelope on stdout, nothing on stderr.
	code, stdout, stderr = execute(t, "client", "read", "--url", refusedURL(t), "--start", "100", "--output", out, "--format", "json")
	if code != errs.ExitTransport || stderr != "" {
		t.Fatalf("connection refused: exit %d, stderr %q", code, stderr)
	}
	var env types.ErrorEnvelope
	decodeJSON(t, stdout, &env)
	if env.Error.Kind != string(errs.KindTransport) || env.Error.Code == "" || env.Error.Message == "" {
		t.Fatalf("unexpected error envelope: %+v", env)
	}

	// --debug adds the cause chain on stderr.
	code, stdout, stderr = execute(t, "--debug", "client", "read", "--url", refusedURL(t), "--start", "100", "--output", out, "--format", "json")
	if code != errs.ExitTransport {
		t.Fatalf("connection refused with --debug: exit %d", code)
	}
	decodeJSON(t, stdout, &env)
	wantContains(t, stderr, "cause:")

	// A Modbus exception maps to the protocol exit code.
	code, _, stderr = execute(t, "client", "read", "--url", url, "--start", "9000", "--output", out)
	if code != errs.ExitProtocol {
		t.Fatalf("modbus exception: exit %d", code)
	}
	wantContains(t, stderr, "error:", "Illegal Data Address")

	// Errors cobra raises itself are usage errors: an unknown command or flag, a flag value
	// of the wrong type, a required flag that is missing, wrong arguments.
	for _, args := range [][]string{
		{"no-such-command"},
		{"client", "read", "--no-such-flag"},
		{"client", "read", "--port", "no-number"},
		{"capture", "extract"},
		{"completion"},
	} {
		code, stdout, stderr = execute(t, args...)
		if code != errs.ExitUsage || stdout != "" {
			t.Fatalf("%v: exit %d, stdout %q", args, code, stdout)
		}
		wantContains(t, stderr, "error:")
	}
}

func TestClassifyFatal(t *testing.T) {
	if classifyFatal(nil) != nil {
		t.Fatal("nil must stay nil")
	}
	wrapped := errs.New(errs.KindProtocol, errs.CodeSunSpecDetectFailed, "SunSpec detect failed", context.Canceled)
	for _, c := range []struct {
		name string
		err  error
		want errs.Kind
	}{
		{"cobra", errors.New("unknown flag: --x"), errs.KindUsage},
		{"typed", errs.New(errs.KindTransport, errs.CodeTransportConnectFailed, "x", nil), errs.KindTransport},
		{"canceled", fmt.Errorf("read: %w", context.Canceled), errs.KindInterrupted},
		{"canceled inside a typed error", wrapped, errs.KindInterrupted},
	} {
		e, ok := errs.As(classifyFatal(c.err))
		if !ok || e.Kind != c.want {
			t.Errorf("%s: got %+v, want kind %s", c.name, e, c.want)
		}
	}
}

// An untyped error from a running command stays internal: only cobra's own errors are usage.
func TestTypeCommandErrors(t *testing.T) {
	sub := &cobra.Command{Use: "sub", RunE: func(*cobra.Command, []string) error { return errors.New("disk on fire") }}
	ok := &cobra.Command{Use: "ok", RunE: func(*cobra.Command, []string) error { return nil }}
	root := &cobra.Command{Use: "root"}
	root.AddCommand(sub, ok)
	typeCommandErrors(root)

	if err := ok.RunE(ok, nil); err != nil {
		t.Fatal(err)
	}
	e, isTyped := errs.As(classifyFatal(sub.RunE(sub, nil)))
	if !isTyped || e.Kind != errs.KindInternal || e.Message != "disk on fire" {
		t.Fatalf("got %+v", e)
	}
}

func TestExecute_help(t *testing.T) {
	code, stdout, stderr := execute(t, "--help")
	if code != errs.ExitOK || stderr != "" {
		t.Fatalf("--help: exit %d, stderr %q", code, stderr)
	}
	wantContains(t, stdout, "Modbusctl CLI application", "client", "discover", "capture", "server", "version", "completion", "--debug")

	code, stdout, _ = execute(t)
	if code != errs.ExitOK {
		t.Fatalf("no arguments: exit %d", code)
	}
	wantContains(t, stdout, "Available Commands:")
}

func TestRequestedInvocation(t *testing.T) {
	for _, tt := range []struct {
		name string
		env  string
		args []string
		json bool
	}{
		{name: "nothing"},
		{name: "flag", args: []string{"--format", "json"}, json: true},
		{name: "flag with =", args: []string{"--format=JSON"}, json: true},
		{name: "flag at the end without a value", args: []string{"--format"}},
		{name: "environment", env: "json", args: []string{"client"}, json: true},
		{name: "flag wins over the environment", env: "json", args: []string{"--format", "text"}},
		{name: "last flag wins", args: []string{"--format", "json", "--format", "table"}},
		{name: "invalid format", args: []string{"--format", "xml"}},
		{name: "after --", args: []string{"--", "--format", "json"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MODBUSCTL_FORMAT", tt.env)
			if got := requestedInvocation(tt.args).HasOutputFormat; got != tt.json {
				t.Fatalf("HasOutputFormat = %v, want %v", got, tt.json)
			}
		})
	}
}

// wantFatalJSON fails the test unless stdout is exactly one error object, of the given kind
// and code, with nothing on stderr.
func wantFatalJSON(t *testing.T, stdout, stderr string, kind errs.Kind, code string) {
	t.Helper()
	if stderr != "" {
		t.Fatalf("stderr: %q", stderr)
	}
	dec := json.NewDecoder(strings.NewReader(stdout))
	var env struct {
		Error *types.ErrorInfo `json:"error"`
	}
	dec.DisallowUnknownFields()
	if err := dec.Decode(&env); err != nil {
		t.Fatalf("stdout is not one error object: %v\n%s", err, stdout)
	}
	if dec.More() {
		t.Fatalf("more than one JSON value on stdout:\n%s", stdout)
	}
	if env.Error == nil || env.Error.Kind != string(kind) || env.Error.Code != code || env.Error.Message == "" {
		t.Fatalf("error object %+v, want kind %s, code %s", env.Error, kind, code)
	}
}

func TestExecute_fatalJSON(t *testing.T) {
	url := refusedURL(t)
	for _, tt := range []struct {
		name string
		env  string
		args []string
		kind errs.Kind
		code string
		exit int
	}{
		{name: "unknown flag", args: []string{"client", "read", "--format", "json", "--nope"},
			kind: errs.KindUsage, code: errs.CodeInvalidUsage, exit: errs.ExitUsage},
		{name: "unknown flag, format with =", args: []string{"client", "read", "--nope", "--format=json"},
			kind: errs.KindUsage, code: errs.CodeInvalidUsage, exit: errs.ExitUsage},
		{name: "unknown flag, format in the environment", env: "json", args: []string{"client", "read", "--nope"},
			kind: errs.KindUsage, code: errs.CodeInvalidUsage, exit: errs.ExitUsage},
		{name: "unknown command", args: []string{"no-such-command", "--format", "json"},
			kind: errs.KindUsage, code: errs.CodeInvalidUsage, exit: errs.ExitUsage},
		{name: "unknown subcommand", env: "json", args: []string{"client", "no-such-command"},
			kind: errs.KindUsage, code: errs.CodeInvalidUsage, exit: errs.ExitUsage},
		{name: "unknown subcommand of a nested group", env: "json", args: []string{"client", "sunspec", "no-such-command"},
			kind: errs.KindUsage, code: errs.CodeInvalidUsage, exit: errs.ExitUsage},
		{name: "flag value of the wrong type", args: []string{"client", "read", "--format", "json", "--port", "x"},
			kind: errs.KindUsage, code: errs.CodeInvalidUsage, exit: errs.ExitUsage},
		{name: "missing required flag", args: []string{"discover", "--format", "json"},
			kind: errs.KindUsage, code: errs.CodeInvalidUsage, exit: errs.ExitUsage},
		{name: "wrong arguments", args: []string{"completion", "nope", "--format", "json"},
			kind: errs.KindUsage, code: errs.CodeInvalidUsage, exit: errs.ExitUsage},
		{name: "validation before the format is attached", args: []string{"discover", "--subnets", "x", "--format", "json"},
			kind: errs.KindInvalidInput, code: errs.CodeInvalidInput, exit: errs.ExitInvalidInput},
		{name: "missing --yes", args: []string{"client", "write", "--url", url, "--start", "1", "--values", "1", "--format", "json"},
			kind: errs.KindUsage, code: errs.CodeConfirmationRequired, exit: errs.ExitUsage},
		{name: "transport", args: []string{"client", "write", "--yes", "--url", url, "--start", "1", "--values", "1", "--format", "json"},
			kind: errs.KindTransport, code: errs.CodeTransportConnectFailed, exit: errs.ExitTransport},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MODBUSCTL_FORMAT", tt.env)
			code, stdout, stderr := execute(t, tt.args...)
			if code != tt.exit {
				t.Fatalf("exit %d, want %d\n%s\n%s", code, tt.exit, stdout, stderr)
			}
			wantFatalJSON(t, stdout, stderr, tt.kind, tt.code)
		})
	}
}

func TestExecute_fatalNotJSON(t *testing.T) {
	t.Setenv("MODBUSCTL_FORMAT", "json")
	code, stdout, stderr := execute(t, "client", "read", "--nope", "--format", "text")
	if code != errs.ExitUsage || stdout != "" || stderr != "error: unknown flag: --nope\n" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestExecute_unknownSubcommandText(t *testing.T) {
	code, stdout, stderr := execute(t, "server", "no-such-command")
	if code != errs.ExitUsage || stdout != "" || !strings.HasPrefix(stderr, "error: unknown command") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	// A group on its own still shows its help.
	code, stdout, _ = execute(t, "client")
	if code != errs.ExitOK || !strings.Contains(stdout, "sunspec") {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}
