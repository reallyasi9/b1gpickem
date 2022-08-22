package editpickers

import (
	"context"
	"fmt"
	"log"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

func RmPickers(ctx *Context) error {

	// error checking
	toRm := make(map[string]firestore.Picker)
	for _, picker := range ctx.Pickers {
		p, ref, err := firestore.GetPickerByLukeName(ctx, ctx.FirestoreClient, picker.LukeName)
		if err != nil {
			return fmt.Errorf("RmPickers: picker '%s' does not exist", picker)
		}
		toRm[ref.ID] = p
	}

	// get seasons to edit if pickers still active in seasons
	editSeasons := make(map[*fs.DocumentRef]firestore.Season)
	if !ctx.KeepSeasons {
		seasons, seasonRefs, err := firestore.GetSeasons(ctx, ctx.FirestoreClient)
		if err != nil {
			return fmt.Errorf("RmPickers: failed to get seasons: %w", err)
		}
		for i, season := range seasons {
			for _, picker := range toRm {
				if _, exists := season.Pickers[picker.LukeName]; exists {
					editSeasons[seasonRefs[i]] = season
				}
			}
		}
	}

	if ctx.DryRun {
		log.Print("DRY RUN: would delete the following pickers:")
		for _, picker := range toRm {
			log.Printf("%s", picker)
		}
		if !ctx.KeepSeasons {
			log.Print("DRY RUN: would deactivate pickers from the following seasons:")
			for _, season := range editSeasons {
				log.Printf("%d", season.Year)
			}
		}
		return nil
	}

	if !ctx.Force {
		return fmt.Errorf("RmPickers: removal of pickers is dangerous: use force flag to force removal")
	}

	pickerCol := ctx.FirestoreClient.Collection(firestore.PICKERS_COLLECTION)
	err := ctx.FirestoreClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
		for ref, season := range editSeasons {
			pickers := season.Pickers
			for _, picker := range toRm {
				delete(pickers, picker.LukeName)
			}
			err := t.Update(ref, []fs.Update{{Path: "pickers", Value: &pickers}})
			if err != nil {
				return err
			}
		}
		for id := range toRm {
			ref := pickerCol.Doc(id)
			err := t.Delete(ref)
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("RmPickers: error running transaction: %w", err)
	}
	return err
}
