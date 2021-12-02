package main

import (
	"context"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/tools/updatemodels"
)

type updateModelsCmd struct {
	DryRun bool `help:"Print database writes to log and exit without writing."`
	Force  bool `help:"Force overwrite or delete data from datastore."`
	Season int  `arg:"" help:"Season ID to update." required:""`
	Week   int  `help:"Week of update."`
}

func (a *updateModelsCmd) Run(g *globalCmd) error {
	ctx := updatemodels.NewContext(context.Background())
	ctx.DryRun = a.DryRun
	ctx.Force = a.Force
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Season = a.Season
	ctx.Week = a.Week
	return updatemodels.UpdateModels(ctx)
}

type getPredictionsCmd struct {
	DryRun bool `help:"Print database writes to log and exit without writing."`
	Force  bool `help:"Force overwrite or delete data from datastore."`
	Season int  `arg:"" help:"Season ID to update." required:""`
	Week   int  `arg:"" help:"Week of update." required:""`
}

func (a *getPredictionsCmd) Run(g *globalCmd) error {
	ctx := updatemodels.NewContext(context.Background())
	ctx.DryRun = a.DryRun
	ctx.Force = a.Force
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Season = a.Season
	ctx.Week = a.Week
	return updatemodels.GetPredictions(ctx)
}

type updateSagarinCmd struct {
	DryRun bool `help:"Print database writes to log and exit without writing."`
	Force  bool `help:"Force overwrite or delete data from datastore."`
	Season int  `arg:"" help:"Season ID to update." required:""`
	Week   int  `arg:"" help:"Week of update." required:""`
}

func (a *updateSagarinCmd) Run(g *globalCmd) error {
	ctx := updatemodels.NewContext(context.Background())
	ctx.DryRun = a.DryRun
	ctx.Force = a.Force
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Season = a.Season
	ctx.Week = a.Week
	return updatemodels.UpdateSagarin(ctx)
}

type addModelsCmd struct {
	DryRun bool              `help:"Print database writes to log and exit without writing."`
	Force  bool              `help:"Force overwrite or delete data from datastore."`
	Models map[string]string `arg:"" help:"Map of system (long) name keys to model (short) name values to add." required:""`
}

func (a *addModelsCmd) Run(g *globalCmd) error {
	ctx := updatemodels.NewContext(context.Background())
	ctx.DryRun = a.DryRun
	ctx.Force = a.Force
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	for sys, name := range a.Models {
		ctx.ModelNames = append(ctx.ModelNames, name)
		ctx.SystemNames = append(ctx.SystemNames, sys)
	}
	return updatemodels.AddModels(ctx)
}

type rmModelsCmd struct {
	DryRun bool     `help:"Print database writes to log and exit without writing."`
	Force  bool     `help:"Force overwrite or delete data from datastore."`
	Models []string `arg:"" help:"Model (short) names to delete." required:""`
}

func (a *rmModelsCmd) Run(g *globalCmd) error {
	ctx := updatemodels.NewContext(context.Background())
	ctx.DryRun = a.DryRun
	ctx.Force = a.Force
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.ModelNames = append(ctx.ModelNames, a.Models...)
	return updatemodels.RmModels(ctx)
}

type lsModelsCmd struct {
}

func (a *lsModelsCmd) Run(g *globalCmd) error {
	ctx := updatemodels.NewContext(context.Background())
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	return updatemodels.LsModels(ctx)
}
