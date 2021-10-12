package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"

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

		pick, err := pickEm(ctx, fsClient, sgame, modelLookup, perfs, modelChoice, gt, fallback)
		log.Print(pick)
	}
}

type gameType int

const (
	straightUp gameType = iota
	noisySpread
	superdog
)

func pickEm(ctx context.Context, fsClient *fs.Client, sg *fs.DocumentRef, perfs []firestore.ModelPerformance, modelChoice *fs.DocumentRef, gt gameType, fallback bool) (firestore.Pick, error) {
	// TODO: fill out Pick from SlateGame
	p := firestore.Pick{
		SlateGame: sg,
	}

	ss, err := sg.Get(ctx)
	if err != nil {
		return p, fmt.Errorf("unable to get slate game \"%s\": %v", sg.Path, err)
	}
	var sgame firestore.SlateGame
	err = ss.DataTo(&sgame)
	if err != nil {
		return p, fmt.Errorf("unable to build SlateGame from \"%s\": %v", sg.Path, err)
	}

	predictions, predRefs, err := firestore.GetPredictions(ctx, fsClient, sgame.Game)
	if err != nil {
		return p, fmt.Errorf("unable to get predictions for slate game \"%d\": %v", sgame.Game.Path, err)
	}

	ss, err = sgame.Game.Get(ctx)
	if err != nil {
		return p, fmt.Errorf("unable to get game \"%s\": %v", sgame.Game.Path, err)
	}
	var game firestore.Game
	err = ss.DataTo(&game)
	if err != nil {
		return p, fmt.Errorf("unable to build Game from \"%s\": %v", sgame.Game.Path, err)
	}

	// If a specific choice is made, try to get that first.
	if modelChoice != nil {
		// simple linear search will do
		var pred firestore.ModelPrediction
		var nilPred firestore.ModelPrediction
		var i int
		for i, pred = range predictions {
			if pred.Model.Path == modelChoice.Path {
				break
			}
		}
		predRef := predRefs[i]
		var perf firestore.ModelPerformance
		var nilPerf firestore.ModelPerformance
		for _, perf = range perfs {
			if perf.Model.Path == modelChoice.Path {
				break
			}
		}
		// use the prediction to fill out the pick
		if pred != nilPred && perf != nilPerf {
			p.FillOut(game, perf, pred, predRef)
			return p, nil
		}
	}
	if !fallback {
		return p, fmt.Errorf("no predictions for game \"%s\" and no fallback specified", sgame.Game.Path)
	}

	// fallback onto best
	pred, predRef, perf, err := getFallbackPrediction(ctx, predictions, predRefs, perfs, gt)
	if err != nil {
		return p, fmt.Errorf("unable to get fallback prediction: %v", err)
	}
	p.FillOut(game, perf, pred, predRef)

	return p, nil
}

func getFallbackPrediction(ctx context.Context, preds []firestore.ModelPrediction, predRefs []*fs.DocumentRef, ps []firestore.ModelPerformance, gt gameType) (bestPred firestore.ModelPrediction, predRef *fs.DocumentRef, bestPerf firestore.ModelPerformance, err error) {
	if len(preds) == 0 {
		err = fmt.Errorf("unable to get fallback prediction: no predictions for game")
		return
	}

	switch gt {
	case straightUp:
		sort.Sort(sort.Reverse(byWins(ps)))
	case noisySpread:
		fallthrough
	case superdog:
		sort.Sort(byMSE(ps))
	default:
		err = fmt.Errorf("unable to get fallback prediction: unrecognized game type %v", gt)
		return
	}

	// Look for the best in the predictions I have
	predByModelID := make(map[string]firestore.ModelPrediction)
	refByModelID := make(map[string]*fs.DocumentRef)
	for i, pred := range preds {
		predByModelID[pred.Model.ID] = pred
		refByModelID[pred.Model.ID] = predRefs[i]
	}

	for _, perf := range ps {
		var ok bool
		if bestPred, ok = predByModelID[perf.Model.ID]; ok {
			bestPerf = perf
			predRef = refByModelID[perf.Model.ID]
			break
		}
	}
	var noPred firestore.ModelPrediction
	if bestPred == noPred {
		err = fmt.Errorf("unable to get fallback model: no predictions match")
	}

	return
}

type byWins []firestore.ModelPerformance

// Len implements Sortable interface
func (b byWins) Len() int {
	return len(b)
}

// Less implements Sortable interface
func (b byWins) Less(i, j int) bool {
	return b[i].Wins < b[j].Wins
}

// Swap implements Sortable interface
func (b byWins) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

type byMSE []firestore.ModelPerformance

// Len implements Sortable interface
func (b byMSE) Len() int {
	return len(b)
}

// Less implements Sortable interface
func (b byMSE) Less(i, j int) bool {
	return b[i].MSE < b[j].MSE
}

// Swap implements Sortable interface
func (b byMSE) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}
