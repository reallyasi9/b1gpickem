package main

import (
	"context"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/bts/pyp"
)

type simulateCmd struct {
	Season int `arg:"" help:"Season to simulate. If negative, the current season will be guessed based on today's date."`

	Seed       int64 `help:"Random seed. Negative values will use the system clock to seed the RNG." default:"-1"`
	Workers    int   `help:"Number of season workers per simulation." short:"n" default:"1"`
	Iterations int   `help:"Number of seasons to simulate worker." short:"i" default:"1000000"`
}

func (a *simulateCmd) Run(g *globalCmd) error {
	ctx := pyp.NewContext(context.Background())
	// ctx.DryRun = g.DryRun
	// ctx.Force = g.Force
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Season = a.Season
	ctx.Seed = a.Seed
	ctx.Workers = a.Workers
	ctx.Iterations = a.Iterations
	return pyp.Simulate(ctx)
}
