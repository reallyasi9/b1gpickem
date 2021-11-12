package setupseason

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/cfbdata"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

// FlagSet is a flag.FlagSet for parsing the setup-season subcommand.
var FlagSet *flag.FlagSet

const COMMAND = "setup-season"

var force bool
var dryrun bool
var project string

func InitializeSubcommand() {
	FlagSet = flag.NewFlagSet(COMMAND, flag.ExitOnError)
	FlagSet.SetOutput(flag.CommandLine.Output())
	FlagSet.Usage = Usage

	FlagSet.BoolVar(&force, "force", false, "Force overwrite of data.")
	FlagSet.BoolVar(&dryrun, "dryrun", false, "Perform dry run: print intended actions to the log, but do not modify any data.")
	FlagSet.StringVar(&project, "project", os.Getenv("GCP_PROJECT"), "GCP Project ID. Defaults to the environment variable GCP_PROJECT, if set.")
}

// Usage is the usage documentation for the setup-season subcommand.
func Usage() {
	fmt.Fprintf(flag.CommandLine.Output(), `Usage: %s [global-flags] %s [flags] <apikey> <season> [week [week...]]
	
Set up a new season in Firestore. Downloads data from api.collegefootballdata.com and creates a season with teams, venues, weeks, and games collections.
	
Arguments:
  apikey string
        API key to access CollegeFootballData.com data.
  season int
        Year to set up.
  week int
		Specific week to setup. Multiple weeks can be specified. If no week is given, all weeks will be setup.

Flags:
`, flag.CommandLine.Name(), COMMAND)

	FlagSet.PrintDefaults()

	fmt.Fprint(flag.CommandLine.Output(), "\nGlobal Flags:\n")

	flag.PrintDefaults()

}

func SetupSeason() {
	err := FlagSet.Parse(flag.Args()[1:])
	if err != nil {
		log.Fatalf("Failed to parse setup-season arguments: %v", err)
	}
	if FlagSet.NArg() < 2 {
		FlagSet.Usage()
		log.Fatal("API key and season arguments not supplied")
	}
	apiKey := FlagSet.Arg(0)

	year, err := strconv.Atoi(FlagSet.Arg(1))
	if err != nil {
		log.Fatalf("Failed to parse season: %v", err)
	}

	weekArgs := make([]int, FlagSet.NArg()-2)
	for i, arg := range FlagSet.Args()[2:] {
		weekArgs[i], err = strconv.Atoi(arg)
		if err != nil {
			log.Fatalf("Failed to parse week: %v", err)
		}
	}

	ctx := context.Background()
	fsClient, err := fs.NewClient(ctx, project)
	if err != nil {
		panic(err)
	}

	httpClient := http.DefaultClient

	weeks, err := cfbdata.GetWeeks(httpClient, apiKey, year, weekArgs)
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
		Year:            year,
		StartTime:       weeks.FirstStartTime(),
		Pickers:         make(map[string]*fs.DocumentRef),
		StreakTeams:     make([]*fs.DocumentRef, 0),
		StreakPickTypes: make([]int, 0),
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
		if err := gs.LinkRefs(teams, venues, wr.Collection(firestore.GAMES_COLLECTION)); err != nil {
			panic(err)
		}
		gamesByWeek[id] = gs
	}

	if dryrun {
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
	if force {
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
