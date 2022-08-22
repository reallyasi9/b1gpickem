package main

import (
	"context"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/tools/pickem"
)

type pickemCmd struct {
	DryRun   bool     `help:"Print database writes to log and exit without writing."`
	Force    bool     `help:"Force overwrite or delete data from datastore."`
	Season   int      `arg:"" help:"Season of slate." required:""`
	Week     int      `arg:"" help:"Week of slate." required:""`
	Picker   string   `arg:"" help:"Picker." required:""`
	Picks    []string `arg:"" help:"Names of teams to pick."`
	SuperDog string   `help:"Superdog pick."`
}

func (a *pickemCmd) Run(g *globalCmd) error {
	ctx := pickem.NewContext(context.Background())
	ctx.DryRun = a.DryRun
	ctx.Force = a.Force
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Season = a.Season
	ctx.Week = a.Week
	ctx.Picker = a.Picker
	ctx.Picks = a.Picks
	ctx.SuperDog = a.SuperDog
	return pickem.Pickem(ctx)
}

type exportPicksCmd struct {
	Season int    `arg:"" help:"Season of slate." required:""`
	Week   int    `arg:"" help:"Week of slate." required:""`
	Picker string `arg:"" help:"Picker." required:""`
	Output string `help:"Output path. Can be a local path or a Google Storage path prefixed by 'gs://'. Default: print to stdout."`
}

func (a *exportPicksCmd) Run(g *globalCmd) error {
	ctx := pickem.NewContext(context.Background())
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Season = a.Season
	ctx.Week = a.Week
	ctx.Picker = a.Picker
	ctx.Output = a.Output
	return pickem.ExportPicks(ctx)
}
