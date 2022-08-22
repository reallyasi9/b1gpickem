package editpickers

import (
	"context"
	"fmt"
	"log"
	"time"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

func EditPicker(ctx *Context) error {

	// error checking
	if len(ctx.Pickers) != 1 {
		return fmt.Errorf("EditPicker: expected one picker to edit, got %d", len(ctx.Pickers))
	}
	newPicker := ctx.Pickers[0]
	nilTime := time.Time{}
	if newPicker.LukeName == "" && newPicker.Name == "" && newPicker.Joined == nilTime {
		return fmt.Errorf("EditPicker: at least one field to edit must be specified")
	}

	snap, err := ctx.FirestoreClient.Collection(firestore.PICKERS_COLLECTION).Doc(ctx.ID).Get(ctx)
	if err != nil {
		return fmt.Errorf("EditPicker: error looking up picker with ID '%s': %w", ctx.ID, err)
	}

	var picker firestore.Picker
	err = snap.DataTo(&picker)
	if err != nil {
		return fmt.Errorf("EditPicker: error converting picker: %w", err)
	}

	// get seasons to edit if picker still active in seasons
	editSeasons := make(map[*fs.DocumentRef]firestore.Season)
	if newPicker.LukeName != "" && !ctx.KeepSeasons {
		seasons, seasonRefs, err := firestore.GetSeasons(ctx, ctx.FirestoreClient)
		if err != nil {
			return fmt.Errorf("EditPicker: failed to get seasons: %w", err)
		}
		for i, season := range seasons {
			if _, exists := season.Pickers[picker.LukeName]; exists {
				editSeasons[seasonRefs[i]] = season
			}
		}
	}

	if ctx.DryRun {
		log.Print("DRY RUN: would make the following edits:")
		log.Printf("%s: %s", snap.Ref.Path, picker)
		if newPicker.LukeName != "" {
			log.Printf("LukeName to '%s'", newPicker.LukeName)
		}
		if newPicker.Name != "" {
			log.Printf("Name to '%s'", newPicker.Name)
		}
		if newPicker.Joined != nilTime {
			log.Printf("Joined to '%s'", newPicker.Joined)
		}
		if !ctx.KeepSeasons {
			log.Print("DRY RUN: would edit the active user in the following seasons:")
			for _, season := range editSeasons {
				log.Printf("%d", season.Year)
			}
		}
		return nil
	}

	if !ctx.Force {
		return fmt.Errorf("EditPicker: edit of pickers is dangerous: use force flag to force edit")
	}

	err = ctx.FirestoreClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
		updates := make([]fs.Update, 0, 3)
		if newPicker.LukeName != "" {
			updates = append(updates, fs.Update{Path: "name_luke", Value: &newPicker.LukeName})
		}
		if newPicker.Name != "" {
			updates = append(updates, fs.Update{Path: "name", Value: &newPicker.Name})
		}
		if newPicker.Joined != nilTime {
			updates = append(updates, fs.Update{Path: "joined", Value: &newPicker.Joined})
		}
		err = t.Update(snap.Ref, updates)
		if err != nil {
			return err
		}

		if !ctx.KeepSeasons {
			for ref, season := range editSeasons {
				pickers := season.Pickers
				pickers[newPicker.LukeName] = pickers[picker.LukeName]
				delete(pickers, picker.LukeName)
				err = t.Update(ref, []fs.Update{{Path: "pickers", Value: &pickers}})
				if err != nil {
					return err
				}
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("EditPicker: error running transaction: %w", err)
	}
	return err
}
