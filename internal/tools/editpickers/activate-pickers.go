package editpickers

import (
	"context"
	"fmt"
	"log"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

func ActivatePickers(ctx *Context) error {

	// get season to edit
	season, seasonRef, err := firestore.GetSeason(ctx, ctx.FirestoreClient, ctx.Season)
	if err != nil {
		return fmt.Errorf("ActivatePickers: failed to get season %d: %w", ctx.Season, err)
	}

	// error checking
	toActivate := make(map[string]*fs.DocumentRef)
	for _, picker := range ctx.Pickers {
		p, ref, err := firestore.GetPickerByLukeName(ctx, ctx.FirestoreClient, picker.LukeName)
		if err != nil {
			return fmt.Errorf("ActivatePickers: picker '%s' does not exist", picker.LukeName)
		}
		if _, exists := season.Pickers[picker.LukeName]; exists {
			return fmt.Errorf("ActivatePickers: picker '%s' already active in season %d", picker.LukeName, season.Year)
		}
		toActivate[p.LukeName] = ref
	}

	if ctx.DryRun {
		log.Printf("DRY RUN: would activate the following pickers in season %d:", season.Year)
		for picker := range toActivate {
			log.Printf("%s", picker)
		}
		return nil
	}

	err = ctx.FirestoreClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
		pickers := season.Pickers
		for name, ref := range toActivate {
			pickers[name] = ref
		}
		err := t.Update(seasonRef, []fs.Update{{Path: "pickers", Value: &pickers}})
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("ActivatePickers: error running transaction: %w", err)
	}
	return err
}
