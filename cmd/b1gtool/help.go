package main

import (
	"flag"
	"fmt"
	"log"
)

// Usage is a map of usage functions to commands.
var Usage map[string]func() = make(map[string]func())

var helpFlagSet *flag.FlagSet

func helpUsage() {
	fmt.Fprint(flag.CommandLine.Output(), `Usage: b1gtool [global-flags] help {command}
	
Get help about a particular command.
	
Arguments:
  command string
        Print help for a specific command

Global Flags:`)
	flag.PrintDefaults()

}

func init() {
	Commands["help"] = help
	Usage["help"] = helpUsage

	helpFlagSet = flag.NewFlagSet("help", flag.ContinueOnError)
	helpFlagSet.Usage = helpUsage
}

func help() {
	helpFlagSet.Parse(flag.Args()[1:])
	var cmd string
	if helpFlagSet.NArg() > 0 {
		cmd = helpFlagSet.Arg(0)
	}
	u, ok := Usage[cmd]
	if !ok {
		flag.CommandLine.Usage()
		log.Fatalf("Unrecognized command \"%s\"", cmd)
	}
	u()
}
