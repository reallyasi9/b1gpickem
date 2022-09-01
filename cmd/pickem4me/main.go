package main

import (
	"context"
	"fmt"
	"log"
	"sort"

	fs "cloud.google.com/go/firestore"
	"github.com/alecthomas/kong"
	"github.com/reallyasi9/b1gpickem/internal/bts"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

type CLI struct {
	ProjectID        string `help:"GCP project ID." env:"GCP_PROJECT" required:""`
	StraightUpModel  string `help:"Model to use for straight-up games. Default fallback is to use the model with the most straight-up wins to date." short:"s"`
	NoisySpreadModel string `help:"Model to use for noisy-spread games. Default fallback is to use the straight-up model, otherwise the model with the smallest MAE to date." short:"n"`
	SuperdogModel    string `help:"Model to use for superdog games. Default fallback is to use the noisy-spread model, otherwise the model with the smallest MAE to date." short:"d"`
	Fallback         bool   `help:"Use fallback models when specified models are undefined."`
	Sagarin          bool   `help:"Use Sagarin points when all else fails."`
	DryRun           bool   `help:"Print intended writes to log and exit without updating the database."`
	Force            bool   `help:"Force overwrite data in the database."`
	Season           int    `arg:"" help:"Season year." required:""`
	Week             int    `arg:"" help:"Week number." required:""`
	Picker           string `help:"Picker (for selecting BTS team, if available)."`
}

func main() {
	var cli CLI
	ctx := kong.Parse(&cli)
	err := cli.Run()
	ctx.FatalIfErrorf(err)
}

func (cli CLI) Run() error {
	ctx := context.Background()
	fsClient, err := fs.NewClient(ctx, cli.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to create Firestore client: %w", err)
	}

	_, pkRef, err := firestore.GetPickerByLukeName(ctx, fsClient, cli.Picker)
	if err != nil {
		return fmt.Errorf("failed to lookup picker '%s': %w", cli.Picker, err)
	}
	log.Printf("Got picker %s from '%s'", pkRef.ID, cli.Picker)

	_, seasonRef, err := firestore.GetSeason(ctx, fsClient, cli.Season)
	if err != nil {
		return fmt.Errorf("failed to determine season from %d: %w", cli.Season, err)
	}
	log.Printf("Using season %s", seasonRef.ID)

	_, weekRef, err := firestore.GetWeek(ctx, seasonRef, cli.Week)
	if err != nil {
		return fmt.Errorf("failed to determine week from %d: %w", cli.Week, err)
	}
	log.Printf("Using week %s", weekRef.ID)

	slateSSs, err := weekRef.Collection(firestore.SLATES_COLLECTION).OrderBy("parsed", fs.Desc).Limit(1).Documents(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("failed to get most recent slate from Firestore: %w", err)
	}
	if len(slateSSs) < 1 {
		return fmt.Errorf("no slates found in Firestore for season %s, week %s: have you run `b1gtool slate parse` yet?", seasonRef.ID, weekRef.ID)
	}

	slateRef := slateSSs[0].Ref
	sgss, err := slateRef.Collection(firestore.SLATE_GAMES_COLLECTION).Documents(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("failed to get games from slate at path '%s': %w", slateRef.Path, err)
	}
	log.Printf("Read %d games from slate at path '%s'", len(sgss), slateRef.Path)

	perfs, _, err := firestore.GetMostRecentModelPerformances(ctx, fsClient, weekRef)
	if err != nil {
		return fmt.Errorf("failed to get model performances: %w\nHave you run `b1gtool models update` yet?", err)
	}

	models, modelRefs, err := firestore.GetModels(ctx, fsClient)
	if err != nil {
		return fmt.Errorf("failed to get model information: %w\nHave you run `b1gtool models setup` yet?", err)
	}
	modelLookup := firestore.NewModelRefsByShortName(models, modelRefs)

	picks := make([]firestore.Pick, len(sgss))
	dogs := make([]dogPick, 0)
	for i, ss := range sgss {
		var sgame firestore.SlateGame
		err = ss.DataTo(&sgame)
		if err != nil {
			return fmt.Errorf("failed to convert SlateGame at path '%s': %w", ss.Ref.Path, err)
		}

		gamess, err := sgame.Game.Get(ctx)
		if err != nil {
			return fmt.Errorf("failed to get game at path '%s': %w", sgame.Game.Path, err)
		}
		var game firestore.Game
		err = gamess.DataTo(&game)
		if err != nil {
			return fmt.Errorf("failed to convert Game at path '%s': %w", gamess.Ref.Path, err)
		}

		var modelChoice *fs.DocumentRef
		var gt gameType
		switch {
		case sgame.NoisySpread != 0:
			modelChoice = modelLookup[cli.NoisySpreadModel]
			gt = noisySpread
		case sgame.Superdog:
			modelChoice = modelLookup[cli.SuperdogModel]
			gt = superdog
		default:
			modelChoice = modelLookup[cli.StraightUpModel]
			gt = straightUp
		}

		pick, err := pickEm(ctx, fsClient, weekRef, ss.Ref, perfs, modelChoice, gt, cli.Fallback, cli.Sagarin)
		if err != nil {
			return fmt.Errorf("failed to pick Game '%s': %w", gamess.Ref.Path, err)
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
	if len(dogs) > 0 {
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
	}

	var sp *firestore.StreakPick
	s, spRef, err := firestore.GetStreakPredictions(ctx, weekRef, pkRef)
	if err != nil {
		if _, ok := err.(firestore.NoStreakPickError); !ok {
			return fmt.Errorf("failed to lookup streak prediction for picker '%s': %w", cli.Picker, err)
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
	if cli.DryRun {
		log.Print("DRY RUN: would write the following to Firestore")
		for _, p := range picks {
			p.Picker = pkRef
			log.Printf("%s", p)
		}
		log.Printf("Streak pick: %+v", sp)
		return nil
	}

	picksCollection := weekRef.Collection(firestore.PICKS_COLLECTION)
	streaksCollection := weekRef.Collection(firestore.STEAK_PREDICTIONS_COLLECTION)

	err = fsClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {

		for _, pick := range picks {
			pick.Picker = pkRef
			pickRef := picksCollection.NewDoc()
			if cli.Force {
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
		if cli.Force {
			err = t.Set(spDoc, sp)
		} else {
			err = t.Create(spDoc, sp)
		}

		return err
	})

	if err != nil {
		return fmt.Errorf("fialed running transaction: %w", err)
	}

	return nil
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
	weekRef,
	sg *fs.DocumentRef,
	perfs []firestore.ModelPerformance,
	modelChoice *fs.DocumentRef,
	gt gameType,
	fallback bool,
	sagarin bool,
) (firestore.Pick, error) {
	// TODO: fill out Pick from SlateGame
	p := firestore.Pick{
		SlateGame: sg,
	}

	ss, err := sg.Get(ctx)
	if err != nil {
		return p, fmt.Errorf("unable to get slate game '%s': %w", sg.Path, err)
	}
	var sgame firestore.SlateGame
	err = ss.DataTo(&sgame)
	if err != nil {
		return p, fmt.Errorf("unable to build SlateGame from '%s': %w", sg.Path, err)
	}

	predictions, predRefs, err := firestore.GetPredictions(ctx, fsClient, sgame.Game)
	if err != nil {
		return p, fmt.Errorf("unable to get predictions for slate game '%s': %w", sgame.Game.Path, err)
	}

	ss, err = sgame.Game.Get(ctx)
	if err != nil {
		return p, fmt.Errorf("unable to get game '%s': %w", sgame.Game.Path, err)
	}
	var game firestore.Game
	err = ss.DataTo(&game)
	if err != nil {
		return p, fmt.Errorf("unable to build Game from '%s': %w", sgame.Game.Path, err)
	}

	// If a specific choice is made, try to get that first.
	if modelChoice != nil {
		// simple linear search will do
		var pred firestore.ModelPrediction
		var nilPred firestore.ModelPrediction
		var iPred int
		for i, prd := range predictions {
			if prd.Model.ID == modelChoice.ID {
				iPred = i
				pred = prd
				break
			}
		}
		var perf firestore.ModelPerformance
		var nilPerf firestore.ModelPerformance
		for _, prf := range perfs {
			if prf.Model.ID == modelChoice.ID {
				perf = prf
				break
			}
		}
		// use the prediction to fill out the pick
		if pred != nilPred && perf != nilPerf {
			log.Printf("Using model %s to pick game %s", pred.Model.ID, ss.Ref.ID)
			p.FillOut(game, perf, pred, predRefs[iPred], sgame.NoisySpread)
			return p, nil
		}
	}
	if !fallback && !sagarin {
		return p, fmt.Errorf("no predictions for game '%s' and no fallback specified", sgame.Game.Path)
	}

	// fallback onto best
	var pred firestore.ModelPrediction
	var predRef *fs.DocumentRef
	var perf firestore.ModelPerformance
	if fallback {
		log.Printf("Using fallback model to pick game %s", ss.Ref.ID)
		pred, predRef, perf, err = getFallbackPrediction(ctx, predictions, predRefs, perfs, gt)
	}
	if err != nil && !sagarin {
		return p, fmt.Errorf("unable to get fallback prediction: %w", err)
	}
	if err != nil || sagarin {
		// fallback fallback to Sagarin points
		log.Printf("Using Sagarin points to pick game %s", ss.Ref.ID)
		pred, predRef, perf, err = getSagarinPrediction(ctx, fsClient, weekRef, perfs, game)
	}
	if err != nil {
		return p, fmt.Errorf("unable to get fallback or Sagarin prediction: %w", err)
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

var sagarinRatings map[string]firestore.ModelTeamPoints

func getSagarinPrediction(ctx context.Context, fsClient *fs.Client, weekRef *fs.DocumentRef, performances []firestore.ModelPerformance, game firestore.Game) (bestPred firestore.ModelPrediction, predRef *fs.DocumentRef, bestPerf firestore.ModelPerformance, err error) {
	// Get most recent Sagarin Ratings proper
	// I can cheat because I know this already.
	if len(sagarinRatings) == 0 {
		sagarinRatings = make(map[string]firestore.ModelTeamPoints)
		sagPointsRef := weekRef.Collection("team-points").Doc("sagarin")
		sagSnaps, e := sagPointsRef.Collection("linesag").Documents(ctx).GetAll()
		if e != nil {
			err = fmt.Errorf("getSagarinPrediction: unable to get sagarin ratings: %w", e)
			return
		}
		for _, s := range sagSnaps {
			var sag firestore.ModelTeamPoints
			err = s.DataTo(&sag)
			if err != nil {
				err = fmt.Errorf("getSagarinPrediction: unable to get sagarin rating: %w", err)
				return
			}
			// Sagarin has one nil team representing a non-recorded team. Don't keep that one.
			if sag.Team == nil {
				continue
			}
			sagarinRatings[sag.Team.ID] = sag
		}
		log.Printf("Sagarin ratings filled (first time)")
	} else {
		log.Printf("Sagarin ratings already filled")
	}

	var sagPerf firestore.ModelPerformance
	sagPerfFound := false
	for _, perf := range performances {
		if perf.Model.ID == "linesag" {
			sagPerf = perf
			sagPerfFound = true
			break
		}
	}
	if !sagPerfFound {
		err = fmt.Errorf("getSagarinPrediction: unable to retrieve most recent Sagarin performance for the week")
		return
	}

	// Build the probability model
	model := bts.NewGaussianSpreadModel(sagarinRatings, sagPerf)

	// Fake a game and predict it
	location := bts.Home
	if game.NeutralSite {
		location = bts.Neutral
	}
	btsGame := bts.NewGame(bts.Team(game.HomeTeam.ID), bts.Team(game.AwayTeam.ID), location)
	_, _, spread := model.MostLikelyOutcome(btsGame)

	// fake the model prediction
	bestPred.AwayTeam = game.AwayTeam
	bestPred.HomeTeam = game.HomeTeam
	bestPred.Model = sagPerf.Model
	bestPred.NeutralSite = game.NeutralSite
	bestPred.Spread = spread

	bestPerf = sagPerf

	predRef = nil

	err = nil

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
