package main

import (
	"flag"
	"fmt"
	"sort"
	"strings"

	"github.com/alecthomas/kong"
)

// ProjectID is the Google Cloud Project ID where the season data will be loaded.
var ProjectID string

// Force, if set, forcefully overwrite data in Firestore instead of failing if the documents already exist.
var Force bool

// DryRun, if true, will print the firestore objects to console rather than writing them to firestore.
var DryRun bool

func usage() {
	cs := make([]string, len(Commands))
	i := 0
	for command := range Commands {
		cs[i] = command
		i++
	}
	sort.Strings(cs)
	cstring := strings.Join(cs, "\n  ")
	fmt.Fprintf(flag.CommandLine.Output(), `Usage: b1gtool [global-flags] <command>

B1GTool: a command-line tool for managing B1G Pick 'Em data and picks.

Commands:
  %s

Global Flags:
`, cstring)

	flag.PrintDefaults()
}

// Commands are nullary functions that are run when commands (the keys of the map) are given as the first argument to the program.
var Commands map[string]func() = make(map[string]func())

func init() {
	flag.Usage = usage

	flag.StringVar(&ProjectID, "project", "", "Google Cloud Project ID.  If equal to the empty string, the environment variable GCP_PROJECT will be used.")
	flag.BoolVar(&Force, "force", false, "Force overwrite of data in Firestore with the SET command rather than failing if the data already exists.")
	flag.BoolVar(&DryRun, "dryrun", false, "Do not write to firestore, but print to console instead.")
}

type globalCmd struct {
	ProjectID string `help:"GCP project ID." env:"GCP_PROJECT" required:""`
}

var CLI struct {
	globalCmd

	Pickers struct {
		Add        addPickersCmd        `cmd:"" help:"Add pickers."`
		Rm         rmPickersCmd         `cmd:"" help:"Remove pickers."`
		Ls         lsPickersCmd         `cmd:"" help:"List all pickers."`
		Edit       editPickerCmd        `cmd:"" help:"Edit picker."`
		Activate   activatePickersCmd   `cmd:"" help:"Activate pickers for a season."`
		Deactivate deactivatePickersCmd `cmd:"" help:"Deactivate pickers for a season."`
	} `cmd:""`

	Teams struct {
		Edit editTeamCmd `cmd:"" help:"Edit team."`
	} `cmd:""`

	Season struct {
		Setup setupSeasonCmd `cmd:"" help:"Setup season."`
	} `cmd:""`
}

func main() {
	ctx := kong.Parse(&CLI)
	err := ctx.Run(&CLI.globalCmd)
	ctx.FatalIfErrorf(err)
}
