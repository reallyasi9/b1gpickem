package main

import (
	"flag"
	"fmt"
)

var helpFlagSet *flag.FlagSet

func helpUsage() {
	fmt.Fprint(flag.CommandLine.Output(), `Usage: b1gtool [global-flags] help {command}
	
Get help about a particular command.
	
Arguments:
  command string
        Print help for a specific command
`)

}

func init() {
	helpFlagSet = flag.NewFlagSet("help", flag.ContinueOnError)
	helpFlagSet.Usage = helpUsage
}

func help() {
	helpFlagSet.Parse(flag.Args()[1:])
	var cmd string
	if helpFlagSet.NArg() > 0 {
		cmd = helpFlagSet.Arg(0)
	}
	switch cmd {
	case "help":
		helpFlagSet.Usage()
	case "setup-season":
		seasonFlagSet.Usage()
	case "setup-model":
		modelFlagSet.Usage()
	case "update-games":
		ugFlagSet.Usage()
	case "update-predictions":
		upFlagSet.Usage()
	case "update-models":
		umFlagSet.Usage()
	case "":
		fallthrough
	default:
		flag.CommandLine.Usage()
	}
}
