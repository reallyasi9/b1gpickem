package setupseason

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

func SplitWeek(ctx *Context) error {
	_, seasonRef, err := firestore.GetSeason(ctx, ctx.FirestoreClient, ctx.Season)
	if err != nil {
		return fmt.Errorf("SplitWeek: failed to get season: %w", err)
	}

	games, gameRefs, err := firestore.GetGamesByStartTime(ctx, seasonRef, ctx.SplitTimeFrom, ctx.SplitTimeTo)
	if err != nil {
		return fmt.Errorf("SplitWeek: failed to get games: %w", err)
	}
	log.Printf("Loaded %d games", len(games))

	week, weekRef, err := firestore.GetWeek(ctx, seasonRef, ctx.NewWeekNumber)
	if _, converted := err.(firestore.NoWeekError); converted {
		// make a new week and use that
		week.Number = ctx.NewWeekNumber
		week.FirstGameStart = ctx.SplitTimeFrom
		weekRef = seasonRef.Collection(firestore.WEEKS_COLLECTION).Doc(strconv.Itoa(ctx.NewWeekNumber))
		log.Printf("Creating new week %d", ctx.NewWeekNumber)
	} else if err != nil {
		log.Printf("Moving to week %d", ctx.NewWeekNumber)
	} else {
		return fmt.Errorf("SplitWeek: failed to get week: %w", err)
	}

	earliestTime := week.FirstGameStart
	newGameRefs := make([]*fs.DocumentRef, len(gameRefs))
	// Set the earliest time for the new week
	for i, game := range games {
		if !game.StartTimeTBD && game.StartTime.Before(earliestTime) {
			earliestTime = game.StartTime
		}
		newGameRefs[i] = weekRef.Collection(firestore.GAMES_COLLECTION).Doc(gameRefs[i].ID)
	}

	if ctx.DryRun {
		log.Print("DRY RUN: would perform the following actions in Firestore:")
		for i, ref := range gameRefs {
			log.Printf("MOVE: %s to %s", ref.Path, newGameRefs[i].Path)
		}
		return nil
	}

	if !ctx.Force {
		return fmt.Errorf("SplitWeek: split-weeks deletes entries: --force flag is required to make changes")
	}

	weeksToReset := make(map[string]*fs.DocumentRef)
	err = ctx.FirestoreClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
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
		return fmt.Errorf("SplitWeek: failed to split week: %w", err)
	}

	for _, resetWeek := range weeksToReset {
		log.Printf("Refreshing week %s", resetWeek.ID)
		t, err := refreshFirstGameStart(ctx, ctx.FirestoreClient, resetWeek)
		if err != nil {
			return fmt.Errorf("SplitWeek: failed to refresh week: %w", err)
		}
		snap, err := resetWeek.Get(ctx)
		if err != nil {
			return fmt.Errorf("SplitWeek: failed to get week: %w", err)
		}
		var w firestore.Week
		err = snap.DataTo(&w)
		if err != nil {
			return fmt.Errorf("SplitWeek: failed to assign week data: %w", err)
		}
		w.FirstGameStart = t
		_, err = resetWeek.Set(ctx, &w)
		if err != nil {
			return fmt.Errorf("SplitWeek: failed to write new week data: %w", err)
		}
	}

	return nil
}

func refreshFirstGameStart(ctx context.Context, fsclient *fs.Client, weekRef *fs.DocumentRef) (time.Time, error) {
	var earliest time.Time
	games, _, err := firestore.GetGames(ctx, weekRef)
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
