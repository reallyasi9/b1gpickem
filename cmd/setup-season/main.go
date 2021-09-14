package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/cfbdata"
	"github.com/reallyasi9/b1gpickem/firestore"
)

// APIKey is a key from collegefootballdata.com
var APIKey string

// ProjectID is the Google Cloud Project ID where the season data will be loaded.
var ProjectID string

// UpdateWeek, if set, will just update one week's worth of games rather than replacing the entire dataset in Firestore.
var UpdateWeek int

// Season is the year of the start of the season.
var Season int

// DryRun, if true, will print the firestore objects to console rather than writing them to firestore.
var DryRun bool

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(), `Usage: setup-season [flags] <Season>

Set up a new season in Firestore. Downloades data from api.collegefootballdata.com and creates a season with teams, venues, weeks, and games collections.

Arguments:
  Season int
    	Year to set up (e.g., %d).
Flags:
`, time.Now().Year())

	flag.PrintDefaults()
}

func init() {
	flag.Usage = usage

	flag.StringVar(&APIKey, "key", "", "API key for collegefootballdata.com.")
	flag.StringVar(&ProjectID, "project", fs.DetectProjectID, "Google Cloud Project ID.  If equal to the empty string, the environment variable GCP_PROJECT will be used.")
	flag.IntVar(&UpdateWeek, "week", -1, "Only update a given week.  If less than or equal to zero, all weeks will be updated.")
	flag.BoolVar(&DryRun, "dryrun", false, "Do not write to firestore, but print to console instead.")
}

func main() {
	parseCommandLine()

	ctx := context.Background()
	fsClient, err := fs.NewClient(ctx, ProjectID)
	if err != nil {
		panic(err)
	}

	httpClient := http.DefaultClient

	weeks, err := cfbdata.GetWeeks(httpClient, APIKey, Season)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Loaded %d weeks\n", weeks.Len())
	if UpdateWeek > 0 {
		var ok bool
		if weeks, ok = weeks.Select(UpdateWeek); !ok {
			panic("week not found")
		}
		fmt.Printf("Limited to week %d\n", UpdateWeek)
	}

	venues, err := cfbdata.GetVenues(httpClient, APIKey)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Loaded %d venues\n", venues.Len())

	teams, err := cfbdata.GetTeams(httpClient, APIKey)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Loaded %d teams\n", teams.Len())

	// games, err := GetGames(httpClient, APIKey, Season, UpdateWeek)
	// if err != nil {
	// 	panic(err)
	// }
	// fmt.Printf("Loaded %d games\n", games.Len())

	// set everything up to write to firestore
	seasonRef := fsClient.Collection("seasons").Doc(strconv.Itoa(Season))
	if err := venues.LinkRefs(seasonRef.Collection("venues")); err != nil {
		panic(err)
	}
	if err := teams.LinkRefs(venues, seasonRef.Collection("teams")); err != nil {
		panic(err)
	}
	// teamRefs, teamsByAbbreviation, teamsByShort, teamsByOther, err := prepTeams(fsClient, seasonRef, teamLookup, teams, venueRefs)
	// if err != nil {
	// 	panic(err)
	// }
	// weekRefs := prepWeeks(fsClient, seasonRef, weekLookup)

	// gamesByWeek := make(map[int]GameCollection)
	// for i, week := range weeks {
	// 	ref := seasonRef.Collection("weeks").Doc(fmt.Sprintf("%d", i))
	// 	weekGames := games.GetWeek(i)
	// 	gameRefs, err := weekGames.LinkRefs(teamRefs, venueRefs, ref.Collection("games"))
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// 	gamesByWeek[i] = weekGames
	// 	week.LinkRefs(gameRefs, seasonRef.Collection("weeks"))
	// }

	// weeks.LinkRefs(gameRefs)
	// updateWeeks(weekLookup, gameRefs)
	// season := firestore.Season{
	// 	Year:                Season,
	// 	StartTime:           weeks[1].FirstGameStart,
	// 	TeamsByOtherName:    teamsByOther,
	// 	TeamsByShortName:    teamsByShort,
	// 	TeamsByAbbreviation: teamsByAbbreviation,
	// }

	if DryRun {
		fmt.Println("DRY RUN: would write the following to firestore:")
		// fmt.Printf("Season:\n%s: %+v\n---\n", seasonRef.Path, season)
		fmt.Println("Venues:")
		cfbdata.DryRun(os.Stdout, venues)
		fmt.Println("---")
		fmt.Println("Teams:")
		cfbdata.DryRun(os.Stdout, teams)
		fmt.Println("---")
		fmt.Println("Weeks:")
		cfbdata.DryRun(os.Stdout, weeks)
		fmt.Println("---")
		return
	}

	// transactions are limited to 500 writes, so split up everything
	// season first
	// _, err = seasonRef.Set(ctx, &season)
	// if err != nil {
	// 	panic(err)
	// }
	// // venues second
	// ids := make([]uint64, 0, len(venueRefs))
	// for id := range venueRefs {
	// 	ids = append(ids, id)
	// }
	// for ll := 0; ll < len(ids); ll += 500 {
	// 	ul := ll + 500
	// 	if ul > len(ids) {
	// 		ul = len(ids)
	// 	}
	// 	err = writeVenues(ctx, fsClient, venueRefs, venueLookup, ids[ll:ul])
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// }
	// // teams third
	// ids = make([]uint64, 0, len(teamRefs))
	// for id := range teamRefs {
	// 	ids = append(ids, id)
	// }
	// for ll := 0; ll < len(ids); ll += 500 {
	// 	ul := ll + 500
	// 	if ul > len(ids) {
	// 		ul = len(ids)
	// 	}
	// 	err = writeTeams(ctx, fsClient, teamRefs, teamLookup, ids[ll:ul])
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// }
	// // weeks fourth
	// for i, ref := range weekRefs {
	// 	week := weekLookup[i]
	// 	_, err = ref.Set(ctx, &week)
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// }
	// // games fifth
	// for wk, grs := range gameRefs {
	// 	gl := gameLookup[wk]
	// 	err = writeGames(ctx, fsClient, grs, gl)
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// }
}

func writeVenues(ctx context.Context, client *fs.Client, vr map[uint64]*fs.DocumentRef, vl map[uint64]firestore.Venue, ids []uint64) error {
	err := client.RunTransaction(ctx, func(ctx context.Context, tx *fs.Transaction) error {
		for _, id := range ids {
			data := vl[id]
			if err := tx.Set(vr[id], &data); err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

func writeTeams(ctx context.Context, client *fs.Client, tr map[uint64]*fs.DocumentRef, tl map[uint64]firestore.Team, ids []uint64) error {
	err := client.RunTransaction(ctx, func(ctx context.Context, tx *fs.Transaction) error {
		for _, id := range ids {
			data := tl[id]
			if err := tx.Set(tr[id], &data); err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

func writeGames(ctx context.Context, client *fs.Client, gr map[uint64]*fs.DocumentRef, gl map[uint64]firestore.Game) error {
	err := client.RunTransaction(ctx, func(ctx context.Context, tx *fs.Transaction) error {
		for id, ref := range gr {
			data := gl[id]
			if err := tx.Set(ref, &data); err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

func parseCommandLine() {
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}
	if APIKey == "" {
		fmt.Println("APIKey not given: this will probably fail.")
	}
	if ProjectID == "" {
		ProjectID = os.Getenv("GCP_PROJECT")
	}
	if ProjectID == "" {
		fmt.Println("-project not given and environment variable GCP_PROJECT not found: this will probably fail.")
	}

	var err error // avoid shadowing
	Season, err = strconv.Atoi(flag.Arg(0))
	if err != nil {
		panic(err)
	}
}

// func prepVenues(client *fs.Client, sr *fs.DocumentRef, vl map[uint64]firestore.Venue) map[uint64]*fs.DocumentRef {
// 	venueRefs := make(map[uint64]*fs.DocumentRef)
// 	for id := range vl {
// 		venueRefs[id] = sr.Collection("venues").Doc(fmt.Sprintf("%d", id))
// 	}
// 	return venueRefs
// }

// func prepTeams(client *fs.Client, sr *fs.DocumentRef, tl map[uint64]firestore.Team, tm map[uint64]Team, vl map[uint64]*fs.DocumentRef) (map[uint64]*fs.DocumentRef, map[string]*fs.DocumentRef, map[string]*fs.DocumentRef, map[string]*fs.DocumentRef, error) {
// 	teamRefs := make(map[uint64]*fs.DocumentRef)
// 	teamsByAbbr := make(map[string]*fs.DocumentRef)
// 	teamsByShort := make(map[string]*fs.DocumentRef)
// 	teamsByOther := make(map[string]*fs.DocumentRef)
// 	for id, team := range tl {
// 		// lookup venue
// 		venueID := tm[id].Location.VenueID
// 		if venueID != nil {
// 			venueRef, ok := vl[*venueID]
// 			if !ok {
// 				return nil, nil, nil, nil, fmt.Errorf("team %d references unknown venue %d", id, *venueID)
// 			}
// 			team.Venue = venueRef
// 			tl[id] = team
// 		}
// 		doc := sr.Collection("teams").Doc(fmt.Sprintf("%d", id))
// 		teamRefs[id] = doc
// 		if _, ok := teamsByAbbr[team.Abbreviation]; ok {
// 			// Attempt 1: abbreviate
// 			team.Abbreviation = abbreviate(team.School)
// 			tl[id] = team
// 		}
// 		_, ok := teamsByAbbr[team.Abbreviation]
// 		for ok {
// 			// Attempt 2: keep adding Xs until we have a unique abbreviation
// 			team.Abbreviation = team.Abbreviation + "X"
// 			tl[id] = team
// 			_, ok = teamsByAbbr[team.Abbreviation]
// 		}
// 		if tfound, ok := teamsByAbbr[team.Abbreviation]; ok {
// 			// Attempt 3: Fail. :()
// 			return nil, nil, nil, nil, fmt.Errorf("abbreviation %s used by both %s and %s", team.Abbreviation, doc.ID, tfound.ID)
// 		}
// 		teamsByAbbr[team.Abbreviation] = doc
// 		for _, name := range team.ShortNames {
// 			if tfound, ok := teamsByShort[name]; ok {
// 				fmt.Printf("short name %s used by both %s and %s: skipping %s\n", name, doc.ID, tfound.ID, doc.ID)
// 				continue
// 			}
// 			teamsByShort[name] = doc
// 		}
// 		for _, name := range team.OtherNames {
// 			if tfound, ok := teamsByOther[name]; ok {
// 				fmt.Printf("other name %s used by both %s and %s: skipping %s\n", name, doc.ID, tfound.ID, doc.ID)
// 				continue
// 			}
// 			teamsByOther[name] = doc
// 		}
// 	}
// 	return teamRefs, teamsByAbbr, teamsByShort, teamsByOther, nil
// }

// func prepWeeks(client *fs.Client, sr *fs.DocumentRef, wl map[int]firestore.Week) map[int]*fs.DocumentRef {
// 	weekRefs := make(map[int]*fs.DocumentRef)
// 	for n, week := range wl {
// 		doc := sr.Collection("weeks").Doc(fmt.Sprintf("%d", n))
// 		weekRefs[n] = doc
// 		week.Season = sr
// 		wl[n] = week
// 	}
// 	return weekRefs
// }

// func prepGames(client *fs.Client, wr *fs.DocumentRef, gl map[uint64]firestore.Game, gm map[uint64]Game, vl map[uint64]*fs.DocumentRef, tl map[uint64]*fs.DocumentRef) (map[uint64]*fs.DocumentRef, error) {
// 	gameRefs := make(map[uint64]*fs.DocumentRef)
// 	for id, game := range gl {
// 		// lookup venue
// 		venueID := gm[id].VenueID
// 		venueRef, ok := vl[venueID]
// 		if !ok {
// 			return nil, fmt.Errorf("game %d references unknown venue %d", id, venueID)
// 		}
// 		game.Venue = venueRef
// 		// lookup home team
// 		homeID := gm[id].HomeID
// 		homeRef, ok := tl[homeID]
// 		if !ok {
// 			return nil, fmt.Errorf("game %d references unknown home team %d", id, homeID)
// 		}
// 		game.HomeTeam = homeRef
// 		// lookup away team
// 		awayID := gm[id].AwayID
// 		awayRef, ok := tl[awayID]
// 		if !ok {
// 			return nil, fmt.Errorf("game %d references unknown away team %d", id, awayID)
// 		}
// 		game.AwayTeam = awayRef
// 		gl[id] = game
// 		gameRefs[id] = wr.Collection("games").Doc(fmt.Sprintf("%d", id))
// 	}
// 	return gameRefs, nil
// }
