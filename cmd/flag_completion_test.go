// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// complete runs the hidden __complete command and returns the candidates (without their
// descriptions) and the directive line.
func complete(t *testing.T, args ...string) ([]string, string) {
	t.Helper()
	code, stdout, stderr := execute(t, append([]string{"__complete"}, args...)...)
	if code != 0 {
		t.Fatalf("__complete %v: exit %d, stderr %q", args, code, stderr)
	}
	var values []string
	var directive string
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if strings.HasPrefix(line, ":") {
			directive = line
			continue
		}
		if strings.HasPrefix(line, "Completion ended") {
			continue
		}
		v, _, _ := strings.Cut(line, "\t")
		values = append(values, v)
	}
	return values, directive
}

func wantCandidates(t *testing.T, args []string, want ...string) {
	t.Helper()
	got, _ := complete(t, args...)
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Errorf("__complete %v = %v, missing %q", args, got, w)
		}
	}
}

func TestCompleteShells(t *testing.T) {
	got, _ := complete(t, "completion", "")
	want := []string{"bash", "zsh", "fish", "powershell"}
	if !slices.Equal(got, want) {
		t.Fatalf("completion shells %v, want %v", got, want)
	}
}

func TestCompleteEnumFlagPerGroup(t *testing.T) {
	wantCandidates(t, []string{"client", "read", "--format", ""}, "text", "json", "table")
	wantCandidates(t, []string{"client", "scan", "--algo", ""}, "safe", "smart", "deep", "sunspec")
	wantCandidates(t, []string{"client", "read", "--parity", ""}, "none", "even", "odd")
	wantCandidates(t, []string{"client", "sunspec", "map", "--regtype", ""}, "holding", "input")
	wantCandidates(t, []string{"client", "read", "--stop-bits", ""}, "1", "2")
	wantCandidates(t, []string{"client", "codecs", "--family", ""}, "float", "integer")
	wantCandidates(t, []string{"client", "read", "--url", "rt"}, "rtu://", "rtuovertcp://")
	wantCandidates(t, []string{"capture", "decode", "--function", ""}, "1", "2", "3", "4")
	wantCandidates(t, []string{"server", "replay", "--missing", ""}, "exception", "zero", "silence")
	wantCandidates(t, []string{"server", "replay", "--before-first", ""}, "hold", "absent")
	wantCandidates(t, []string{"discover", "--format", ""}, "text", "json", "table")
}

func TestCompleteFileFlags(t *testing.T) {
	got, directive := complete(t, "capture", "decode", "--input", "")
	if !slices.Equal(got, []string{"mbcap", "jsonl", "csv"}) {
		t.Fatalf("--input extensions %v", got)
	}
	if want := fmt.Sprintf(":%d", cobra.ShellCompDirectiveFilterFileExt); directive != want {
		t.Fatalf("--input directive %q, want %q", directive, want)
	}
	if _, directive := complete(t, "client", "read", "--timeout", ""); directive != fmt.Sprintf(":%d", cobra.ShellCompDirectiveNoFileComp) {
		t.Fatalf("--timeout directive %q, want no file completion", directive)
	}
}

// valueSetUsage matches a usage string that lists the values of a flag.
var valueSetUsage = regexp.MustCompile(`\w+\|\w+|one of|: \w+, \w+,? (or|and) \w+|\w+ or \w+ \(`)

// TestEveryValueFlagCompletes walks every command: a flag that takes a value has a
// completion function, so a flag whose usage lists a value set cannot lack one.
func TestEveryValueFlagCompletes(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		check := func(f *pflag.Flag) {
			if f.Value.Type() == "bool" || f.Name == "help" {
				return
			}
			if _, ok := c.GetFlagCompletionFunc(f.Name); !ok {
				kind := "value"
				if valueSetUsage.MatchString(f.Usage) {
					kind = "value set"
				}
				t.Errorf("%s --%s (%s flag) has no completion: %s", c.CommandPath(), f.Name, kind, f.Usage)
			}
		}
		c.LocalFlags().VisitAll(check)
		c.InheritedFlags().VisitAll(check)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(rootCmd)
}

func TestCompleteInterfaceFlag(t *testing.T) {
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("net.Interfaces: %v", err)
	}
	want := make([]string, 0, len(ifaces))
	for _, i := range ifaces {
		want = append(want, i.Name)
	}
	got, directive := complete(t, "discover", "--interface", "")
	if !slices.Equal(got, want) {
		t.Fatalf("--interface candidates %v, want %v", got, want)
	}
	if want := fmt.Sprintf(":%d", cobra.ShellCompDirectiveNoFileComp); directive != want {
		t.Fatalf("--interface directive %q, want %q", directive, want)
	}
}

func TestCompleteInterfacesError(t *testing.T) {
	saved := netInterfaces
	t.Cleanup(func() { netInterfaces = saved })
	netInterfaces = func() ([]net.Interface, error) { return nil, errors.New("no interfaces") }

	got, directive := completeInterfaces(nil, nil, "")
	if got != nil {
		t.Fatalf("candidates %v on error, want none", got)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("directive %v on error, want %v", directive, cobra.ShellCompDirectiveNoFileComp)
	}
}
