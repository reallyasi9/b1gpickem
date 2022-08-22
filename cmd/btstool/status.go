package main

import (
	"context"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/tools/btsstatus"
)

type statusCmd struct {
	Season int `arg:"" help:"Season to modify. If negative, the current season will be guessed based on today's date."`
}

func (a *statusCmd) Run(g *globalCmd) error {
	ctx := btsstatus.NewContext(context.Background())
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Season = a.Season
	return btsstatus.PrintStatus(ctx)
}
