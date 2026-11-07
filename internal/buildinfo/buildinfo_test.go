// SPDX-License-Identifier: MIT

package buildinfo

import (
	"runtime"
	"runtime/debug"
	"testing"
)

func setVars(t *testing.T, v, tg, c, d string, bi *debug.BuildInfo) {
	t.Helper()
	ov, ot, oc, od, orb := version, tag, commit, buildDate, readBuildInfo
	t.Cleanup(func() { version, tag, commit, buildDate, readBuildInfo = ov, ot, oc, od, orb })
	version, tag, commit, buildDate = v, tg, c, d
	readBuildInfo = func() (*debug.BuildInfo, bool) { return bi, bi != nil }
}

const platform = runtime.GOOS + "/" + runtime.GOARCH

func TestTextAllFields(t *testing.T) {
	setVars(t, "v3.5.1", "v3.5.1", "1802d0f9afe8642d1e278120b76fcdfac967cb9e", "2026-11-05T15:41:49Z", nil)
	want := "tool 3.5.1 (tag v3.5.1, commit 1802d0f9afe8642d1e278120b76fcdfac967cb9e, built 2026-11-05T15:41:49Z, " + runtime.Version() + ", " + platform + ")"
	if got := Get("tool").String(); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestTextWithoutTag(t *testing.T) {
	setVars(t, "3.6.0-rc1", "", "abc", "2026-11-05T15:41:49Z", nil)
	want := "tool 3.6.0-rc1 (commit abc, built 2026-11-05T15:41:49Z, " + runtime.Version() + ", " + platform + ")"
	if got := Get("tool").String(); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestDevFallbackNothingInjected(t *testing.T) {
	setVars(t, "", "", "", "", nil)
	want := "tool dev (commit unknown, built unknown, " + runtime.Version() + ", " + platform + ")"
	if got := Get("tool").String(); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestDevFallbackFromBuildInfo(t *testing.T) {
	setVars(t, "", "", "", "", &debug.BuildInfo{
		Main: debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "1802d0f9afe8642d1e278120b76fcdfac967cb9e"},
			{Key: "vcs.modified", Value: "true"},
		},
	})
	want := "tool dev (commit 1802d0f9afe8642d1e278120b76fcdfac967cb9e-dirty, built unknown, " + runtime.Version() + ", " + platform + ")"
	if got := Get("tool").String(); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestGoInstallModuleVersion(t *testing.T) {
	setVars(t, "", "", "", "", &debug.BuildInfo{
		Main:     debug.Module{Version: "v3.5.1"},
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abc"}, {Key: "vcs.modified", Value: "false"}},
	})
	info := Get("tool")
	if info.Version != "3.5.1" || info.Commit != "abc" || info.Tag != "" {
		t.Fatalf("got %+v", info)
	}
}

func TestJSONKeyOrderAndValues(t *testing.T) {
	setVars(t, "3.5.1", "v3.5.1", "abc", "2026-11-05T15:41:49Z", nil)
	want := `{"name":"tool","version":"3.5.1","tag":"v3.5.1","commit":"abc","built":"2026-11-05T15:41:49Z","go":"` + runtime.Version() + `","platform":"` + platform + `"}`
	if got := Get("tool").JSON(); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	setVars(t, "", "", "", "", nil)
	want = `{"name":"tool","version":"dev","tag":"","commit":"unknown","built":"unknown","go":"` + runtime.Version() + `","platform":"` + platform + `"}`
	if got := Get("tool").JSON(); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestPseudoAndDirtyModuleVersionsAreDev(t *testing.T) {
	for _, v := range []string{"(devel)", "v0.0.0-20261107212210-dc108a41d537+dirty", "v3.5.2-0.20261107212210-dc108a41d537", "v3.5.1+dirty"} {
		setVars(t, "", "", "", "", &debug.BuildInfo{Main: debug.Module{Version: v}})
		if got := Get("tool").Version; got != "dev" {
			t.Errorf("module version %q: got %q, want dev", v, got)
		}
	}
}
