package main

import (
	"context"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/tools/btspick"
)

type makePickCmd struct {
	Season int               `arg:"" help:"Season to modify. If negative, the current season will be guessed based on today's date." required:""`
	Week   int               `arg:"" help:"Week to modify. If negative, the current week will be guessed based on today's date." required:""`
	Picks  map[string]string `arg:"" help:"Mapping of streaker Luke name to comma-separated team other names picked by the streaker for the week. An empty team name value will clear the streaker's pick for the week."`
}

func (a *makePickCmd) Run(g *globalCmd) error {
	ctx := btspick.NewContext(context.Background())
	ctx.DryRun = g.DryRun
	ctx.Force = g.Force
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Season = a.Season
	ctx.Week = a.Week
	ctx.Picks = a.Picks
	return btspick.MakePicks(ctx)
}
