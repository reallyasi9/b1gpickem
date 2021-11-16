package updategames

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/cfbdata"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

// FlagSet is a flag.FlagSet for parsing the update-games subcommand.
var FlagSet *flag.FlagSet

const COMMAND = "update-games"

var dryrun bool
var project string

// Usage is the usage documentation for the update-games subcommand.
func Usage() {
	fmt.Fprintf(flag.CommandLine.Output(), `Usage: %s [global-flags] %s [flags] <apikey> <season> <week> [week [...]]
	
Update games by week in Firestore. Downloads data from api.collegefootballdata.com and updates the time and score of games.
	
Arguments:
  apikey string
        API key to access CollegeFootballData.com data.
  season int
        Year to set up.
  week int
        Week(s) to update. Multiple weeks allowed.

Flags:
`, flag.CommandLine.Name(), COMMAND)

	FlagSet.PrintDefaults()

	fmt.Fprint(flag.CommandLine.Output(), "\nGlobal Flags:\n")

	flag.PrintDefaults()

}

func InitializeSubcommand() {
	FlagSet = flag.NewFlagSet(COMMAND, flag.ExitOnError)
	FlagSet.SetOutput(flag.CommandLine.Output())
	FlagSet.Usage = Usage

	FlagSet.BoolVar(&dryrun, "dryrun", false, "Perform dry run: print intended actions to the log, but do not modify any data.")
	FlagSet.StringVar(&project, "project", os.Getenv("GCP_PROJECT"), "GCP Project ID. Defaults to the environment variable GCP_PROJECT, if set.")
}

func UpdateGames() {
	err := FlagSet.Parse(flag.Args()[1:])
	if err != nil {
		log.Fatalf("Failed to parse update-games arguments: %v", err)
	}
	if FlagSet.NArg() < 3 {
		FlagSet.Usage()
		log.Fatal("API key, season, and week arguments not supplied")
	}

	apiKey := FlagSet.Arg(0)

	year, err := strconv.Atoi(FlagSet.Arg(1))
	if err != nil {
		log.Fatalf("Failed to parse season: %v", err)
	}

	weekNumbers, err := parseWeekArgs(FlagSet.Args()[2:])
	if err != nil {
		log.Fatalf("Failed to parse weeks: %v", err)
	}

	httpClient := http.DefaultClient

	weekGroups := make(map[int]cfbdata.GameCollection)
	for _, wn := range weekNumbers {
		games, err := cfbdata.GetGames(httpClient, apiKey, year, wn)
		if err != nil {
			panic(err)
		}
		weekGroups[wn] = games
		log.Printf("Loaded %d games from week %d\n", games.Len(), wn)
	}

	ctx := context.Background()
	fsClient, err := fs.NewClient(ctx, project)
	if err != nil {
		panic(err)
	}
	seasonRef := fsClient.Collection(firestore.SEASONS_COLLECTION).Doc(strconv.Itoa(year))

	// find games that do not exist
	knownWeekGroups, unknownWeekGroups, err := splitWeekGroups(ctx, seasonRef, weekGroups)

	var tc cfbdata.TeamCollection
	var vc cfbdata.VenueCollection
	var once sync.Once
	for wn, wg := range unknownWeekGroups {
		if wg.Len() == 0 {
			continue
		}
		log.Printf("Discovered new games in week %d: requesting additional data", wn)
		once.Do(func() {
			tc, err = cfbdata.GetTeams(httpClient, apiKey)
			if err != nil {
				panic(err)
			}
			log.Printf("Got %d teams", tc.Len())
			vc, err = cfbdata.GetVenues(httpClient, apiKey)
			if err != nil {
				panic(err)
			}
			log.Printf("Got %d venues", vc.Len())
			venuesCollection := seasonRef.Collection(firestore.VENUES_COLLECTION)
			err = vc.LinkRefs(venuesCollection)
			if err != nil {
				panic(err)
			}
			teamsCollection := seasonRef.Collection(firestore.TEAMS_COLLECTION)
			err = tc.LinkRefs(vc, teamsCollection)
			if err != nil {
				panic(err)
			}
		})
		weekCol := seasonRef.Collection(firestore.WEEKS_COLLECTION)
		err = wg.LinkRefs(tc, vc, weekCol)
		if err != nil {
			panic(err)
		}
		unknownWeekGroups[wn] = wg
	}

	if dryrun {
		log.Print("DRY RUN: would update the following games:")
		for wn, games := range knownWeekGroups {
			log.Printf("Known games for week %d", wn)
			cfbdata.DryRun(log.Writer(), games)
		}
		for wn, games := range unknownWeekGroups {
			log.Printf("Unknown games for week %d", wn)
			cfbdata.DryRun(log.Writer(), games)
		}
		return
	}

	err = fsClient.RunTransaction(ctx, func(ctx context.Context, tx *fs.Transaction) error {
		// check existence of weeks
		for wn := range knownWeekGroups {
			weekRef := seasonRef.Collection(firestore.WEEKS_COLLECTION).Doc(strconv.Itoa(wn))
			weekS, err := tx.Get(weekRef)
			if err != nil {
				return err
			}
			if !weekS.Exists() {
				return fmt.Errorf("week %d does not exist (%s)", wn, weekRef.Path)
			}
		}
		// update old games
		for wn, games := range knownWeekGroups {
			weekRef := seasonRef.Collection(firestore.WEEKS_COLLECTION).Doc(strconv.Itoa(wn))
			for i := 0; i < games.Len(); i++ {
				id := games.ID(i)
				game := games.Datum(i).(firestore.Game)
				gameRef := weekRef.Collection(firestore.GAMES_COLLECTION).Doc(strconv.Itoa(int(id)))
				err := tx.Update(gameRef, []fs.Update{
					{Path: "home_points", Value: game.HomePoints},
					{Path: "away_points", Value: game.AwayPoints},
					{Path: "start_time", Value: game.StartTime},
					{Path: "start_time_tbd", Value: game.StartTimeTBD},
				})
				if err != nil {
					return err
				}
			}
		}
		// make new games
		for wn, games := range unknownWeekGroups {
			weekRef := seasonRef.Collection(firestore.WEEKS_COLLECTION).Doc(strconv.Itoa(wn))
			for i := 0; i < games.Len(); i++ {
				id := games.ID(i)
				game := games.Datum(i).(firestore.Game)
				gameRef := weekRef.Collection(firestore.GAMES_COLLECTION).Doc(strconv.Itoa(int(id)))
				err := tx.Create(gameRef, &game)
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
}

func parseWeekArgs(args []string) ([]int, error) {
	weeks := make(map[int]struct{})
	for _, w := range args {
		iwk, err := strconv.Atoi(w)
		if err != nil {
			return nil, fmt.Errorf("unable to parse week '%s': %v", w, err)
		}
		weeks[iwk] = struct{}{}
	}
	distinctWeeks := make([]int, 0, len(weeks))
	for i := range weeks {
		distinctWeeks = append(distinctWeeks, i)
	}
	return distinctWeeks, nil
}

func splitWeekGroups(ctx context.Context, seasonRef *fs.DocumentRef, weekGroups map[int]cfbdata.GameCollection) (known, unknown map[int]cfbdata.GameCollection, err error) {
	known = make(map[int]cfbdata.GameCollection)
	unknown = make(map[int]cfbdata.GameCollection)

	for wn, gc := range weekGroups {
		_, weekRef, e := firestore.GetWeek(ctx, seasonRef, wn)
		if e != nil {
			err = e
			return
		}
		_, gameRefs, e := firestore.GetGames(ctx, weekRef)
		if e != nil {
			err = e
			return
		}
		knownGames := make(map[int64]struct{})
		for _, ref := range gameRefs {
			sid, _ := strconv.Atoi(ref.ID)
			knownGames[int64(sid)] = struct{}{}
		}
		knownIdx := make([]int, 0)
		for i := 0; i < gc.Len(); i++ {
			id := gc.ID(i)
			if _, ok := knownGames[id]; ok {
				knownIdx = append(knownIdx, i)
			}
		}
		known[wn], unknown[wn] = gc.Split(knownIdx)
	}

	return known, unknown, err
}
