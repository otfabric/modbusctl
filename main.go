package main

import "github.com/otfabric/modbusctl/cmd"

// Injected at link time (see Makefile): -X main.version=... etc.
var (
	version   = "dev"
	tag       = ""
	commit    = ""
	buildDate = ""
)

func main() {
	cmd.SetBuildMeta(version, tag, commit, buildDate)
	cmd.Execute()
}
