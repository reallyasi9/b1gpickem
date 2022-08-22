package editpickers

import (
	"context"
	"fmt"
	"log"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

func DeactivatePickers(ctx *Context) error {

	// get season to edit
	season, seasonRef, err := firestore.GetSeason(ctx, ctx.FirestoreClient, ctx.Season)
	if err != nil {
		return fmt.Errorf("DeactivatePickers: failed to get season %d: %w", ctx.Season, err)
	}

	// error checking
	toDeactivate := make(map[string]struct{})
	for _, picker := range ctx.Pickers {
		p, _, err := firestore.GetPickerByLukeName(ctx, ctx.FirestoreClient, picker.LukeName)
		if err != nil {
			return fmt.Errorf("DeactivatePickers: picker '%s' does not exist", picker.LukeName)
		}
		if _, exists := season.Pickers[picker.LukeName]; !exists {
			return fmt.Errorf("DeactivatePickers: picker '%s' not active in season %d", picker.LukeName, season.Year)
		}
		toDeactivate[p.LukeName] = struct{}{}
	}

	if ctx.DryRun {
		log.Printf("DRY RUN: would deactivate the following pickers in season %d:", season.Year)
		for picker := range toDeactivate {
			log.Printf("%s", picker)
		}
		return nil
	}

	err = ctx.FirestoreClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
		pickers := season.Pickers
		for name := range toDeactivate {
			delete(pickers, name)
		}
		err := t.Update(seasonRef, []fs.Update{{Path: "pickers", Value: &pickers}})
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("DeactivatePickers: error running transaction: %w", err)
	}
	return err
}
