// The subcommand update-sagarin scrapes the team Sagarin ratings from https://sagarin.com/sports/cfsend.htm.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"time"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/firestore"
)

const ratingColor = "#9900ff"
const predictorColor = "#0000ff"
const goldenColor = "#bb0000"
const recentColor = "#006B3C"

// homeAdvRE parses Sagarin output for the home advantage line.
// Order: RATING, POINTS, GOLDEN_MEAN, RECENT
var homeAdvRE = regexp.MustCompile(`(?i)` +
	`\[<font color="` + ratingColor + `">\s*([\-0-9\.]+)</font>\].*?` + // rating
	`\[<font color="` + predictorColor + `">\s*([\-0-9\.]+)</font>\].*?` + // predictor
	`\[<font color="` + goldenColor + `">\s*([\-0-9\.]+)</font>\].*?` + // golden
	`\[<font color="` + recentColor + `">\s*([\-0-9\.]+)</font>\].*?`) // recent

// ratingsRE parses Sagarin output for each team's rating.
var ratingsRE = regexp.MustCompile(`(?i)<font color="#000000">\s*` +
	`\d+\s+` + // rank
	`(.*?)\s+` + // name
	`[A]+\s*=</font>` + // league
	`<font color="` + ratingColor + `">\s*([\-0-9\.]+).*?` + // rating
	`<font color="` + predictorColor + `">\s*([\-0-9\.]+).*?` + // predictor
	`<font color="` + goldenColor + `">\s*([\-0-9\.]+).*?` + // golden
	`<font color="` + recentColor + `">\s*([\-0-9\.]+)`) // recent

// unrankedRE grabs the unranked team points (should be -91, but just in case...)
var unrankedRE = regexp.MustCompile(`(?i)___UNRATED___.*?` +
	`<font color="` + ratingColor + `">\s*([\-0-9\.]+).*?` + // rating
	`<font color="` + predictorColor + `">\s*([\-0-9\.]+).*?` + // predictor
	`<font color="` + goldenColor + `">\s*([\-0-9\.]+).*?` + // golden
	`<font color="` + recentColor + `">\s*([\-0-9\.]+)`) // recent

// usFlagSet is a flag.FlagSet for parsing the update-sagarin subcommand.
var usFlagSet *flag.FlagSet

// sagURL is the URL or file name of the file containing Sagarin ratings.
var sagURL string

// usUsage is the usage documentation for the update-sagarin subcommand.
func usUsage() {
	fmt.Fprint(flag.CommandLine.Output(), `Usage: b1gtool [global-flags] update-sagarin [flags] <season> <week>
	
Update Sagarin team ratings and game outcome predictions.
	
Arguments:
  season int
      Year of games being updated.
  week int
      Week of games being updated.

Flags:
`)

	usFlagSet.PrintDefaults()

	fmt.Fprint(flag.CommandLine.Output(), "\nGlobal Flags:\n")

	flag.PrintDefaults()

}

func init() {
	Commands["update-sagarin"] = updateSagarin
	Usage["update-sagarin"] = usUsage

	usFlagSet = flag.NewFlagSet("update-sagarin", flag.ExitOnError)
	usFlagSet.SetOutput(flag.CommandLine.Output())
	usFlagSet.Usage = usUsage

	usFlagSet.StringVar(&sagURL, "sagarin", "https://sagarin.com/sports/cfsend.htm", "URL or file name of file containing Sagarin team ratings.")
}

func updateSagarin() {
	err := usFlagSet.Parse(flag.Args()[1:])
	if err != nil {
		log.Fatalf("Failed to parse update-sagarin arguments: %v", err)
	}

	if usFlagSet.NArg() != 2 {
		usFlagSet.Usage()
		log.Fatal("Season and week arguments not supplied")
	}
	year := usFlagSet.Arg(0) // technically, strings are okay
	week := usFlagSet.Arg(1)

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
	teamLookup := firestore.NewTeamRefsByOtherName(teams, refs)

	models, refs, err := firestore.GetModels(ctx, fsClient)
	if err != nil {
		log.Fatalf("Failed to get models: %v", err)
	}
	modelLookup := firestore.NewModelRefsByShortName(models, refs)

	// Get the four Sagarin models in order
	modelNames := []string{"linesag", "linesagpred", "linesaggm", "linesagr"}
	modelRefs := make([]*fs.DocumentRef, 4)
	for i, n := range modelNames {
		var ok bool
		if modelRefs[i], ok = modelLookup[n]; !ok {
			log.Fatalf("Failed to find reference to model \"%s\"", n)
		}
	}

	sagTable, err := parseSagarinTable(sagURL, teamLookup, modelRefs)
	if err != nil {
		log.Fatalf("Failed to create Sagarin table: %v", err)
	}

	// Begin writing
	if DryRun {
		log.Printf("DRY RUN: Would write the following to Firestore:")
		for _, s := range sagTable {
			log.Printf("%v", s)
		}
		return
	}

	now := time.Now()
	weekRef := seasonRef.Collection("weeks").Doc(week)
	pointsRef := weekRef.Collection("team_points").Doc(now.Format(time.RFC3339))
	// TODO: move to firestore?
	type timestamped struct {
		Timestamp time.Time `firestore:"timestamp"`
	}
	ts := timestamped{Timestamp: now}
	if Force {
		_, err = pointsRef.Set(ctx, &ts)
	} else {
		_, err = pointsRef.Create(ctx, &ts)
	}
	if err != nil {
		log.Fatalf("Failed to create timestamped points document: %v", err)
	}

	for model, ele := range sagTable {

		err = fsClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
			for _, s := range ele {
				var ref *fs.DocumentRef
				if s.Team == nil {
					ref = pointsRef.Collection(model).Doc("UNKNOWN")
				} else {
					ref = pointsRef.Collection(model).Doc(s.Team.ID)
				}
				if Force {
					err = t.Set(ref, &s)
				} else {
					err = t.Create(ref, &s)
				}
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			log.Fatalf("Failed to write transaction: %v", err)
		}
	}

}

type sagarinElement []firestore.ModelTeamPoints

// parseSagarinTable parses the table provided by Sagarin for each team.
func parseSagarinTable(f string, lookup firestore.TeamRefsByName, modelRefs []*fs.DocumentRef) (map[string]sagarinElement, error) {
	var rc io.ReadCloser
	if _, err := url.Parse(f); err == nil {
		// <sigh> Oh Sagarin...
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		httpClient := &http.Client{Transport: tr}
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

	content, err := ioutil.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("parseSagarinTable: cannot read body from \"%s\": %v", f, err)
	}

	bodyString := string(content)

	homeMatches := homeAdvRE.FindStringSubmatch(bodyString)
	if homeMatches == nil {
		return nil, fmt.Errorf("parseSagarinTable: cannot find home advantage line in \"%s\"", f)
	}

	teamMatches := ratingsRE.FindAllStringSubmatch(bodyString, -1)
	if teamMatches == nil {
		return nil, fmt.Errorf("parseSagarinTable: cannot find team lines in \"%s\"", f)
	}

	unrankedMatches := unrankedRE.FindStringSubmatch(bodyString)
	if unrankedMatches == nil {
		return nil, fmt.Errorf("parseSagarinTable: cannot find unranked team line in \"%s\"", f)
	}

	advantages := make([]float64, 4)
	for i := 0; i < 4; i++ {
		var err error
		advantages[i], err = strconv.ParseFloat(homeMatches[i+1], 64)
		if err != nil {
			return nil, fmt.Errorf("parseSagarinTable: cannot parse home advantage string \"%s\" as float: %v", homeMatches[i+1], err)
		}
	}

	ratings := make(map[string]sagarinElement)
	for i := 0; i < 4; i++ {
		m := modelRefs[i]
		ratings[m.ID] = make(sagarinElement, 0, len(teamMatches)+1)
	}

	seenTeams := make(map[string]struct{}) // stop if already seen.
	var teamNotFoundErr error
	for _, match := range teamMatches {
		name := match[1]
		if _, ok := seenTeams[name]; ok {
			break // names are duplicated
		}
		seenTeams[name] = struct{}{}

		teamRef, exists := lookup[name]
		if !exists {
			teamNotFoundErr = fmt.Errorf("parseSagarinTable: last team not found: \"%s\"", name)
			log.Printf("Team \"%s\" not found in teams", name)
		}

		for j := 0; j < 4; j++ {
			rating, err := strconv.ParseFloat(match[j+2], 64) // name also here
			if err != nil {
				return nil, fmt.Errorf("parseSagarinTable: cannot parse rating string \"%s\" as float: %v", match[j+2], err)
			}
			m := modelRefs[j]
			tr := firestore.ModelTeamPoints{
				Model:         m,
				Team:          teamRef,
				Points:        rating,
				HomeAdvantage: advantages[j],
			}
			ratings[m.ID] = append(ratings[m.ID], tr)
		}
	}
	if teamNotFoundErr != nil {
		return nil, teamNotFoundErr
	}

	for j := 0; j < 4; j++ {
		rating, err := strconv.ParseFloat(unrankedMatches[j+1], 64) // unranked have no name
		if err != nil {
			return nil, fmt.Errorf("parseSagarinTable: cannot parse unranked rating string \"%s\" as float: %v", unrankedMatches[j], err)
		}
		m := modelRefs[j]
		tr := firestore.ModelTeamPoints{
			Model:         m,
			Team:          nil,
			Points:        rating,
			HomeAdvantage: advantages[j],
		}
		ratings[m.ID] = append(ratings[m.ID], tr)
	}

	return ratings, nil
}
