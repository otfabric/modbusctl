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
	want := `{"name":"tool","version":"3.5.1","tag":"v3.5.1","commit":"abc","built":"2026-11-05T15:41:49Z","libraries":[],"go":"` + runtime.Version() + `","platform":"` + platform + `"}`
	if got := Get("tool").JSON(); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	setVars(t, "", "", "", "", nil)
	want = `{"name":"tool","version":"dev","tag":"","commit":"unknown","built":"unknown","libraries":[],"go":"` + runtime.Version() + `","platform":"` + platform + `"}`
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

func TestLibrariesFromBuildInfo(t *testing.T) {
	setVars(t, "1.0.0", "", "abc", "x", &debug.BuildInfo{
		Main: debug.Module{Path: "github.com/otfabric/go-self", Version: "(devel)"},
		Deps: []*debug.Module{
			{Path: "github.com/otfabric/go-modbus", Version: "v1.11.1"},
			{Path: "github.com/spf13/cobra", Version: "v1.10.2"},
			{Path: "github.com/otfabric/go-canbus", Version: "v0.8.0", Replace: &debug.Module{Path: "../go-canbus", Version: "v0.8.1"}},
			{Path: "github.com/otfabric/go-self", Version: "v9.9.9"},
			{Path: "github.com/otfabric/go-x", Version: "v0.0.0-20261107212210-dc108a41d537"},
		},
	})
	got := Get("tool").Libraries
	want := []Library{{"go-canbus", "0.8.1"}, {"go-modbus", "1.11.1"}, {"go-x", "0.0.0-20261107212210-dc108a41d537"}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestLibrariesFormatting(t *testing.T) {
	base := Info{Name: "tool", Version: "1.0.0", Tag: "v1.0.0", Commit: "abc", Built: "2026-11-05T15:41:49Z", Go: "go1.27.3", Platform: "darwin/arm64"}
	cases := []struct {
		libs       []Library
		text, json string
	}{
		{[]Library{},
			"tool 1.0.0 (tag v1.0.0, commit abc, built 2026-11-05T15:41:49Z, go1.27.3, darwin/arm64)",
			`{"name":"tool","version":"1.0.0","tag":"v1.0.0","commit":"abc","built":"2026-11-05T15:41:49Z","libraries":[],"go":"go1.27.3","platform":"darwin/arm64"}`},
		{[]Library{{"go-canbus", "0.8.0"}},
			"tool 1.0.0 (tag v1.0.0, commit abc, built 2026-11-05T15:41:49Z, go-canbus 0.8.0, go1.27.3, darwin/arm64)",
			`{"name":"tool","version":"1.0.0","tag":"v1.0.0","commit":"abc","built":"2026-11-05T15:41:49Z","libraries":[{"name":"go-canbus","version":"0.8.0"}],"go":"go1.27.3","platform":"darwin/arm64"}`},
		{[]Library{{"go-canbus", "0.8.0"}, {"go-canopen", "0.5.1"}},
			"tool 1.0.0 (tag v1.0.0, commit abc, built 2026-11-05T15:41:49Z, go-canbus 0.8.0, go-canopen 0.5.1, go1.27.3, darwin/arm64)",
			`{"name":"tool","version":"1.0.0","tag":"v1.0.0","commit":"abc","built":"2026-11-05T15:41:49Z","libraries":[{"name":"go-canbus","version":"0.8.0"},{"name":"go-canopen","version":"0.5.1"}],"go":"go1.27.3","platform":"darwin/arm64"}`},
	}
	for _, c := range cases {
		i := base
		i.Libraries = c.libs
		if got := i.String(); got != c.text {
			t.Errorf("text\n got  %q\n want %q", got, c.text)
		}
		if got := i.JSON(); got != c.json {
			t.Errorf("json\n got  %s\n want %s", got, c.json)
		}
	}
}
