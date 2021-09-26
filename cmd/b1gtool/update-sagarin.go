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

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/firestore"
)

const ratingColor = "#9900ff"
const predictorColor = "#0000ff"
const goldenColor = "#bb0000"
const recentColor = "#006B3C"

// homeAdvRE parses Sagarin output for the home advantage line.
// Order: RATING, POINTS, GOLDEN_MEAN, RECENT
var homeAdvRE = regexp.MustCompile(`(?i)(?:HOME ADVANTAGE=\[<font color="(#[0-9a-f]{6})">\s*([\-0-9.]+)</font>\]\s*){4}`)

// ratingsRE parses Sagarin output for each team's rating.
var ratingsRE = regexp.MustCompile(`(?i)<font color="#000000">\s*` +
	`\d+\s+` + // rank
	`(.*?)\s+` + // name
	`[A]+\s*=</font>` + // league
	`(?:<font color="(#[0-9a-f]{6})">\s*([\-0-9.]+)</font>\s*){4}`) // RECENT

// The subcommand update-sagarin scrapes the team Sagarin ratings from https://sagarin.com/sports/cfsend.htm.

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
	// week := usFlagSet.Arg(1)

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
	sagTable, err := parseSagarinTable(sagURL, teamLookup)
	if err != nil {
		log.Fatalf("Failed to create Sagarin table: %v", err)
	}

	fmt.Println(sagTable)

	// weekRef := seasonRef.Collection("weeks").Doc(week)
	// games, refs, err := firestore.GetGames(ctx, fsClient, weekRef)
	// if err != nil {
	// 	log.Fatalf("Failed to get games: %v", err)
	// }

	// gameLookup := newGameRefsByTeams(games, refs)
	// predictions, err := pt.GetWritablePredictions(gameLookup, tps)
	// if err != nil {
	// 	log.Fatalf("Failed making writable predictions: %v", err)
	// }
}

// parseSagarinTable parses the table provided by Sagarin for each team.
func parseSagarinTable(f string, lookup *teamRefsByName) ([]firestore.ModelTeamPoints, error) {
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

	// now := time.Now()

	content, err := ioutil.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("parseSagarinTable: cannot read body from \"%s\": %v", f, err)
	}

	bodyString := string(content)

	homeMatches := homeAdvRE.FindStringSubmatch(bodyString)
	if len(homeMatches) != 5 { // 4 advantages + full match
		return nil, fmt.Errorf("parseSagarinTable: cannot find home advantage line in \"%s\"", f)
	}

	// ratingAdv, err := strconv.ParseFloat(homeMatches[1], 64)
	// if err != nil {
	// 	return nil, fmt.Errorf("parseSagarinTable: cannot parse string \"%s\" as float: %v", homeMatches[1], err)
	// }

	pointsAdv, err := strconv.ParseFloat(homeMatches[2], 64)
	if err != nil {
		return nil, fmt.Errorf("parseSagarinTable: cannot parse string \"%s\" as float: %v", homeMatches[2], err)
	}

	// goldenMeanAdv, err := strconv.ParseFloat(homeMatches[3], 64)
	// if err != nil {
	// 	return nil, fmt.Errorf("parseSagarinTable: cannot parse string \"%s\" as float: %v", homeMatches[3], err)
	// }

	// recentAdv, err := strconv.ParseFloat(homeMatches[4], 64)
	// if err != nil {
	// 	return nil, fmt.Errorf("parseSagarinTable: cannot parse string \"%s\" as float: %v", homeMatches[4], err)
	// }

	// adv := firestore.SagarinModelParameters{
	// 	TimeDownloaded:          now,
	// 	URL:                     url,
	// 	RatingHomeAdvantage:     ratingAdv,
	// 	PointsHomeAdvantage:     pointsAdv,
	// 	GoldenMeanHomeAdvantage: goldenMeanAdv,
	// 	RecentHomeAdvantage:     recentAdv,
	// }
	// log.Printf("parsed home advantages: %v", adv)

	teamMatches := ratingsRE.FindAllStringSubmatch(bodyString, -1)
	if len(teamMatches) == 0 {
		return nil, fmt.Errorf("parseSagarinTable: cannot find team lines in \"%s\"", f)
	}
	ratings := make([]firestore.ModelTeamPoints, len(teamMatches))
	for i, match := range teamMatches {
		name := match[1]
		teamRef, close, exists := lookup.Lookup(name)
		if !exists {
			estring := fmt.Sprintf("parseSagarinTable: team name \"%s\" not found in teams", name)
			if close == nil {
				return nil, fmt.Errorf(estring)
			}
			estring += fmt.Sprintf("; possible matces: %v", close)
			return nil, fmt.Errorf(estring)
		}

		rating, err := strconv.ParseFloat(match[2], 64)
		if err != nil {
			return nil, fmt.Errorf("parseSagarinTable: cannot parse string \"%s\" as float: %v", match[2], err)
		}

		// points, err := strconv.ParseFloat(match[3], 64)
		// if err != nil {
		// 	return nil, fmt.Errorf("parseSagarinTable: cannot parse string \"%s\" as float: %v", match[3], err)
		// }

		// goldenMean, err := strconv.ParseFloat(match[4], 64)
		// if err != nil {
		// 	return nil, fmt.Errorf("parseSagarinTable: cannot parse string \"%s\" as float: %v", match[4], err)
		// }

		// recent, err := strconv.ParseFloat(match[5], 64)
		// if err != nil {
		// 	return nil, fmt.Errorf("parseSagarinTable: cannot parse string \"%s\" as float: %v", match[5], err)
		// }

		ratings[i] = firestore.ModelTeamPoints{
			Model:         nil, // TODO
			Team:          teamRef,
			Points:        rating,    // TODO
			HomeAdvantage: pointsAdv, // TODO
		}
		log.Printf("parsed team ratings: %v", ratings[i])
	}

	return ratings, nil
}
