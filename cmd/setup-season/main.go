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

// Force, if set, forcefully overwrite data in Firestore instead of failing if the documents already exist
var Force bool

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
	flag.BoolVar(&Force, "force", false, "Force overwrite of data in Firestore with the SET command rather than failing if the data already exists.")
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

	games, err := cfbdata.GetGames(httpClient, APIKey, Season)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Loaded %d games\n", games.Len())

	// games, err := GetGames(httpClient, APIKey, Season, UpdateWeek)
	// if err != nil {
	// 	panic(err)
	// }
	// fmt.Printf("Loaded %d games\n", games.Len())

	// set everything up to write to firestore
	seasonRef := fsClient.Collection("seasons").Doc(strconv.Itoa(Season))
	season := firestore.Season{
		Year:      Season,
		StartTime: weeks.FirstStartTime(),
	}
	if err := weeks.LinkRefs(seasonRef, seasonRef.Collection("weeks")); err != nil {
		panic(err)
	}
	if err := venues.LinkRefs(seasonRef.Collection("venues")); err != nil {
		panic(err)
	}
	if err := teams.LinkRefs(venues, seasonRef.Collection("teams")); err != nil {
		panic(err)
	}
	gamesByWeek := make(map[int64]cfbdata.GameCollection)
	for i := 0; i < weeks.Len(); i++ {
		id := weeks.ID(i)
		wr := weeks.Ref(i)
		gs := games.GetWeek(int(id))
		if err := gs.LinkRefs(teams, venues, wr.Collection("games")); err != nil {
			panic(err)
		}
		gamesByWeek[id] = gs
	}

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
		fmt.Println("Games:")
		for wk, gc := range gamesByWeek {
			fmt.Printf("Week %d\n", wk)
			cfbdata.DryRun(os.Stdout, gc)
		}
		fmt.Println("---")
		return
	}

	// Either set or create, depending on force parameter
	writeFunc := func(tx *fs.Transaction, ref *fs.DocumentRef, d interface{}) error {
		return tx.Create(ref, d)
	}
	if Force {
		fmt.Println("Forcing overwrite with SET command")
		writeFunc = func(tx *fs.Transaction, ref *fs.DocumentRef, d interface{}) error {
			return tx.Set(ref, d)
		}
		_, err := seasonRef.Set(ctx, &season)
		if err != nil {
			panic(err)
		}
	} else {
		fmt.Println("Writing with CREATE command")
		_, err := seasonRef.Create(ctx, &season)
		if err != nil {
			panic(err)
		}
	}

	// Venues second
	errs := cfbdata.IterateWrite(ctx, fsClient, venues, 500, writeFunc)
	for err := range errs {
		if err != nil {
			panic(err)
		}
	}
	// Teams third
	errs = cfbdata.IterateWrite(ctx, fsClient, teams, 500, writeFunc)
	for err := range errs {
		if err != nil {
			panic(err)
		}
	}
	// Games fourth
	for _, weekOfGames := range gamesByWeek {
		errs = cfbdata.IterateWrite(ctx, fsClient, weekOfGames, 500, writeFunc)
		for err := range errs {
			if err != nil {
				panic(err)
			}
		}
	}
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
