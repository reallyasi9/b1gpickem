package main

import (
	"context"
	"time"

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

type splitWeekCmd struct {
	DryRun        bool      `help:"Print database writes to log and exit without writing."`
	Force         bool      `help:"Force overwrite or delete data from datastore."`
	Season        int       `arg:"" help:"Season ID to create." required:""`
	WeekToSplit   int       `arg:"" help:"Week to split." required:""`
	SplitTimeFrom time.Time `arg:"" help:"Split time. Games that kickoff after this time (inclusive) will be split into a new week." required:"" format:"2006/01/02 15:04:05 MST"`
	SplitTimeTo   time.Time `arg:"" help:"Split time. Games that kickoff before this time (exclusive) will be split into a new week." required:"" format:"2006/01/02 15:04:05 MST"`
	NewWeekNumber int       `arg:"" help:"New week number." required:""`
}

func (a *splitWeekCmd) Run(g *globalCmd) error {
	ctx := setupseason.NewContext(context.Background())
	ctx.DryRun = a.DryRun
	ctx.Force = a.Force
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Season = a.Season
	ctx.SplitWeek = a.WeekToSplit
	ctx.SplitTimeFrom = a.SplitTimeFrom
	ctx.SplitTimeTo = a.SplitTimeTo
	ctx.NewWeekNumber = a.NewWeekNumber
	return setupseason.SplitWeek(ctx)
}
