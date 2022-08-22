package btsweeks

import (
	"context"
	"fmt"
	"log"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

func SetWeekTypes(ctx *Context) error {
	season, seasonRef, err := firestore.GetSeason(ctx.Context, ctx.FirestoreClient, ctx.Season)
	if err != nil {
		return fmt.Errorf("SetWeekTypes: failed to get season %d: %w", ctx.Season, err)
	}

	if ctx.DryRun {
		log.Printf("DRY RUN: would set the following streak teams for season %d:", ctx.Season)
		for npicks, nweeks := range ctx.WeekTypes {
			log.Printf("%d weeks of %d picks", nweeks, npicks)
		}
		return nil
	}

	// one last error check: count teams and weeks and compare
	nPicks := 0
	for typ, n := range ctx.WeekTypes {
		nPicks += typ * n
	}
	if nPicks != len(season.StreakTeams) && !ctx.Force {
		return fmt.Errorf("SetWeekTypes: number of streak picks calculated from week types (%d) not equal to number of streak teams (%d): explicitly override with --force argument", nPicks, len(season.StreakTeams))
	}
	err = ctx.FirestoreClient.RunTransaction(ctx.Context, func(c context.Context, t *fs.Transaction) error {
		return t.Update(seasonRef, []fs.Update{{Path: "streak_pick_types", Value: &ctx.WeekTypes}})
	})

	if err != nil {
		return fmt.Errorf("SetWeekTypes: failed to execute transaction: %w", err)
	}
	return nil
}
