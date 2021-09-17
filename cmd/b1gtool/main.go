package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

// ProjectID is the Google Cloud Project ID where the season data will be loaded.
var ProjectID string

// Force, if set, forcefully overwrite data in Firestore instead of failing if the documents already exist.
var Force bool

// DryRun, if true, will print the firestore objects to console rather than writing them to firestore.
var DryRun bool

func usage() {
	fmt.Fprint(flag.CommandLine.Output(), `Usage: b1gtool [global-flags] <command>

B1GTool: a command-line tool for managing B1G Pick 'Em data and picks.

Commands:
  setup-season

Global Flags:
`)

	flag.PrintDefaults()
}

func init() {
	flag.Usage = usage

	flag.StringVar(&ProjectID, "project", "", "Google Cloud Project ID.  If equal to the empty string, the environment variable GCP_PROJECT will be used.")
	flag.BoolVar(&Force, "force", false, "Force overwrite of data in Firestore with the SET command rather than failing if the data already exists.")
	flag.BoolVar(&DryRun, "dryrun", false, "Do not write to firestore, but print to console instead.")
}

func main() {
	parseCommandLine()
	switch flag.Arg(0) {
	case "help":
		help()
	case "setup-season":
		setupSeason()
	default:
		flag.Usage()
		log.Fatalf("Command '%s' not understood", flag.Arg(0))
	}
	log.Print("Done.")
}

func parseCommandLine() {
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	if flag.Arg(0) == "help" {
		return
	}

	if ProjectID == "" {
		ProjectID = os.Getenv("GCP_PROJECT")
	}
	if ProjectID == "" {
		log.Fatal("Project ID flag not supplied and no GCP_PROJECT environment variable found")
	}
}