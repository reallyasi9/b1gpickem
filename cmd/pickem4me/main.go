package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/firestore"
)

// season holds the flag value of the season of the slate.
var season int

// week holds the flag value of the week of the slate.
var week int

// picker holds the flag value of the picker (for Beat the Streak picks).
var picker string

// suSystem holds the flag value of the system preferred for picking straight-up games.
var suSystem string

// nsSystem holds the flag value of the system preferred for picking noisy spread games.
var nsSystem string

// sdSystem holds the flag value of the system preferred for picking superdog games.
var sdSystem string

// fallback holds the flag value specifying whether to error out rather than use fallback systems for prediction.
var fallback bool

// dryRun holds the flag value specifying whether data should not be written to Firestore.
var dryRun bool

// output holds the flag value of the output file name.
var output string

// projectID is the Google Cloud Storage project ID.
var projectID string

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(), `Usage: pickem4me [flags]
	
Records picks for a slate's games using predictions stored in Firestore.

Flags:
`)

	flag.PrintDefaults()
}

func init() {
	flag.Usage = usage

	flag.IntVar(&season, "season", -1, "`Season` of the slate to pick. If less than zero, the most recent season in Firestore will be used.")
	flag.IntVar(&week, "week", -1, "`Week` of the slate to pick. If less than zero, the week with the closest start date not in the past in Firestore will be used.")
	flag.StringVar(&picker, "picker", "", "`Picker` for Beat the Streak picks. If an empty string, no Beat the Streak picks will be made.")
	flag.StringVar(&suSystem, "su", "", "`System` to use for straight-up picks. System names begin with \"line\". If an empty string and \"-fallback\" is true, the system with the best straight-up prediction accuracy will be used, else an error is returned.")
	flag.StringVar(&nsSystem, "ns", "", "`System` to use for noisy spread picks. System names begin with \"line\". If an empty string and \"-fallback\" is true, the system with the best mean squared error will be used, else an error is returned.")
	flag.StringVar(&sdSystem, "sd", "", "`System` to use for superdog picks. System names begin with \"line\". If an empty string and \"-fallback\" is true, the system with the best mean squared error will be used, else an error is returned.")
	flag.BoolVar(&fallback, "fallback", true, "If true, predictions missing for the various prediction systems will fall back on the best system available. If all else fails, Sagarin points will be used to predict a spread for the game.")
	flag.BoolVar(&dryRun, "dryrun", false, "If true, log what would be written to Firestore, but do not write anything.")
	flag.StringVar(&output, "output", "", "If not empty, output Excel Workbook to the given `location`. Specify a URL with a gs:// schema to store in a Google Cloud Storage bucket. Ignores \"-dryrun\".")
	flag.StringVar(&projectID, "project", os.Getenv("GCP_PROJECT"), "Use the Firestore database from the given Google Cloud `Project`. Defaults to the environment variable GCP_PROJECT.")
}

func main() {
	flag.Parse()

	ctx := context.Background()
	fsClient, err := fs.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Unable to create Firestore client: %v", err)
	}

	_, seasonRef, err := firestore.GetSeason(ctx, fsClient, season)
	if err != nil {
		log.Fatalf("Unable to determine season from \"%d\": %v", season, err)
	}

	_, weekRef, err := firestore.GetWeek(ctx, fsClient, seasonRef, week)
	if err != nil {
		log.Fatalf("Unable to determine week from \"%d\": %v", week, err)
	}

	slateSSs, err := weekRef.Collection("slates").OrderBy("created", fs.Desc).Limit(1).Documents(ctx).GetAll()
	if err != nil {
		log.Fatalf("Unable to get most recent slate from Firestore: %v", err)
	}
	if len(slateSSs) < 1 {
		log.Fatalf("No slates found in Firestore for season %s, week %s: have you run `b1gtool parse-slate` yet?", seasonRef.ID, weekRef.ID)
	}

	slateRef := slateSSs[0].Ref
	sgss, err := slateRef.Collection("games").Documents(ctx).GetAll()
	if err != nil {
		log.Fatalf("Unable to get games from slate at path \"%s\": %v", slateRef.Path, err)
	}
	log.Printf("Read %d games from slate at path \"%s\"", len(sgss), slateRef.Path)

	perfs, _, err := firestore.GetMostRecentModelPerformances(ctx, fsClient, weekRef)
	if err != nil {
		log.Fatalf("Unable to get model performances: %v\nHave you run update-models?", err)
	}

	// TODO: performance by model short name... need to get short names from perf.Model
	models, modelRefs, err := firestore.GetModels(ctx, fsClient)
	if err != nil {
		log.Fatalf("Unable to get model information: %v\nHave you run setup-model?", err)
	}
	modelLookup := firestore.NewModelRefsBySystem(models, modelRefs)

	for _, ss := range sgss {
		var sgame firestore.SlateGame
		err = ss.DataTo(&sgame)
		if err != nil {
			log.Fatalf("Unable to convert SlateGame at path \"%s\": %v", ss.Ref.Path, err)
		}

		gamess, err := sgame.Game.Get(ctx)
		if err != nil {
			log.Fatalf("Unable to get game at path \"%s\": %v", sgame.Game.Path, err)
		}
		var game firestore.Game
		err = gamess.DataTo(&game)
		if err != nil {
			log.Fatalf("Unable to convert Game at path \"%s\": %v", gamess.Ref.Path, err)
		}

		var modelChoice string
		var gt gameType
		switch {
		case sgame.NoisySpread != 0:
			modelChoice = nsSystem
			gt = noisySpread
		case sgame.Superdog:
			modelChoice = sdSystem
			gt = superdog
		default:
			modelChoice = suSystem
			gt = straightUp
		}

		pick, err := pickEm(sgame, modelLookup, modelChoice, gt, fallback)
		log.Print(pick)
	}
}

type gameType int

const (
	straightUp gameType = iota
	noisySpread
	superdog
)

func pickEm(sg firestore.SlateGame, mlu firestore.ModelRefsByName, choice string, gt gameType, fallback bool) (firestore.Pick, error) {

	predictions := sg.Game.Collection("predictions")

}
