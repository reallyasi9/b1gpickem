package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/cfbdata"
	"github.com/reallyasi9/b1gpickem/firestore"
)

// ugFlagSet is a flag.FlagSet for parsing the update-games subcommand.
var ugFlagSet *flag.FlagSet

// ugUsage is the usage documentation for the update-games subcommand.
func ugUsage() {
	fmt.Fprint(flag.CommandLine.Output(), `Usage: b1gtool [global-flags] update-games [flags] <apikey> <season> <week> [week [...]]
	
Update games by week in Firestore. Downloads data from api.collegefootballdata.com and updates the time and score of games.
	
Arguments:
  apikey string
        API key to access CollegeFootballData.com data.
  season int
        Year to set up.
  week int
        Week(s) to update. Multiple weeks allowed.
Flags:
`)

	ugFlagSet.PrintDefaults()

	fmt.Fprint(flag.CommandLine.Output(), "Global Flags:\n")

	flag.PrintDefaults()

}

func init() {
	ugFlagSet = flag.NewFlagSet("update-games", flag.ExitOnError)
	ugFlagSet.SetOutput(flag.CommandLine.Output())
	ugFlagSet.Usage = ugUsage
}

func updateGames() {
	err := ugFlagSet.Parse(flag.Args()[1:])
	if err != nil {
		log.Fatalf("Failed to parse update-games arguments: %v", err)
	}
	if ugFlagSet.NArg() < 3 {
		ugFlagSet.Usage()
		log.Fatal("API key, season, and week arguments not supplied")
	}
	if Force {
		log.Print("-force argument ignored")
	}

	apiKey := ugFlagSet.Arg(0)

	year, err := strconv.Atoi(ugFlagSet.Arg(1))
	if err != nil {
		log.Fatalf("Failed to parse season: %v", err)
	}

	weekNumbers, err := parseWeekArgs(ugFlagSet.Args()[2:])
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

	if DryRun {
		log.Print("DRY RUN: would update the following games:")
		for wn, games := range weekGroups {
			log.Printf("Week %d", wn)
			cfbdata.DryRun(log.Writer(), games)
		}
		return
	}

	// Only update scores and start times
	ctx := context.Background()
	fsClient, err := fs.NewClient(ctx, ProjectID)
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
			for i := 0; i < games.Len(); i++ {
				id := games.ID(i)
				game := games.Datum(i).(firestore.Game)
				gameRef := weekRef.Collection("games").Doc(strconv.Itoa(int(id)))
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
