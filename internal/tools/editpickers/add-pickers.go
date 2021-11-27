package editpickers

import (
	"context"
	"fmt"
	"log"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

func AddPickers(ctx *Context) error {

	// error checking
	for _, picker := range ctx.Pickers {
		_, _, err := firestore.GetPickerByLukeName(ctx, ctx.FirestoreClient, picker.LukeName)
		if err == nil {
			return fmt.Errorf("AddPickers: picker '%s' already exists, try EditPickers instead", picker)
		}
		if _, ok := err.(firestore.PickerNotFound); !ok {
			return fmt.Errorf("AddPickers: error looking up picker '%s': %w", picker, err)
		}
	}

	if ctx.DryRun {
		log.Print("DRY RUN: would add the following pickers:")
		for _, picker := range ctx.Pickers {
			log.Printf("%s", picker)
		}
		return nil
	}

	pickerCol := ctx.FirestoreClient.Collection(firestore.PICKERS_COLLECTION)
	err := ctx.FirestoreClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
		for _, picker := range ctx.Pickers {
			ref := pickerCol.Doc(picker.LukeName)
			err := t.Create(ref, &picker)
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("AddPickers: error running transaction: %w", err)
	}
	return err
}
