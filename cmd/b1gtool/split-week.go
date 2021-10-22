package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strconv"
	"time"

	fs "cloud.google.com/go/firestore"
	bpefs "github.com/reallyasi9/b1gpickem/internal/firestore"
)

var splitFlagSet *flag.FlagSet

// splitUsage is the usage documentation for the split-week subcommand.
func splitUsage() {
	fmt.Fprintf(flag.CommandLine.Output(), `Usage: b1gtool [global-flags] split-week [flags] <season> <new-week> <date-from> <date-to>
	
Reassign games to a new week number.

Arguments:
  season int
      Season year from which to reassign games. A value less than zero will cause the program to attempt to detect the season from the date-from argument.
  new-week int
      Week number to which the matched games will be assigned. If the week number does not exist, it will be created.
  date-from date
      Date in YYYY/MM/DD format to mark the start of the games to be assigned to the new week number. Games that start on or after this date will be assigned the new week number.
  date-to date
      Date in YYYY/MM/DD format to mark the end of the games to be assigned to the new week number. Games before this date will be assigned the new week number.
	
Flags:
`)

	splitFlagSet.PrintDefaults()

	fmt.Fprint(flag.CommandLine.Output(), "\nGlobal Flags:\n")

	flag.PrintDefaults()

}

func init() {
	cmd := "split-week"

	splitFlagSet = flag.NewFlagSet(cmd, flag.ExitOnError)
	splitFlagSet.SetOutput(flag.CommandLine.Output())
	splitFlagSet.Usage = splitUsage

	Commands[cmd] = splitWeek
	Usage[cmd] = splitUsage
}

func splitWeek() {
	err := splitFlagSet.Parse(flag.Args()[1:])
	if err != nil {
		log.Fatalf("Failed to parse split-week arguments: %v", err)
	}
	if splitFlagSet.NArg() != 4 {
		splitFlagSet.Usage()
		log.Fatal("Wrong number of arguments supplied.")
	}
	season, err := strconv.Atoi(splitFlagSet.Arg(0))
	if err != nil {
		log.Fatalf("Unable to parse season argument: %v", err)
	}
	log.Printf("Season: %d", season)
	newWeek, err := strconv.Atoi(splitFlagSet.Arg(1))
	if err != nil {
		log.Fatalf("Unable to parse new-week argument: %v", err)
	}
	log.Printf("New week: %d", newWeek)
	dateFormat := "2006/01/02"
	startTime, err := time.Parse(dateFormat, splitFlagSet.Arg(2))
	if err != nil {
		log.Fatalf("Unable to parse date-from argument: %v", err)
	}
	log.Printf("Date from: %v", startTime)
	endTime, err := time.Parse(dateFormat, splitFlagSet.Arg(3))
	if err != nil {
		log.Fatalf("Unable to parse date-to argument: %v", err)
	}
	log.Printf("Date to: %v", endTime)
	if season < 0 {
		season = startTime.Year()
		log.Printf("Parsed season from date: %d", season)
	}

	ctx := context.Background()
	fsclient, err := fs.NewClient(ctx, ProjectID)
	if err != nil {
		log.Fatalf("Unable to create Firestore client: %v", err)
	}

	_, seasonRef, err := bpefs.GetSeason(ctx, fsclient, season)
	if err != nil {
		log.Fatalf("Unable to get season: %v", err)
	}

	games, gameRefs, err := bpefs.GetGamesByStartTime(ctx, fsclient, seasonRef, startTime, endTime)
	if err != nil {
		log.Fatalf("Unable to get games: %v", err)
	}
	log.Printf("Loaded %d games", len(games))

	week, weekRef, err := bpefs.GetWeek(ctx, fsclient, seasonRef, newWeek)
	if err != nil {
		// make a new week and use that
		week.Number = newWeek
		week.FirstGameStart = endTime
		weekRef = seasonRef.Collection(bpefs.WEEKS_COLLECTION).Doc(strconv.Itoa(newWeek))
		log.Printf("Creating new week %d", newWeek)
	} else {
		log.Printf("Moving to week %d", newWeek)
	}

	earliestTime := week.FirstGameStart
	newGameRefs := make([]*fs.DocumentRef, len(gameRefs))
	// Set the earliest time for the new week
	for i, game := range games {
		if !game.StartTimeTBD && game.StartTime.Before(earliestTime) {
			earliestTime = game.StartTime
		}
		newGameRefs[i] = weekRef.Collection(bpefs.GAMES_COLLECTION).Doc(gameRefs[i].ID)
	}

	if DryRun {
		log.Print("DRY RUN: would perform the following actions in Firestore:")
		for i, ref := range gameRefs {
			log.Printf("MOVE: %s to %s", ref.Path, newGameRefs[i].Path)
		}
		return
	}

	if !Force {
		log.Print("Because split-weeks deletes entries, -force flag is required to make changes.")
		return
	}

	weeksToReset := make(map[string]*fs.DocumentRef)
	err = fsclient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
		err = t.Set(weekRef, &week)
		if err != nil {
			return err
		}
		for i, ref := range gameRefs {
			err := t.Delete(ref)
			if err != nil {
				return err
			}
			err = t.Set(newGameRefs[i], &games[i])
			if err != nil {
				return err
			}
			weeksToReset[ref.Parent.Parent.ID] = ref.Parent.Parent
		}
		return nil
	})

	if err != nil {
		log.Fatalf("Failed to split week: %v", err)
	}

	for _, resetWeek := range weeksToReset {
		log.Printf("Refreshing week %s", resetWeek.ID)
		t, err := refreshFirstGameStart(ctx, fsclient, resetWeek)
		if err != nil {
			log.Fatalf("Failed to refresh week: %v", err)
		}
		snap, err := resetWeek.Get(ctx)
		if err != nil {
			log.Fatalf("Failed to get week: %v", err)
		}
		var w bpefs.Week
		err = snap.DataTo(&w)
		if err != nil {
			log.Fatalf("Failed to assign week data: %v", err)
		}
		w.FirstGameStart = t
		_, err = resetWeek.Set(ctx, &w)
		if err != nil {
			log.Fatalf("Failed to write new week data: %v", err)
		}
	}

}

func refreshFirstGameStart(ctx context.Context, fsclient *fs.Client, weekRef *fs.DocumentRef) (time.Time, error) {
	var earliest time.Time
	games, _, err := bpefs.GetGames(ctx, fsclient, weekRef)
	if err != nil {
		return earliest, err
	}
	var nilTime time.Time
	for _, game := range games {
		if !game.StartTimeTBD && (earliest.Equal(nilTime) || game.StartTime.Before(earliest)) {
			earliest = game.StartTime
		}
	}
	return earliest, nil
}
