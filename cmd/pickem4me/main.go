package main

import (
	"context"
	"errors"
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
	StraightUpModel  string `help:"Model to use for straight-up games. Default fallback is to use the model with the most straight-up wins to date, otherwise uses Sagarin scores." short:"s"`
	NoisySpreadModel string `help:"Model to use for noisy-spread games. Default fallback is to use the straight-up model, otherwise the model with the smallest MAE to date." short:"n"`
	SuperdogModel    string `help:"Model to use for superdog games. Default fallback is to use the noisy-spread model, otherwise the model with the smallest MAE to date." short:"d"`
	Fallback         bool   `help:"Use fallback models when specified models are undefined."`
	DryRun           bool   `help:"Print intended writes to log and exit without updating the database."`
	Force            bool   `help:"Force overwrite data in the database."`
	Season           int    `arg:"" help:"Season year." required:""`
	Week             int    `arg:"" help:"Week number." required:""`
	Picker           string `arg:"" help:"Picker who is making picks." required:""`
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

	games, gameRefs, err := firestore.GetGames(ctx, weekRef)
	if err != nil {
		return fmt.Errorf("failed to get all games from week %s: %w", weekRef.ID, err)
	}
	gamesByID := make(map[string]firestore.Game)
	gameRefsByID := make(map[string]*fs.DocumentRef)
	for i, ref := range gameRefs {
		gamesByID[ref.ID] = games[i]
		gameRefsByID[ref.ID] = ref
	}

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

	perfs, perfRefs, err := firestore.GetMostRecentModelPerformances(ctx, fsClient, weekRef)
	if err != nil {
		return fmt.Errorf("failed to get model performances: %w\nHave you run `b1gtool models update` yet?", err)
	}

	// Get most recent Sagarin Ratings for backup
	// I can cheat because I know this already.
	sagPointsRef := weekRef.Collection("team-points").Doc("sagarin")
	sagSnaps, err := sagPointsRef.Collection("linesag").Documents(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("unable to get sagarin ratings: %w", err)
	}
	sagarinRatings := make(map[string]firestore.ModelTeamPoints)
	for _, s := range sagSnaps {
		var sag firestore.ModelTeamPoints
		err = s.DataTo(&sag)
		if err != nil {
			return fmt.Errorf("unable to get sagarin rating: %w", err)
		}
		// Sagarin has one nil team representing a non-recorded team. Don't keep that one.
		if sag.Team == nil {
			continue
		}
		sagarinRatings[sag.Team.ID] = sag
	}

	var sagPerf *firestore.ModelPerformance
	for _, perf := range perfs {
		if perf.Model.ID == "linesag" {
			sagPerf = &perf
			break
		}
	}
	if sagPerf == nil {
		return fmt.Errorf("unable to find most recent Sagarin performance for the week")
	}

	// Build the probability model
	model := bts.NewGaussianSpreadModel(sagarinRatings, *sagPerf)
	log.Printf("Built Sagarin fallback model %v", model)

	picks := make([]*firestore.Pick, len(sgss))
	dogs := make([]DogPick, 0)
	for i, ss := range sgss {
		var sgame firestore.SlateGame
		err = ss.DataTo(&sgame)
		if err != nil {
			return fmt.Errorf("failed to convert SlateGame at path '%s': %w", ss.Ref.Path, err)
		}

		game, ok := gamesByID[sgame.Game.ID]
		if !ok {
			return fmt.Errorf("slate game refers to game at path '%s', which is not present in week %s", sgame.Game.Path, weekRef.ID)
		}

		gp, err := NewGamePredictions(ctx, game, sgame, ss.Ref, perfs, perfRefs)
		if err != nil {
			return fmt.Errorf("unable to make game predictions lookup object: %w", err)
		}

		var preferredModel string
		var gt gameType
		switch {
		case sgame.NoisySpread != 0:
			preferredModel = cli.NoisySpreadModel
			gt = noisySpread
		case sgame.Superdog:
			preferredModel = cli.SuperdogModel
			gt = superdog
		default:
			preferredModel = cli.StraightUpModel
			gt = straightUp
		}

		pick, err := gp.Pick(preferredModel)
		var nfErr ModelNotFoundError
		if errors.As(err, &nfErr) && cli.Fallback {
			pick, err = gp.Fallback(model)
		}
		if err != nil {
			return fmt.Errorf("unable to make pick of slate game %s: %w", sgame, err)
		}

		if gt == superdog {
			// reverse superdog picks
			if pick.PickedTeam.ID == game.HomeTeam.ID {
				pick.PickedTeam = game.AwayTeam
			} else {
				pick.PickedTeam = game.HomeTeam
			}
			pick.PredictedProbability = 1 - pick.PredictedProbability
			dogs = append(dogs, DogPick{teamID: pick.PickedTeam.ID, points: sgame.Value, prob: pick.PredictedProbability})
		}
		picks[i] = pick
	}

	// Pick dog by unpicking undogs. Huh.
	if len(dogs) > 0 {
		sort.Sort(sort.Reverse(ByValue(dogs)))
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
	s, spRef, err := firestore.GetMostRecentStreakPrediction(ctx, weekRef, pkRef)
	var nspErr firestore.NoStreakPickError
	if err != nil && !errors.As(err, &nspErr) {
		return fmt.Errorf("failed to lookup streak prediction for picker '%s': %w", cli.Picker, err)
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
			log.Printf("%s", p)
		}
		log.Printf("Streak pick: %+v", sp)
		return nil
	}

	picksCollection := weekRef.Collection(firestore.PICKS_COLLECTION)
	streaksCollection := weekRef.Collection(firestore.STREAK_PICKS_COLLECTION)

	err = fsClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {

		for _, pick := range picks {
			pick.Picker = pkRef
			pickRef := picksCollection.NewDoc()
			if cli.Force {
				err = t.Set(pickRef, pick)
			} else {
				err = t.Create(pickRef, pick)
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
		return fmt.Errorf("failed running transaction: %w", err)
	}

	return nil
}

type gameType int

const (
	straightUp gameType = iota
	noisySpread
	superdog
)
