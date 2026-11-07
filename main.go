// SPDX-License-Identifier: MIT

package main

import (
	"os"

	"github.com/otfabric/modbusctl/cmd"
)

// Injected at link time (see Makefile): -X main.version=... etc.
var (
	version   = ""
	tag       = ""
	commit    = ""
	buildDate = ""
)

func main() {
	cmd.SetBuildMeta(version, tag, commit, buildDate)
	os.Exit(cmd.Execute())
}
