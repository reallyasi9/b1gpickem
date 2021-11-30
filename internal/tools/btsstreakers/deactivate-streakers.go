package btsstreakers

import (
	"context"
	"fmt"
	"log"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

func DeactivateStreakers(ctx *Context) error {
	season, seasonRef, err := firestore.GetSeason(ctx, ctx.FirestoreClient, ctx.Season)
	if err != nil {
		return fmt.Errorf("DeactivateStreakers: failed to get season %d: %w", ctx.Season, err)
	}
	weekRefs, err := seasonRef.Collection(firestore.WEEKS_COLLECTION).DocumentRefs(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("DeactivateStreakers: failed to get weeks: %w", err)
	}
	// error check
	pickerRefs := make(map[string][]*fs.DocumentRef)
	for _, name := range ctx.StreakerNames {
		var ref *fs.DocumentRef
		var exists bool
		if ref, exists = season.Pickers[name]; !exists {
			return fmt.Errorf("DeactivateStreakers: streaker '%s' not active in season %d", name, ctx.Season)
		}
		strRefs := make([]*fs.DocumentRef, 0)
		for _, weekRef := range weekRefs {
			_, strRef, err := firestore.GetStreakTeamsRemaining(ctx, seasonRef, weekRef, ref)
			if err != nil {
				if _, converts := err.(firestore.NoStreakTeamsRemaining); converts {
					continue
				}
				return fmt.Errorf("DeactivateStreakers: failed to get streak teams remaining for picker '%s' in season %d week '%s'", name, ctx.Season, weekRef.ID)
			}
			strRefs = append(strRefs, strRef)
		}
		pickerRefs[ref.ID] = strRefs
	}

	if ctx.DryRun {
		log.Printf("DRY RUN: would deactivate and delete the following streak teams remaining for season %d:", ctx.Season)
		for id, strs := range pickerRefs {
			log.Printf("%s: %v", id, strs)
		}
		return nil
	}

	if !ctx.Force {
		return fmt.Errorf("DeactivateStreakers: refusing to delete records from datastore: rerun with --force argument to override")
	}

	err = ctx.FirestoreClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
		for _, strs := range pickerRefs {
			for _, ref := range strs {
				err = t.Delete(ref)
				if err != nil {
					return err
				}
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("DeactivateStreakers: failed to execute transaction: %w", err)
	}

	return nil
}
