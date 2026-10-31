// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
)

// check runs the health check with args and returns its exit code and log output.
func check(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	code := run(args, log.New(&buf, "", 0))
	return code, buf.String()
}

// listen returns the address of a TCP listener that accepts and holds connections.
func listen(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = conn.Close() })
		}
	}()
	return l.Addr().String()
}

// closedAddr returns a TCP address nothing listens on.
func closedAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

// httpServer serves the given status code and counts requests.
func httpServer(t *testing.T, status int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	hits := new(atomic.Int32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, hits
}

func TestRun_noChecks(t *testing.T) {
	code, out := check(t)
	if code != 0 || out != "" {
		t.Fatalf("no checks: exit %d, output %q", code, out)
	}
}

func TestRun_tcp(t *testing.T) {
	a, b := listen(t), listen(t)
	code, out := check(t, "-tcp", a, "-tcp", b)
	if code != 0 {
		t.Fatalf("exit %d, want 0:\n%s", code, out)
	}
	if want := a + " : Online\n" + b + " : Online\n"; out != want {
		t.Fatalf("output %q, want %q", out, want)
	}

	// The first unreachable address fails the check; later ones are not tried.
	down := closedAddr(t)
	code, out = check(t, "-tcp", a, "-tcp", down, "-tcp", b)
	if code != 1 {
		t.Fatalf("exit %d, want 1:\n%s", code, out)
	}
	if !strings.HasPrefix(out, a+" : Online\nConnection error: ") || !strings.HasSuffix(out, down+" : Unreachable\n") {
		t.Fatalf("unexpected output:\n%s", out)
	}
	if strings.Contains(out, b) {
		t.Fatalf("checks continued after a failure:\n%s", out)
	}
}

func TestRun_http(t *testing.T) {
	ok, okHits := httpServer(t, http.StatusNoContent)
	second, _ := httpServer(t, http.StatusOK)
	notFound, _ := httpServer(t, http.StatusNotFound)
	broken, _ := httpServer(t, http.StatusInternalServerError)

	code, out := check(t, "-http", ok.URL, "-http", second.URL)
	if code != 0 {
		t.Fatalf("exit %d, want 0:\n%s", code, out)
	}
	if want := ok.URL + " : Online - response: 204\n" + second.URL + " : Online - response: 200\n"; out != want {
		t.Fatalf("output %q, want %q", out, want)
	}
	if okHits.Load() != 1 {
		t.Fatalf("expected one request, server saw %d", okHits.Load())
	}

	// Status codes from 400 up fail the check.
	for _, srv := range []*httptest.Server{notFound, broken} {
		code, out = check(t, "-http", srv.URL, "-http", ok.URL)
		if code != 1 {
			t.Fatalf("exit %d, want 1:\n%s", code, out)
		}
		if !strings.HasPrefix(out, srv.URL+" : Error - response: ") || strings.Contains(out, ok.URL) {
			t.Fatalf("unexpected output:\n%s", out)
		}
	}
	if okHits.Load() != 1 {
		t.Fatalf("checks continued after a failure: server saw %d requests", okHits.Load())
	}

	// An unreachable server fails the check.
	down := "http://" + closedAddr(t)
	code, out = check(t, "-http", down)
	if code != 1 {
		t.Fatalf("exit %d, want 1:\n%s", code, out)
	}
	if !strings.HasPrefix(out, "Connection error: ") || !strings.HasSuffix(out, down+" : Unreachable\n") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func TestRun_tcpThenHTTP(t *testing.T) {
	addr := listen(t)
	srv, hits := httpServer(t, http.StatusOK)
	code, out := check(t, "-http", srv.URL, "-tcp", addr)
	if code != 0 {
		t.Fatalf("exit %d, want 0:\n%s", code, out)
	}
	// TCP checks run before HTTP checks, whatever the flag order.
	if want := addr + " : Online\n" + srv.URL + " : Online - response: 200\n"; out != want {
		t.Fatalf("output %q, want %q", out, want)
	}

	// A failed TCP check stops before any HTTP request is made.
	code, _ = check(t, "-tcp", closedAddr(t), "-http", srv.URL)
	if code != 1 || hits.Load() != 1 {
		t.Fatalf("exit %d, HTTP server saw %d requests", code, hits.Load())
	}
}

// quietStderr silences the flag package's usage output for the duration of the test.
func quietStderr(t *testing.T) {
	t.Helper()
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = null
	t.Cleanup(func() {
		os.Stderr = orig
		_ = null.Close()
	})
}

func TestRun_flags(t *testing.T) {
	quietStderr(t)
	if code, out := check(t, "-no-such-flag"); code != 2 || out != "" {
		t.Fatalf("unknown flag: exit %d, output %q", code, out)
	}
	if code, _ := check(t, "-tcp"); code != 2 {
		t.Fatalf("flag without a value: exit %d, want 2", code)
	}
	if code, out := check(t, "-h"); code != 0 || out != "" {
		t.Fatalf("-h: exit %d, output %q", code, out)
	}
}

func TestArrayFlags(t *testing.T) {
	var f arrayFlags
	for _, v := range []string{"a:1", "b:2"} {
		if err := f.Set(v); err != nil {
			t.Fatal(err)
		}
	}
	if len(f) != 2 || f[0] != "a:1" || f[1] != "b:2" {
		t.Fatalf("arrayFlags = %v", f)
	}
	if f.String() == "" {
		t.Fatal("String must not be empty")
	}
}

// TestMain_process runs the real main in a child process to check the exit status and the
// standard logger output.
func TestMain_process(t *testing.T) {
	if args := os.Getenv("HEALTHCHECK_TEST_ARGS"); args != "" {
		os.Args = append([]string{"healthcheck"}, strings.Split(args, " ")...)
		main()
		return
	}
	up, down := listen(t), closedAddr(t)
	for _, tc := range []struct {
		addr string
		want int
		log  string
	}{
		{up, 0, up + " : Online"},
		{down, 1, down + " : Unreachable"},
	} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestMain_process$")
		cmd.Env = append(os.Environ(), "HEALTHCHECK_TEST_ARGS=-tcp "+tc.addr)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err := cmd.Run()
		got := 0
		if err != nil {
			ee, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("run child: %v", err)
			}
			got = ee.ExitCode()
		}
		if got != tc.want {
			t.Fatalf("-tcp %s: exit %d, want %d\n%s", tc.addr, got, tc.want, stderr.String())
		}
		if !strings.Contains(stderr.String(), tc.log) {
			t.Fatalf("-tcp %s: stderr %q does not contain %q", tc.addr, stderr.String(), tc.log)
		}
	}
}
