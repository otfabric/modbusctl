// SPDX-License-Identifier: MIT

package cmd

import (
	"net"
	"slices"
	"strings"

	"github.com/otfabric/go-modbus/codec"
	"github.com/otfabric/modbusctl/internal/cli"
	"github.com/otfabric/modbusctl/internal/format"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// fixedFlagValues are the value sets of flags that the commands do not register a
// completion for themselves. A command that registered its own keeps it.
var fixedFlagValues = map[string][]string{
	"format":        formatValues(),
	"function":      {"0", "1", "2", "3", "4"}, // capture commands: 0 = the only one
	"before-first":  {"hold", "absent"},
	"missing":       {"exception", "zero", "silence"},
	"retry-timeout": {"0", "1"},
	"data-bits":     {"0", "5", "6", "7", "8"},
	"stop-bits":     {"0", "1", "2"},
	"baud":          {"0", "1200", "2400", "4800", "9600", "19200", "38400", "57600", "115200"},
	"family":        codecFamilies(),
	"value-kind":    codecValueKinds(),
}

// fileFlagExtensions are the flags that name an input file of a known type.
var fileFlagExtensions = map[string][]string{
	"input":       {"mbcap", "jsonl", "csv"},
	"profile":     {"json"},
	"blocks-file": {"json"},
	"tls-ca":      {"pem", "crt", "cer"},
	"tls-cert":    {"pem", "crt", "cer"},
	"tls-key":     {"pem", "key"},
}

// anyFileFlags name a file of any type (an output path).
var anyFileFlags = []string{"output"}

// urlSchemes are the Modbus URL schemes --url completes.
var urlSchemes = []string{"tcp://", "tcp+tls://", "udp://", "rtu://", "ascii://", "rtuovertcp://", "rtuoverudp://", "asciiovertcp://"}

// registerFlagCompletions gives every value flag of c and its subcommands a completion
// function: the values of an enum flag, files of the right type for a file flag, the
// network interfaces, the URL schemes, and no file names for a free value (a number,
// an address). A command without positional arguments completes no file names either.
func registerFlagCompletions(c *cobra.Command) {
	if c.Runnable() && !c.HasSubCommands() && c.ValidArgs == nil && c.ValidArgsFunction == nil {
		c.ValidArgsFunction = cobra.NoFileCompletions
	}
	c.LocalNonPersistentFlags().VisitAll(func(f *pflag.Flag) { registerFlagCompletion(c, f) })
	c.PersistentFlags().VisitAll(func(f *pflag.Flag) { registerFlagCompletion(c, f) })
	for _, sub := range c.Commands() {
		registerFlagCompletions(sub)
	}
}

func registerFlagCompletion(c *cobra.Command, f *pflag.Flag) {
	if f.Value.Type() == "bool" {
		return
	}
	if _, ok := c.GetFlagCompletionFunc(f.Name); ok {
		return
	}
	switch {
	case fixedFlagValues[f.Name] != nil:
		_ = cli.RegisterEnumFlagCompletion(c, f.Name, fixedFlagValues[f.Name])
	case fileFlagExtensions[f.Name] != nil:
		_ = c.RegisterFlagCompletionFunc(f.Name, cobra.FixedCompletions(fileFlagExtensions[f.Name], cobra.ShellCompDirectiveFilterFileExt))
	case slices.Contains(anyFileFlags, f.Name):
		_ = c.RegisterFlagCompletionFunc(f.Name, cobra.FixedCompletions(nil, cobra.ShellCompDirectiveDefault))
	case f.Name == "url":
		_ = c.RegisterFlagCompletionFunc(f.Name, cobra.FixedCompletions(urlSchemes, cobra.ShellCompDirectiveNoFileComp|cobra.ShellCompDirectiveNoSpace))
	case f.Name == "interface":
		_ = c.RegisterFlagCompletionFunc(f.Name, completeInterfaces)
	default:
		_ = c.RegisterFlagCompletionFunc(f.Name, cobra.NoFileCompletions)
	}
}

// netInterfaces lists the network interfaces; a variable so the tests can make it fail.
var netInterfaces = net.Interfaces

func completeInterfaces(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	ifaces, err := netInterfaces()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	names := make([]string, 0, len(ifaces))
	for _, i := range ifaces {
		names = append(names, i.Name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func formatValues() []string {
	vals := make([]string, 0, 3)
	for v := range format.ValueDescriptions() {
		vals = append(vals, v)
	}
	slices.Sort(vals)
	return vals
}

func codecFamilies() []string {
	return codecDescriptorValues(func(d codec.CodecDescriptor) string { return d.Family.String() })
}

func codecValueKinds() []string {
	return codecDescriptorValues(func(d codec.CodecDescriptor) string { return d.ValueKind.String() })
}

func codecDescriptorValues(field func(codec.CodecDescriptor) string) []string {
	var vals []string
	for _, d := range codec.AvailableCodecDescriptors() {
		if v := strings.TrimSpace(field(d)); v != "" && !slices.Contains(vals, v) {
			vals = append(vals, v)
		}
	}
	slices.Sort(vals)
	return vals
}
