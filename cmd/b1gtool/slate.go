package main

import (
	"context"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/tools/parseslate"
)

type parseSlateCmd struct {
	DryRun bool   `help:"Print database writes to log and exit without writing."`
	Force  bool   `help:"Force overwrite or delete data from datastore."`
	Season int    `arg:"" help:"Season of slate." required:""`
	Week   int    `arg:"" help:"Week of slate." required:""`
	Slate  string `arg:"" help:"Path to slate. Can be either a local path or a Google Storage URL starting with 'gs://'." required:""`
}

func (a *parseSlateCmd) Run(g *globalCmd) error {
	ctx := parseslate.NewContext(context.Background())
	ctx.DryRun = a.DryRun
	ctx.Force = a.Force
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Season = a.Season
	ctx.Week = a.Week
	ctx.Slate = a.Slate
	return parseslate.ParseSlate(ctx)
}
