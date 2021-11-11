package updategames

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

	if dryrun {
		log.Print("DRY RUN: would update the following games:")
		for wn, games := range weekGroups {
			log.Printf("Week %d", wn)
			cfbdata.DryRun(log.Writer(), games)
		}
		return
	}

	// Only update scores and start times
	ctx := context.Background()
	fsClient, err := fs.NewClient(ctx, project)
	if err != nil {
		panic(err)
	}
	seasonRef := fsClient.Collection("seasons").Doc(strconv.Itoa(year))
	for wn, games := range weekGroups {
		err := fsClient.RunTransaction(ctx, func(ctx context.Context, tx *fs.Transaction) error {
			weekRef := seasonRef.Collection("weeks").Doc(strconv.Itoa(wn))
			weekS, err := tx.Get(weekRef)
			if err != nil {
				return err
			}
			if !weekS.Exists() {
				return fmt.Errorf("week %d does not exist (%s)", wn, weekRef.Path)
			}
			updates := make(map[*fs.DocumentRef]firestore.Game)
			for i := 0; i < games.Len(); i++ {
				id := games.ID(i)
				game := games.Datum(i).(firestore.Game)
				gameRef := weekRef.Collection(firestore.GAMES_COLLECTION).Doc(strconv.Itoa(int(id)))
				snap, err := tx.Get(gameRef)
				if err != nil || !snap.Exists() {
					log.Printf("Game %s does not exist in Firestore, so it will not be updated. Use setup-season to pick up this new game.", gameRef.Path)
					continue
				}
				updates[gameRef] = game
			}
			for gameRef, game := range updates {
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
			return nil
		})
		if err != nil {
			panic(err)
		}
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
