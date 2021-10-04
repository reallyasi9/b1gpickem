package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/cfbdata"
	"github.com/reallyasi9/b1gpickem/firestore"
)

// seasonFlagSet is a flag.FlagSet for parsing the setup-season subcommand.
var seasonFlagSet *flag.FlagSet

// seasonUsage is the usage documentation for the setup-season subcommand.
func seasonUsage() {
	fmt.Fprintf(flag.CommandLine.Output(), `Usage: b1gtool [global-flags] setup-season [flags] <apikey> <season>
	
Set up a new season in Firestore. Downloads data from api.collegefootballdata.com and creates a season with teams, venues, weeks, and games collections.
	
Arguments:
  apikey string
        API key to access CollegeFootballData.com data.
  season int
        Year to set up (e.g., %d).

Flags:
`, time.Now().Year())

	seasonFlagSet.PrintDefaults()

	fmt.Fprint(flag.CommandLine.Output(), "\nGlobal Flags:\n")

	flag.PrintDefaults()

}

func init() {
	seasonFlagSet = flag.NewFlagSet("setup-season", flag.ExitOnError)
	seasonFlagSet.SetOutput(flag.CommandLine.Output())
	seasonFlagSet.Usage = seasonUsage

	Commands["setup-season"] = setupSeason
	Usage["setup-season"] = seasonUsage
}

func setupSeason() {
	err := seasonFlagSet.Parse(flag.Args()[1:])
	if err != nil {
		log.Fatalf("Failed to parse setup-season arguments: %v", err)
	}
	if seasonFlagSet.NArg() < 2 {
		seasonFlagSet.Usage()
		log.Fatal("API key and season arguments not supplied")
	}
	if seasonFlagSet.NArg() > 2 {
		seasonFlagSet.Usage()
		log.Fatal("Too many arguments supplied")
	}
	apiKey := seasonFlagSet.Arg(0)

	year, err := strconv.Atoi(seasonFlagSet.Arg(1))
	if err != nil {
		log.Fatalf("Failed to parse season: %v", err)
	}

	ctx := context.Background()
	fsClient, err := fs.NewClient(ctx, ProjectID)
	if err != nil {
		panic(err)
	}

	httpClient := http.DefaultClient

	weeks, err := cfbdata.GetWeeks(httpClient, apiKey, year)
	if err != nil {
		panic(err)
	}
	log.Printf("Loaded %d weeks\n", weeks.Len())

	venues, err := cfbdata.GetVenues(httpClient, apiKey)
	if err != nil {
		panic(err)
	}
	log.Printf("Loaded %d venues\n", venues.Len())

	teams, err := cfbdata.GetTeams(httpClient, apiKey)
	if err != nil {
		panic(err)
	}
	log.Printf("Loaded %d teams\n", teams.Len())

	games, err := cfbdata.GetAllGames(httpClient, apiKey, year)
	if err != nil {
		panic(err)
	}
	log.Printf("Loaded %d games\n", games.Len())

	// eliminate teams that are not in games
	teams = teams.EliminateNonContenders(games)

	// set everything up to write to firestore
	seasonRef := fsClient.Collection("seasons").Doc(strconv.Itoa(year))
	season := firestore.Season{
		Year:      year,
		StartTime: weeks.FirstStartTime(),
		Pickers:   make(map[string]*fs.DocumentRef),
	}
	if err := weeks.LinkRefs(seasonRef.Collection("weeks")); err != nil {
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
		log.Println("DRY RUN: would write the following to firestore:")
		log.Printf("Season:\n%s: %+v\n---\n", seasonRef.Path, season)
		log.Println("Venues:")
		cfbdata.DryRun(log.Writer(), venues)
		log.Println("---")
		log.Println("Teams:")
		cfbdata.DryRun(log.Writer(), teams)
		log.Println("---")
		log.Println("Weeks:")
		cfbdata.DryRun(log.Writer(), weeks)
		log.Println("---")
		log.Println("Games:")
		for wk, gc := range gamesByWeek {
			log.Printf("Week %d\n", wk)
			cfbdata.DryRun(log.Writer(), gc)
		}
		log.Println("---")
		return
	}

	// Either set or create, depending on force parameter
	writeFunc := func(tx *fs.Transaction, ref *fs.DocumentRef, d interface{}) error {
		return tx.Create(ref, d)
	}
	if Force {
		log.Println("Forcing overwrite with SET command")
		writeFunc = func(tx *fs.Transaction, ref *fs.DocumentRef, d interface{}) error {
			return tx.Set(ref, d)
		}
		_, err := seasonRef.Set(ctx, &season)
		if err != nil {
			panic(err)
		}
	} else {
		log.Println("Writing with CREATE command")
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
	// Weeks fourth
	errs = cfbdata.IterateWrite(ctx, fsClient, weeks, 500, writeFunc)
	for err := range errs {
		if err != nil {
			panic(err)
		}
	}
	// Games fifth
	for _, weekOfGames := range gamesByWeek {
		errs = cfbdata.IterateWrite(ctx, fsClient, weekOfGames, 500, writeFunc)
		for err := range errs {
			if err != nil {
				panic(err)
			}
		}
	}
}
