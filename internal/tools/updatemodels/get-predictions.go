package updatemodels

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

const PRED_CSV = "https://www.thepredictiontracker.com/ncaapredictions.csv"

func GetPredictions(ctx *Context) error {

	pt, err := newPredictionTable(PRED_CSV)
	if err != nil {
		return fmt.Errorf("GetPredictions: Failed to read prediction table from CSV '%s': %w", PRED_CSV, err)
	}

	year := strconv.Itoa(ctx.Season)
	seasonRef := ctx.FirestoreClient.Collection(firestore.SEASONS_COLLECTION).Doc(year)
	teams, refs, err := firestore.GetTeams(ctx, seasonRef)
	if err != nil {
		return fmt.Errorf("GetPredictions: Failed to get teams: %w", err)
	}

	teamLookup, err2 := firestore.NewTeamRefsByOtherName(teams, refs)
	if err2 != nil {
		panic(err2)
	}
	tps, err := pt.Matchups(teamLookup)
	if err != nil {
		return fmt.Errorf("GetPredictions: Failed to match teams to refs: %w", err)
	}

	week := strconv.Itoa(ctx.Week)
	weekRef := seasonRef.Collection(firestore.WEEKS_COLLECTION).Doc(week)
	games, grefs, err := firestore.GetGames(ctx, weekRef)
	if err != nil {
		return fmt.Errorf("GetPredictions: Failed to get games: %w", err)
	}

	models, mrefs, err := firestore.GetModels(ctx, ctx.FirestoreClient)
	if err != nil {
		return fmt.Errorf("GetPredictions: Failed to get models: %w", err)
	}

	modelLookup := firestore.NewModelRefsByShortName(models, mrefs)
	gameLookup := firestore.NewGameRefsByMatchup(games, grefs)
	predictions, err := pt.GetWritablePredictions(gameLookup, modelLookup, tps, seasonRef)
	if err != nil {
		return fmt.Errorf("GetPredictions: Failed making writable predictions: %w", err)
	}

	if ctx.DryRun {
		log.Print("DRY RUN: would write the following:")
		log.Print(predictions)
		return nil
	}

	batch := ctx.FirestoreClient.Batch()
	for i := 0; i < len(predictions); i += 500 {
		ul := i + 500
		if ul > len(predictions) {
			ul = len(predictions)
		}
		subset := predictions[i:ul]
		// err = ctx.FirestoreClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
		for _, rp := range subset {
			if ctx.Force {
				// err := t.Set(rp.ref, &rp.pred)
				batch.Set(rp.ref, &rp.pred)
			} else {
				batch.Create(rp.ref, &rp.pred)
			}
		}
		_, err = batch.Commit(ctx)

		if err != nil {
			return fmt.Errorf("GetPredictions: Writing batch commit to firestore failed: %w", err)
		}
	}

	return nil
}

// predictionTable collects the predictions for a set of models in a nice format.
type predictionTable struct {
	homeTeams   []string
	awayTeams   []string
	neutral     []bool
	predictions map[string][]float64
	missing     map[string][]bool
}

func newPredictionTable(f string) (*predictionTable, error) {
	var rc io.ReadCloser
	if _, err := url.Parse(f); err == nil {
		httpClient := http.DefaultClient
		var err error
		rc, err = request(httpClient, f)
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		rc, err = os.Open(f)
		if err != nil {
			return nil, err
		}
	}
	defer rc.Close()
	csvr := csv.NewReader(rc)

	record, err := csvr.Read()
	if err != nil {
		return nil, fmt.Errorf("error reading header from '%s': %w", f, err)
	}
	header, err := headerMap(record)
	if err != nil {
		return nil, err
	}
	homeTeams := make([]string, 0)
	awayTeams := make([]string, 0)
	neutral := make([]bool, 0)
	predictions := make(map[string][]float64)
	missing := make(map[string][]bool)
	for {
		record, err := csvr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		for colname, which := range header {
			val := record[which]
			switch colname {
			case "home":
				homeTeams = append(homeTeams, val)
			case "road":
				awayTeams = append(awayTeams, val)
			case "neutral":
				neu, err := strconv.ParseBool(val)
				if err != nil {
					return nil, fmt.Errorf("error parsing neutral site value '%s': %w", val, err)
				}
				neutral = append(neutral, neu)
			default:
				if !strings.HasPrefix(colname, "line") {
					continue
				}
				if colname == "linestd" {
					continue // not a real prediction: just the standard deviation
				}
				f := 0.
				m := true
				if val != "" {
					m = false
					f, err = strconv.ParseFloat(val, 64)
				}
				if err != nil {
					return nil, err
				}
				predictions[colname] = append(predictions[colname], f)
				missing[colname] = append(missing[colname], m)
			}
		}
	}
	return &predictionTable{
		homeTeams:   homeTeams,
		awayTeams:   awayTeams,
		neutral:     neutral,
		predictions: predictions,
		missing:     missing,
	}, nil
}

func headerMap(record []string) (map[string]int, error) {
	out := make(map[string]int)
	for i, s := range record {
		if j, ok := out[s]; ok {
			return nil, fmt.Errorf("header '%s' repeated in columns %d and %d", s, i, j)
		}
		out[s] = i
	}
	return out, nil
}

func (pt *predictionTable) Matchups(lookup firestore.TeamRefsByName) ([]firestore.Matchup, error) {
	tps := make([]firestore.Matchup, len(pt.homeTeams))
	for i := range pt.homeTeams {
		ht := pt.homeTeams[i]
		at := pt.awayTeams[i]

		href, ok := lookup[ht]
		if !ok {
			return nil, fmt.Errorf("no team matching home team '%s' in game %d", ht, i)
		}
		aref, ok := lookup[at]
		if !ok {
			return nil, fmt.Errorf("no team matching away team '%s' in game %d", at, i)
		}
		tps[i] = firestore.Matchup{Home: href.ID, Away: aref.ID, Neutral: pt.neutral[i]}
	}
	return tps, nil
}

type refPred struct {
	ref  *fs.DocumentRef
	pred firestore.ModelPrediction
}

func (pt *predictionTable) GetWritablePredictions(g firestore.GameRefsByMatchup, modelLookup firestore.ModelRefsByName, ms []firestore.Matchup, season *fs.DocumentRef) ([]refPred, error) {
	predictions := make([]refPred, 0)
	seen := make(map[firestore.Matchup]struct{})
	for i, tp := range ms {
		gref, swap, wrongNeutral, ok := g.LookupCorrectMatchup(tp)
		homeID := tp.Home
		awayID := tp.Away
		if !ok {
			return nil, fmt.Errorf("failed to get game with matchup %s @ %s", tp.Away, tp.Home)
		}
		if swap {
			log.Printf("Game %s (%s @ %s) has home/away swapped between predictions and ground truth", gref.ID, tp.Home, tp.Away)
			homeID, awayID = awayID, homeID
		}
		if wrongNeutral {
			log.Printf("Game %s (%s v %s) has wrong neutral site flag (should be %t)", gref.ID, tp.Away, tp.Home, !tp.Neutral)
		}
		mu := firestore.Matchup{Home: homeID, Away: awayID, Neutral: tp.Neutral}
		if _, ok := seen[mu]; ok {
			log.Printf("Game %s (%s v %s) has already been seen!", gref.ID, tp.Away, tp.Home)
			continue
		}
		seen[mu] = struct{}{}

		col := gref.Collection(firestore.PREDICTIONS_COLLECTION)
		for model := range pt.predictions {
			if pt.missing[model][i] {
				continue
			}
			mref, ok := modelLookup[model]
			if !ok {
				return nil, fmt.Errorf("failed to get model with name \"%s\"", model)
			}
			p := pt.predictions[model][i]
			mp := firestore.ModelPrediction{
				Model:       mref,
				HomeTeam:    season.Collection(firestore.TEAMS_COLLECTION).Doc(homeID),
				AwayTeam:    season.Collection(firestore.TEAMS_COLLECTION).Doc(awayID),
				NeutralSite: tp.Neutral != wrongNeutral,
				Spread:      p,
			}
			ref := col.Doc(model)
			predictions = append(predictions, refPred{ref: ref, pred: mp})
		}
	}
	return predictions, nil
}
