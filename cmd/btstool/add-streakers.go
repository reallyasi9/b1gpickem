package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"cloud.google.com/go/firestore"
	bpefs "github.com/reallyasi9/b1gpickem/internal/firestore"
)

var streakerSeason int

var streakerFlagSet *flag.FlagSet

func init() {
	streakerFlagSet = flag.NewFlagSet("add-streakers", flag.ExitOnError)
	streakerFlagSet.SetOutput(flag.CommandLine.Output())
	streakerFlagSet.Usage = streakerUsage

	streakerFlagSet.IntVar(&streakerSeason, "season", -1, "Season year. Negative values will calculate season based on today's date.")

	Commands["add-streakers"] = addStreakers
	Usage["add-streakers"] = streakerUsage
}

func streakerUsage() {
	w := flag.CommandLine.Output()
	fmt.Fprint(w, `btstool [global-flags] add-streakers [flags] streaker [streaker...]

Instantiate streakers for a season's BTS competition.

Arguments:
  streaker
      A picker (specified by short name) to add to the BTS competition. Multiple iterations are allowed.

Flags:
`)

	streakerFlagSet.PrintDefaults()

	fmt.Fprint(flag.CommandLine.Output(), "\nGlobal Flags:\n")

	flag.PrintDefaults()
}

func addStreakers() {
	ctx := context.Background()

	err := streakerFlagSet.Parse(flag.Args()[1:])
	if err != nil {
		log.Fatalf("Failed to parse add-streakers arguments: %v", err)
	}

	if streakerFlagSet.NArg() < 1 {
		streakerFlagSet.Usage()
		log.Fatal("At least one streaker required")
	}

	fsclient, err := firestore.NewClient(ctx, ProjectID)
	if err != nil {
		log.Print(err)
		log.Fatalf("Check that the project ID \"%s\" is correctly specified (either the -project flag or the GCP_PROJECT environment variable)", ProjectID)
	}

	season, seasonRef, err := bpefs.GetSeason(ctx, fsclient, streakerSeason)
	if err != nil {
		log.Fatal(err)
	}

	_, weekRef, err := bpefs.GetWeek(ctx, fsclient, seasonRef, 1)
	if err != nil {
		log.Fatal(err)
	}

	strs := make([]bpefs.StreakTeamsRemaining, streakerFlagSet.NArg())
	for i, name := range streakerFlagSet.Args() {
		var pickerRef *firestore.DocumentRef
		var ok bool
		if pickerRef, ok = season.Pickers[name]; !ok {
			log.Fatalf("Picker '%s' not playing in season %d", name, season.Year)
		}

		str, _, err := bpefs.GetStreakTeamsRemaining(ctx, fsclient, seasonRef, nil, pickerRef)
		if err != nil {
			log.Fatalf("Cannot get streak teams remaining: %v", err)
		}
		strs[i] = str
	}

	if DryRun {
		log.Print("DRY RUN: would write the following to Firestore:")
		for _, str := range strs {
			log.Printf("%v", str)
		}
		return
	}

	coll := weekRef.Collection("streak-teams-remaining")
	err = fsclient.RunTransaction(ctx, func(c context.Context, t *firestore.Transaction) error {
		for _, str := range strs {
			doc := coll.Doc(str.Picker.ID)
			var err error
			if Force {
				err = t.Set(doc, &str)
			} else {
				err = t.Create(doc, &str)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		log.Fatalf("Unable to write to Firestore: %v", err)
	}
}
