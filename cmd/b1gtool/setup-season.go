package main

import (
	"context"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/tools/setupseason"
)

type setupSeasonCmd struct {
	DryRun bool   `help:"Print database writes to log and exit without writing."`
	Force  bool   `help:"Force overwrite or delete data from datastore."`
	ApiKey string `arg:"" help:"CollegeFootballData.com API key." required:""`
	Season int    `arg:"" help:"Season ID to create." required:""`
	Week   []int  `help:"Weeks to update."`
}

func (a *setupSeasonCmd) Run(g *globalCmd) error {
	ctx := setupseason.NewContext(context.Background())
	ctx.DryRun = a.DryRun
	ctx.Force = a.Force
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.ApiKey = a.ApiKey
	ctx.Season = a.Season
	ctx.Weeks = a.Week
	return setupseason.SetupSeason(ctx)
}
