package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

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

// force holds the flag value specifying whether data should be forcefully overwritten in Firestore.
var force bool

// projectID is the Google Cloud Storage project ID.
var projectID string

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(), `Usage: pickem4me [flags] <season> <week> <picker>
	
Records picks for a slate's games using predictions stored in Firestore.

Arguments:
  season (int)
      Make picks for this season. If less than zero, the season will be guessed based on today's date.
  week (int)
      Make picks for this week. If less than zero, the week will be guessed based on today's date.
  picker (string)
      Register the picks to this picker. Also used to lookup the Beat the Streak picks.

Flags:
`)

	flag.PrintDefaults()
}

func init() {
	flag.Usage = usage

	flag.StringVar(&suSystem, "su", "", "`System` to use for straight-up picks. System names begin with \"line\". If an empty string and \"-fallback\" is true, the system with the best straight-up prediction accuracy will be used, else an error is returned.")
	flag.StringVar(&nsSystem, "ns", "", "`System` to use for noisy spread picks. System names begin with \"line\". If an empty string and \"-fallback\" is true, the system with the best mean squared error will be used, else an error is returned.")
	flag.StringVar(&sdSystem, "sd", "", "`System` to use for superdog picks. System names begin with \"line\". If an empty string and \"-fallback\" is true, the system with the best mean squared error will be used, else an error is returned.")
	flag.BoolVar(&fallback, "fallback", true, "If true, predictions missing for the various prediction systems will fall back on the best system available. If all else fails, Sagarin points will be used to predict a spread for the game.")
	flag.BoolVar(&dryRun, "dryrun", false, "If true, log what would be written to Firestore, but do not write anything.")
	flag.BoolVar(&force, "force", false, "If true, force overwrite exiting documents in Firestore.")
	flag.StringVar(&projectID, "project", os.Getenv("GCP_PROJECT"), "Use the Firestore database from the given Google Cloud `Project`. Defaults to the environment variable GCP_PROJECT.")
}

func main() {
	flag.Parse()

	ctx := context.Background()
	fsClient, err := fs.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Unable to create Firestore client: %v", err)
	}

	if flag.NArg() != 3 {
		flag.Usage()
		log.Fatalf("Season, Week, and Picker arguments required.")
	}
	season, err := strconv.Atoi(flag.Arg(0))
	if err != nil {
		flag.Usage()
		log.Fatalf("Season '%s' not an integer.", flag.Arg(0))
	}
	week, err := strconv.Atoi(flag.Arg(1))
	if err != nil {
		flag.Usage()
		log.Fatalf("Week '%s' not an integer.", flag.Arg(1))
	}
	picker := flag.Arg(2)
	_, pkRef, err := firestore.GetPickerByLukeName(ctx, fsClient, picker)
	if err != nil {
		log.Fatalf("Unable to lookup picker '%s': %v", picker, err)
	}

	_, seasonRef, err := firestore.GetSeason(ctx, fsClient, season)
	if err != nil {
		log.Fatalf("Unable to determine season from \"%d\": %v", season, err)
	}
	log.Printf("Using season %s", seasonRef.ID)

	_, weekRef, err := firestore.GetWeek(ctx, seasonRef, week)
	if err != nil {
		log.Fatalf("Unable to determine week from \"%d\": %v", week, err)
	}
	log.Printf("Using week %s", weekRef.ID)

	slateSSs, err := weekRef.Collection("slates").OrderBy("parsed", fs.Desc).Limit(1).Documents(ctx).GetAll()
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

	models, modelRefs, err := firestore.GetModels(ctx, fsClient)
	if err != nil {
		log.Fatalf("Unable to get model information: %v\nHave you run setup-model?", err)
	}
	modelLookup := firestore.NewModelRefsBySystem(models, modelRefs)

	picks := make([]firestore.Pick, len(sgss))
	dogs := make([]dogPick, 0)
	for i, ss := range sgss {
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

		var modelChoice *fs.DocumentRef
		var gt gameType
		switch {
		case sgame.NoisySpread != 0:
			modelChoice = modelLookup[nsSystem]
			gt = noisySpread
		case sgame.Superdog:
			modelChoice = modelLookup[sdSystem]
			gt = superdog
		default:
			modelChoice = modelLookup[suSystem]
			gt = straightUp
		}

		pick, err := pickEm(ctx, fsClient, ss.Ref, perfs, modelChoice, gt, fallback)
		if err != nil {
			log.Fatalf("Unable to pick Game \"%s\": %v", gamess.Ref.Path, err)
		}
		if gt == superdog {
			// reverse superdog picks
			if pick.PickedTeam.ID == game.HomeTeam.ID {
				pick.PickedTeam = game.AwayTeam
			} else {
				pick.PickedTeam = game.HomeTeam
			}
			pick.PredictedProbability = 1 - pick.PredictedProbability
			dogs = append(dogs, dogPick{teamID: pick.PickedTeam.ID, points: sgame.Value, prob: pick.PredictedProbability})
		}
		picks[i] = pick
	}

	// Pick dog by unpicking undogs. Huh.
	sort.Sort(sort.Reverse(byValue(dogs)))
	unpickedDogs := make(map[string]struct{})
	for _, dog := range dogs[1:] {
		unpickedDogs[dog.teamID] = struct{}{}
	}
	for i, p := range picks {
		if _, ok := unpickedDogs[p.PickedTeam.ID]; ok {
			p.PickedTeam = nil
			picks[i] = p
		}
	}

	var sp *firestore.StreakPick
	s, spRef, err := firestore.GetStreakPredictions(ctx, weekRef, pkRef)
	if err != nil {
		if _, ok := err.(firestore.NoStreakPickError); !ok {
			log.Fatalf("Unable to lookup streak prediction for picker '%s': %v", picker, err)
		}
	} else if spRef != nil {
		sp = &firestore.StreakPick{
			PickedTeams:          s.BestPick,
			StreakPredictions:    spRef,
			PredictedSpread:      s.Spread,
			PredictedProbability: s.Probability,
			Picker:               pkRef,
		}
	}

	// write what I have
	if dryRun {
		log.Print("DRY RUN: would write the following to Firestore")
		for _, p := range picks {
			log.Printf("%s", p)
		}
		log.Printf("%+v", sp)
		return
	}

	picksCollection := weekRef.Collection(firestore.PICKS_COLLECTION)
	streaksCollection := weekRef.Collection(firestore.STEAK_PREDICTIONS_COLLECTION)

	err = fsClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {

		for _, pick := range picks {
			pick.Picker = pkRef
			pickRef := picksCollection.NewDoc()
			if force {
				err = t.Set(pickRef, &pick)
			} else {
				err = t.Create(pickRef, &pick)
			}
			if err != nil {
				return err
			}
		}

		if sp == nil {
			return err
		}
		spDoc := streaksCollection.NewDoc()
		if force {
			err = t.Set(spDoc, sp)
		} else {
			err = t.Create(spDoc, sp)
		}

		return err
	})

	// TODO: output Excel file for submission
	// TODO: write Excel file to Store?
}

type gameType int

const (
	straightUp gameType = iota
	noisySpread
	superdog
)

func pickEm(
	ctx context.Context,
	fsClient *fs.Client,
	sg *fs.DocumentRef,
	perfs []firestore.ModelPerformance,
	modelChoice *fs.DocumentRef,
	gt gameType,
	fallback bool) (firestore.Pick, error) {
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
		return p, fmt.Errorf("unable to get predictions for slate game \"%s\": %v", sgame.Game.Path, err)
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
		log.Printf("Using model %s to pick game %s", pred.Model.ID, ss.Ref.ID)
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
			p.FillOut(game, perf, pred, predRef, sgame.NoisySpread)
			return p, nil
		}
	}
	if !fallback {
		return p, fmt.Errorf("no predictions for game \"%s\" and no fallback specified", sgame.Game.Path)
	}

	// fallback onto best
	log.Printf("Using fallback model to pick game %s", ss.Ref.ID)
	pred, predRef, perf, err := getFallbackPrediction(ctx, predictions, predRefs, perfs, gt)
	if err != nil {
		return p, fmt.Errorf("unable to get fallback prediction: %v", err)
	}
	p.FillOut(game, perf, pred, predRef, sgame.NoisySpread)

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
	log.Printf("Fallback model chosen: %s", bestPred.Model.ID)

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

type dogPick struct {
	teamID string
	points int
	prob   float64
}

type byValue []dogPick

// Len implements Sortable interface
func (b byValue) Len() int {
	return len(b)
}

// Less implements Sortable interface
func (b byValue) Less(i, j int) bool {
	vi := b[i].prob * float64(b[i].points)
	vj := b[j].prob * float64(b[j].points)
	if vi == vj {
		return b[i].points < b[j].points
	}
	return vi < vj
}

// Swap implements Sortable interface
func (b byValue) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}
