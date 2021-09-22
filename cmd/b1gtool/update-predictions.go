package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	fs "cloud.google.com/go/firestore"
	edlib "github.com/hbollon/go-edlib"
	"github.com/reallyasi9/b1gpickem/firestore"
)

// The subcommand update-predictions scrapes the individual model predictions for each game of the week from https://www.thepredictiontracker.com/ncaapredictions.csv.
// Only a subset of games are predicted each week (games that have an opening Vegas line).

// upFlagSet is a flag.FlagSet for parsing the update-predictions subcommand.
var upFlagSet *flag.FlagSet

// predCSV is the URL for game predictions.
var predCSV string

// upUsage is the usage documentation for the update-predictions subcommand.
func upUsage() {
	fmt.Fprint(flag.CommandLine.Output(), `Usage: b1gtool [global-flags] update-predictions [flags] <season> <week>
	
Update predictions for games in Firestore. Downloads data from thepredictiontracker.com.
	
Arguments:
  season int
      Year of games being updated.
  week int
      Week of games being updated.

Flags:
`)

	ugFlagSet.PrintDefaults()

	fmt.Fprint(flag.CommandLine.Output(), "Global Flags:\n")

	flag.PrintDefaults()

}

func init() {
	upFlagSet = flag.NewFlagSet("update-predictions", flag.ExitOnError)
	upFlagSet.SetOutput(flag.CommandLine.Output())
	upFlagSet.Usage = upUsage

	upFlagSet.StringVar(&predCSV, "csv", "https://www.thepredictiontracker.com/ncaapredictions.csv", "URL or file name of CSV file containing model predictions.")
}

func updatePredictions() {
	err := upFlagSet.Parse(flag.Args()[1:])
	if err != nil {
		log.Fatalf("Failed to parse update-predictions arguments: %v", err)
	}

	if upFlagSet.NArg() != 2 {
		upFlagSet.Usage()
		log.Fatal("Season and week arguments not supplied")
	}
	year := upFlagSet.Arg(0) // technically, strings are okay
	week := upFlagSet.Arg(1)

	pt, err := newPredictionTable(predCSV)
	if err != nil {
		log.Fatalf("Failed to read prediction table from CSV '%s': %v", predCSV, err)
	}

	ctx := context.Background()
	fsClient, err := fs.NewClient(ctx, ProjectID)
	if err != nil {
		log.Fatalf("Failed to create firestore client: %v", err)
	}

	seasonRef := fsClient.Collection("seasons").Doc(year)
	teams, refs, err := firestore.GetTeams(ctx, fsClient, seasonRef)
	if err != nil {
		log.Fatalf("Failed to get teams: %v", err)
	}

	teamLookup := newTeamRefsByName(teams, refs)
	tps, err := pt.teamPairs(teamLookup)
	if err != nil {
		log.Fatalf("Failed to match teams to refs: %v", err)
	}

	weekRef := seasonRef.Collection("weeks").Doc(week)
	games, refs, err := firestore.GetGames(ctx, fsClient, weekRef)
	if err != nil {
		log.Fatalf("Failed to get games: %v", err)
	}

	gameLookup := newGameRefsByTeams(games, refs)
	predictions, err := pt.GetWritablePredictions(gameLookup, tps)
	if err != nil {
		log.Fatalf("Failed making writable predictions: %v", err)
	}

	if DryRun {
		log.Print("DRY RUN: would write the following:")
		log.Print(predictions)
		return
	}

	for i := 0; i < len(predictions); i += 500 {
		ul := i + 500
		if ul > len(predictions) {
			ul = len(predictions)
		}
		subset := predictions[i:ul]
		err = fsClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
			for _, rp := range subset {
				if Force {
					err := t.Set(rp.ref, &rp.pred)
					if err != nil {
						return err
					}
				} else {
					err := t.Create(rp.ref, &rp.pred)
					if err != nil {
						return err
					}
				}
			}
			return nil
		})

		if err != nil {
			log.Fatalf("Writing to firestore failed: %v", err)
		}
	}
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
		return nil, fmt.Errorf("error reading header from '%s': %v", f, err)
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
					return nil, fmt.Errorf("error parsing neutral site value '%s': %v", val, err)
				}
				neutral = append(neutral, neu)
			default:
				if !strings.HasPrefix(colname, "line") {
					continue
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

func request(client *http.Client, url string) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to do request: %v", err)
	}
	return resp.Body, nil
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

// TeamPair is a collection of a home and away team document ref.
type teamPair struct {
	Home    *fs.DocumentRef
	Away    *fs.DocumentRef
	neutral bool
}

func (pt *predictionTable) teamPairs(lookup *teamRefsByName) ([]teamPair, error) {
	tps := make([]teamPair, len(pt.homeTeams))
	for i := range pt.homeTeams {
		ht := pt.homeTeams[i]
		at := pt.awayTeams[i]

		href, possible, ok := lookup.Lookup(ht)
		if !ok {
			return nil, fmt.Errorf("no team matching home team '%s' in game %d: best matches are %v", ht, i, possible)
		}
		aref, possible, ok := lookup.Lookup(at)
		if !ok {
			return nil, fmt.Errorf("no team matching away team '%s' in game %d: best matches are %v", at, i, possible)
		}
		tps[i] = teamPair{Home: href, Away: aref, neutral: pt.neutral[i]}
	}
	return tps, nil
}

// teamRefsByName is a type for quick lookups of teams by name.
type teamRefsByName struct {
	names  []string
	byName map[string]*fs.DocumentRef
}

func newTeamRefsByName(teams []firestore.Team, refs []*fs.DocumentRef) *teamRefsByName {
	names := make([]string, 0, len(teams))
	byName := make(map[string]*fs.DocumentRef)
	for i, t := range teams {
		for _, n := range t.OtherNames {
			names = append(names, n)
			byName[n] = refs[i]
		}
	}
	return &teamRefsByName{
		names:  names,
		byName: byName,
	}
}

func (t *teamRefsByName) Lookup(name string) (*fs.DocumentRef, []string, bool) {
	if r, ok := t.byName[name]; ok {
		return r, nil, true
	}
	// find closest 3 team names by edit distance
	closest, err := edlib.FuzzySearchSet(name, t.names, 3, edlib.Jaccard)
	if err != nil {
		return nil, nil, false
	}
	return nil, closest, false
}

// gameRefsByTeams is a struct for quick lookups of games by home/away teams.
type gameRefsByTeams struct {
	homeTeams map[string]*fs.DocumentRef
	awayTeams map[string]*fs.DocumentRef
	neutral   map[*fs.DocumentRef]bool
}

func newGameRefsByTeams(games []firestore.Game, refs []*fs.DocumentRef) *gameRefsByTeams {
	homeTeams := make(map[string]*fs.DocumentRef)
	awayTeams := make(map[string]*fs.DocumentRef)
	neutral := make(map[*fs.DocumentRef]bool)
	for i, g := range games {
		homeTeams[g.HomeTeam.ID] = refs[i]
		awayTeams[g.AwayTeam.ID] = refs[i]
		neutral[refs[i]] = g.NeutralSite
	}
	return &gameRefsByTeams{
		homeTeams: homeTeams,
		awayTeams: awayTeams,
		neutral:   neutral,
	}
}

func (g *gameRefsByTeams) Lookup(tp teamPair) (game *fs.DocumentRef, swap bool, wrongNeutral bool, ok bool) {
	hgRef, hOK := g.homeTeams[tp.Home.ID]
	agRef, aOK := g.awayTeams[tp.Away.ID]
	swap = false
	if hOK && aOK && hgRef == agRef {
		game = hgRef
		ok = true
		wrongNeutral = g.neutral[hgRef] != tp.neutral
		return
	}
	if hOK && aOK && hgRef != agRef {
		game = nil
		ok = false
		return
	}

	// Try swapping home and away
	hgRef, hOK = g.awayTeams[tp.Home.ID]
	agRef, aOK = g.homeTeams[tp.Away.ID]
	swap = true
	if hOK && aOK && hgRef == agRef {
		game = hgRef
		ok = true
		wrongNeutral = g.neutral[hgRef] != tp.neutral
		return
	}

	return nil, false, false, false
}

type refPred struct {
	ref  *fs.DocumentRef
	pred firestore.ModelPrediction
}

func (pt *predictionTable) GetWritablePredictions(g *gameRefsByTeams, tps []teamPair) ([]refPred, error) {
	predictions := make([]refPred, 0)
	for i, tp := range tps {
		gref, swap, wrongNeutral, ok := g.Lookup(tp)
		homeRef := tp.Home
		awayRef := tp.Away
		if !ok {
			return nil, fmt.Errorf("failed to get game with matchup %s @ %s", tp.Away.ID, tp.Home.ID)
		}
		if swap {
			log.Printf("Game %s (%s @ %s) has home/away swapped between predictions and ground truth", gref.ID, tp.Home.ID, tp.Away.ID)
			homeRef, awayRef = awayRef, homeRef
		}
		if wrongNeutral {
			log.Printf("Game %s (%s v %s) has wrong neutral site flag (should be %t)", gref.ID, tp.Away.ID, tp.Home.ID, !tp.neutral)
		}
		// TODO: match model to prediction
		col := gref.Collection("predictions")
		for model := range pt.predictions {
			if pt.missing[model][i] {
				continue
			}
			p := pt.predictions[model][i]
			mp := firestore.ModelPrediction{
				Model:       nil, // TODO: lookup
				HomeTeam:    homeRef,
				AwayTeam:    awayRef,
				NeutralSite: tp.neutral != wrongNeutral, // TODO: lookup
				Spread:      p,
			}
			ref := col.Doc(model)
			predictions = append(predictions, refPred{ref: ref, pred: mp})
		}
	}
	return predictions, nil
}
