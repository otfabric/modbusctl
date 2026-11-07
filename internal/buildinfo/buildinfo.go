// SPDX-License-Identifier: MIT

// Package buildinfo reports the version, tag, commit and build date of the binary on one
// line or as one JSON object. The release workflow sets the variables at link time with
// -X; a development build (go run, go build, go install) falls back on the module version
// and the VCS stamp of runtime/debug.
package buildinfo

import (
	"encoding/json"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
)

// Set at link time: -X <module>/internal/buildinfo.version=${VERSION} and so on.
var (
	version   string
	tag       string
	commit    string
	buildDate string
)

// readBuildInfo is runtime/debug.ReadBuildInfo; the tests replace it.
var readBuildInfo = debug.ReadBuildInfo

// Info is what `version` prints. The field order is the order of the JSON keys.
type Info struct {
	Name      string    `json:"name"`
	Version   string    `json:"version"`
	Tag       string    `json:"tag"`
	Commit    string    `json:"commit"`
	Built     string    `json:"built"`
	Libraries []Library `json:"libraries"`
	Go        string    `json:"go"`
	Platform  string    `json:"platform"`
}

// Library is an OT Fabric library (a github.com/otfabric/go-* module) the
// binary is built with.
type Library struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// libraryPrefix selects the OT Fabric libraries among the dependencies.
const libraryPrefix = "github.com/otfabric/go-"

// libraries returns the OT Fabric libraries of the build, sorted by name: the
// replacement when a module is replaced, never the main module itself.
func libraries(bi *debug.BuildInfo) []Library {
	libs := []Library{}
	for _, d := range bi.Deps {
		m := d
		if d.Replace != nil && d.Replace.Version != "" {
			m = d.Replace // a replacement by a local directory has no version
		}
		if !strings.HasPrefix(d.Path, libraryPrefix) || d.Path == bi.Main.Path {
			continue
		}
		name := d.Path[strings.LastIndex(d.Path, "/")+1:]
		libs = append(libs, Library{Name: name, Version: strings.TrimPrefix(m.Version, "v")})
	}
	sort.Slice(libs, func(i, j int) bool { return libs[i].Name < libs[j].Name })
	return libs
}

// pseudoVersion matches the timestamp and commit of a Go pseudo-version
// (v0.0.0-20261107212210-dc108a41d537), which go build stamps from the VCS.
var pseudoVersion = regexp.MustCompile(`\d{14}-[0-9a-f]{12}`)

// isRelease reports whether the module version go stamped is a released
// version (go install …@vX.Y.Z), not (devel), a pseudo-version or a build
// of a modified tree (+dirty).
func isRelease(v string) bool {
	return v != "" && v != "(devel)" && !strings.HasSuffix(v, "+dirty") && !pseudoVersion.MatchString(v)
}

// Get resolves the build information of the binary called name.
func Get(name string) Info {
	info := Info{
		Name:     name,
		Version:  strings.TrimPrefix(strings.TrimSpace(version), "v"),
		Tag:      strings.TrimSpace(tag),
		Commit:   strings.TrimSpace(commit),
		Built:    strings.TrimSpace(buildDate),
		Go:       runtime.Version(),
		Platform: runtime.GOOS + "/" + runtime.GOARCH,
	}
	bi, ok := readBuildInfo()
	info.Libraries = []Library{}
	if ok {
		info.Libraries = libraries(bi)
	}
	if info.Version == "" && ok {
		if v := bi.Main.Version; isRelease(v) {
			info.Version = strings.TrimPrefix(v, "v")
		}
	}
	if info.Commit == "" && ok {
		var revision string
		var modified bool
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				revision = s.Value
			case "vcs.modified":
				modified = s.Value == "true"
			}
		}
		if revision != "" {
			info.Commit = revision
			if modified {
				info.Commit += "-dirty"
			}
		}
	}
	if info.Version == "" {
		info.Version = "dev"
	}
	if info.Commit == "" {
		info.Commit = "unknown"
	}
	if info.Built == "" {
		info.Built = "unknown"
	}
	return info
}

// String is the text line: name version (tag …, commit …, built …, libraries…, go,
// os/arch). The tag part is left out when no tag was set, the libraries when there are none.
func (i Info) String() string {
	var b strings.Builder
	b.WriteString(i.Name + " " + i.Version + " (")
	if i.Tag != "" {
		b.WriteString("tag " + i.Tag + ", ")
	}
	b.WriteString("commit " + i.Commit + ", built " + i.Built + ", ")
	for _, l := range i.Libraries {
		b.WriteString(l.Name + " " + l.Version + ", ")
	}
	b.WriteString(i.Go + ", " + i.Platform + ")")
	return b.String()
}

// JSON is the one-line JSON object, keys in the order name, version, tag, commit, built,
// libraries, go, platform.
func (i Info) JSON() string {
	out, err := json.Marshal(i)
	if err != nil { // cannot happen: every field is a string
		return "{}"
	}
	return string(out)
}

// Set replaces the link-time values, for a main package that receives them with -X main.*.
func Set(v, t, c, d string) {
	version, tag, commit, buildDate = v, t, c, d
}
