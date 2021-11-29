package btsstreakers

import (
	"context"
	"fmt"
	"log"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

func ActivateStreakers(ctx *Context) error {
	season, seasonRef, err := firestore.GetSeason(ctx, ctx.FirestoreClient, ctx.Season)
	if err != nil {
		return fmt.Errorf("ActivateStreakers: failed to get season %d: %w", ctx.Season, err)
	}
	_, weekRef, err := firestore.GetFirstWeek(ctx, seasonRef)
	if err != nil {
		return fmt.Errorf("ActivateStreakers: failed to get first week: %w", err)
	}
	// error check
	pickerRefs := make(map[*fs.DocumentRef]struct{})
	for _, name := range ctx.StreakerNames {
		var ref *fs.DocumentRef
		var exists bool
		if ref, exists = season.Pickers[name]; !exists {
			return fmt.Errorf("ActivateStreakers: streaker '%s' not active in season %d", name, ctx.Season)
		}
		_, _, err = firestore.GetStreakTeamsRemaining(ctx, seasonRef, weekRef, ref)
		if err != nil {
			if _, converts := err.(firestore.NoStreakTeamsRemaining); !converts {
				return fmt.Errorf("ActivateStreakers: failed to lookup streak teams remaining for streaker '%s' in week '%s' of season %d: %w", name, weekRef.ID, ctx.Season, err)
			}
			return fmt.Errorf("ActivateStreakers: picker '%s' already active in season %d", name, ctx.Season)
		}
		pickerRefs[ref] = struct{}{}
	}

	strs := make(map[*fs.DocumentRef]firestore.StreakTeamsRemaining)
	for pickerRef := range pickerRefs {
		str, ref, err := firestore.GetStreakTeamsRemaining(ctx, seasonRef, nil, pickerRef)
		if err != nil {
			return fmt.Errorf("ActivateStreakers: failed to lookup streak teams remaining for streaker '%s' in week '%s' of season %d: %w", pickerRef.ID, weekRef.ID, ctx.Season, err)
		}
		if str.PickTypesRemaining == nil {
			return fmt.Errorf("ActivateStreakers: refusing to activate streak for season %d when no week types are defined", ctx.Season)
		}
		strs[ref] = str
	}

	if ctx.DryRun {
		log.Printf("DRY RUN: would activate the following streaks for season %d:", ctx.Season)
		for ref, str := range strs {
			log.Printf("%s: %v", ref.Path, str)
		}
		return nil
	}

	err = ctx.FirestoreClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
		for ref, str := range strs {
			err = t.Create(ref, &str)
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("ActivateStreakers: failed to execute transaction: %w", err)
	}

	return nil
}
