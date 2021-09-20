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

// The subcommand update-predictions scrapes both the performance to date of the various prediction models from https://www.thepredictiontracker.com/ncaaresults.php
// as well as the individual model predictions for each game of the week from https://www.thepredictiontracker.com/ncaapredictions.csv.
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
	// week := upFlagSet.Arg(1)

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

	fmt.Print(tps)
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
	Home *fs.DocumentRef
	Away *fs.DocumentRef
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
		tps[i] = teamPair{Home: href, Away: aref}
	}
	return tps, nil
}

// TeamRefsByName is a type for quick lookups of teams by name.
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
