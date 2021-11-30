package main

import (
	"context"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/tools/btsweeks"
)

type setupWeeksCmd struct {
	Season int   `arg:"" help:"Season to modify. If negative, the current season will be guessed based on today's date."`
	Types  []int `arg:"" help:"Number of pick types in order. The first argument is the number of byes, the second is the number of single-pick weeks, the third is the number of double-downs, etc."`
}

func (a *setupWeeksCmd) Run(g *globalCmd) error {
	ctx := btsweeks.NewContext(context.Background())
	ctx.DryRun = g.DryRun
	ctx.Force = g.Force
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Season = a.Season
	ctx.WeekTypes = a.Types
	return btsweeks.SetWeekTypes(ctx)
}
