// The subcommand update-sagarin scrapes the team Sagarin ratings from https://sagarin.com/sports/cfsend.htm.
package updatemodels

import (
	"context"
	"crypto/tls"
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
	"github.com/reallyasi9/b1gpickem/internal/firestore"
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
var unrankedRE = regexp.MustCompile(`(?i)[_\*]{3}UNRATED[_\*]{3}.*?` +
	`<font color="` + ratingColor + `">\s*([\-0-9\.]+).*?` + // rating
	`<font color="` + predictorColor + `">\s*([\-0-9\.]+).*?` + // predictor
	`<font color="` + goldenColor + `">\s*([\-0-9\.]+).*?` + // golden
	`<font color="` + recentColor + `">\s*([\-0-9\.]+)`) // recent

// sagURL is the URL or file name of the file containing Sagarin ratings.
const SAG_URL = "https://sagarin.com/sports/cfsend.htm"

func UpdateSagarin(ctx *Context) error {
	year := strconv.Itoa(ctx.Season)
	week := strconv.Itoa(ctx.Week)

	seasonRef := ctx.FirestoreClient.Collection(firestore.SEASONS_COLLECTION).Doc(year)
	teams, refs, err := firestore.GetTeams(ctx, seasonRef)
	if err != nil {
		return fmt.Errorf("GetPredictions: Failed to get teams: %w", err)
	}
	teamLookup, err := firestore.NewTeamRefsByOtherName(teams, refs)
	if err != nil {
		panic(err)
	}

	models, refs, err := firestore.GetModels(ctx, ctx.FirestoreClient)
	if err != nil {
		return fmt.Errorf("GetPredictions: Failed to get models: %w", err)
	}
	modelLookup := firestore.NewModelRefsByShortName(models, refs)

	// Get the four Sagarin models in order
	modelNames := []string{"linesag", "linesagpred", "linesaggm", "linesagr"}
	modelRefs := make([]*fs.DocumentRef, 4)
	for i, n := range modelNames {
		var ok bool
		if modelRefs[i], ok = modelLookup[n]; !ok {
			return fmt.Errorf("GetPredictions: Failed to find reference to model \"%s\"", n)
		}
	}

	sagTable, err := parseSagarinTable(SAG_URL, teamLookup, modelRefs)
	if err != nil {
		return fmt.Errorf("GetPredictions: Failed to create Sagarin table: %w", err)
	}

	// Begin writing
	if ctx.DryRun {
		log.Printf("DRY RUN: Would write the following to Firestore:")
		for _, s := range sagTable {
			log.Printf("%v", s)
		}
		return nil
	}

	weekRef := seasonRef.Collection("weeks").Doc(week)
	pointsRef := weekRef.Collection("team-points").Doc("sagarin")
	// TODO: move to firestore?
	type timestamped struct {
		Timestamp time.Time `firestore:"timestamp,serverTimestamp"`
	}
	ts := timestamped{}
	if ctx.Force {
		_, err = pointsRef.Set(ctx, &ts)
	} else {
		_, err = pointsRef.Create(ctx, &ts)
	}
	if err != nil {
		return fmt.Errorf("GetPredictions: Failed to create timestamped points document: %w", err)
	}

	for model, ele := range sagTable {

		err = ctx.FirestoreClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
			for _, s := range ele {
				var ref *fs.DocumentRef
				if s.Team == nil {
					ref = pointsRef.Collection(model).Doc("UNKNOWN")
				} else {
					ref = pointsRef.Collection(model).Doc(s.Team.ID)
				}
				if ctx.Force {
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
			return fmt.Errorf("GetPredictions: Failed to write transaction: %w", err)
		}
	}
	return nil
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
		return nil, fmt.Errorf("parseSagarinTable: cannot read body from \"%s\": %w", f, err)
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
			return nil, fmt.Errorf("parseSagarinTable: cannot parse home advantage string \"%s\" as float: %w", homeMatches[i+1], err)
		}
	}

	ratings := make(map[string]sagarinElement)
	for i := 0; i < 4; i++ {
		m := modelRefs[i]
		ratings[m.ID] = make(sagarinElement, 0, len(teamMatches)+1)
	}

	seenTeams := make(map[string]struct{}) // stop if already seen.
	for _, match := range teamMatches {
		name := match[1]
		if _, ok := seenTeams[name]; ok {
			break // names are duplicated
		}
		seenTeams[name] = struct{}{}

		teamRef, exists := lookup[name]
		if !exists {
			log.Printf("Warning: team \"%s\" not found in teams. Make sure they only have FBS teams on their schedule!", name)
			continue
		}

		for j := 0; j < 4; j++ {
			rating, err := strconv.ParseFloat(match[j+2], 64) // name also here
			if err != nil {
				return nil, fmt.Errorf("parseSagarinTable: cannot parse rating string \"%s\" as float: %w", match[j+2], err)
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

	for j := 0; j < 4; j++ {
		rating, err := strconv.ParseFloat(unrankedMatches[j+1], 64) // unranked have no name
		if err != nil {
			return nil, fmt.Errorf("parseSagarinTable: cannot parse unranked rating string \"%s\" as float: %w", unrankedMatches[j], err)
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
