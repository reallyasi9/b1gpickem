package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

// modelFlagSet is a flag.FlagSet for parsing the setup-model subcommand.
var modelFlagSet *flag.FlagSet

// modelUsage is the usage documentation for the setup-model subcommand.
func modelUsage() {
	fmt.Fprintf(flag.CommandLine.Output(), `Usage: b1gtool [global-flags] setup-model [flags] <shortname> <systemname>
	
Establish a new model in Firestore.
	
Arguments:
  shortname string
        Short name of model used in ThePredictionTracker.com CSV file (begins with "line").
  systemname string
        Name of model used in ThePredictionTracker.com results tracker table.

Flags:
`)

	modelFlagSet.PrintDefaults()

	fmt.Fprint(flag.CommandLine.Output(), "\nGlobal Flags:\n")

	flag.PrintDefaults()

}

func init() {
	modelFlagSet = flag.NewFlagSet("setup-model", flag.ExitOnError)
	modelFlagSet.SetOutput(flag.CommandLine.Output())
	modelFlagSet.Usage = modelUsage

	Commands["setup-model"] = setupModel
	Usage["setup-model"] = modelUsage
}

func setupModel() {
	err := modelFlagSet.Parse(flag.Args()[1:])
	if err != nil {
		log.Fatalf("Failed to parse setup-model arguments: %v", err)
	}
	if modelFlagSet.NArg() < 2 {
		modelFlagSet.Usage()
		log.Fatal("Model short name and system name not supplied")
	}
	if modelFlagSet.NArg() > 2 {
		modelFlagSet.Usage()
		log.Fatal("Too many arguments supplied")
	}

	ctx := context.Background()
	fsClient, err := fs.NewClient(ctx, ProjectID)
	if err != nil {
		log.Fatalf("Error creating firestore client: %v", err)
	}

	shortName := modelFlagSet.Arg(0)
	systemName := modelFlagSet.Arg(1)
	model := firestore.Model{
		System:    systemName,
		ShortName: shortName,
	}
	ref := fsClient.Collection("models").Doc(shortName)

	if DryRun {
		log.Printf("DRY RUN: would write the following to firestore at %s:", ref.Path)
		log.Println(model)
		return
	}

	if Force {
		_, err = ref.Set(ctx, &model)
	} else {
		_, err = ref.Create(ctx, &model)
	}
	if err != nil {
		log.Fatalf("Error writing to firestore: %v", err)
	}
}
