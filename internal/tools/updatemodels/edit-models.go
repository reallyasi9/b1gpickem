package updatemodels

import (
	"context"
	"fmt"
	"log"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

func AddModels(ctx *Context) error {

	toWrite := make(map[*fs.DocumentRef]firestore.Model)
	for i, systemName := range ctx.SystemNames {
		shortName := ctx.ModelNames[i]

		model := firestore.Model{
			System:    systemName,
			ShortName: shortName,
		}
		ref := ctx.FirestoreClient.Collection(firestore.MODELS_COLLECTION).Doc(shortName)
		toWrite[ref] = model
	}

	if ctx.DryRun {
		log.Printf("DRY RUN: would write the following to firestore:")
		for ref, model := range toWrite {
			log.Printf("%s -> %v", ref.Path, model)
		}

		return nil
	}

	err := ctx.FirestoreClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
		var err error
		for ref, model := range toWrite {
			if ctx.Force {
				err = t.Set(ref, &model)
			} else {
				err = t.Create(ref, &model)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("AddModels: failed to run transaction: %w", err)
	}
	return nil
}

func RmModels(ctx *Context) error {

	models, modelRefs, err := firestore.GetModels(ctx, ctx.FirestoreClient)
	if err != nil {
		return fmt.Errorf("RmModels: failed to get models: %w", err)
	}

	lookup := firestore.NewModelRefsByShortName(models, modelRefs)
	toRm := make([]*fs.DocumentRef, 0)
	for _, name := range ctx.ModelNames {
		ref, ok := lookup[name]
		if !ok {
			return fmt.Errorf("RmModels: model with short name '%s' not found", name)
		}
		toRm = append(toRm, ref)
	}

	if ctx.DryRun {
		log.Printf("DRY RUN: would delete the following to firestore:")
		for _, ref := range toRm {
			log.Printf("%s", ref.Path)
		}

		return nil
	}

	if !ctx.Force {
		return fmt.Errorf("RmModels: refusing to delete models without --force flag")
	}

	err = ctx.FirestoreClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
		for _, ref := range toRm {
			err := t.Delete(ref)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("RmModels: failed to run transaction: %w", err)
	}
	return nil
}

func LsModels(ctx *Context) error {

	models, _, err := firestore.GetModels(ctx, ctx.FirestoreClient)
	if err != nil {
		return fmt.Errorf("LsModels: failed to get models: %w", err)
	}

	for _, model := range models {
		fmt.Printf("%s\n", model)
	}

	return nil
}
