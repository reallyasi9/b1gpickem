package main

import (
	"context"
	"time"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
	"github.com/reallyasi9/b1gpickem/internal/tools/editpickers"
)

type addPickersCmd struct {
	DryRun  bool               `help:"Print database writes to log and exit without writing."`
	Pickers []firestore.Picker `arg:"" help:"Pickers to add. Must be strings in LukeName[:FullName[:JoinDate]] format." required:""`
}

func (a *addPickersCmd) Run(g *globalCmd) error {
	ctx := editpickers.NewContext(context.Background())
	ctx.DryRun = a.DryRun
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Pickers = a.Pickers
	return editpickers.AddPickers(ctx)
}

type rmPickersCmd struct {
	Force       bool     `help:"Force overwriting or deleting data in database." xor:"Force,DryRun"`
	DryRun      bool     `help:"Print database writes to log and exit without writing." xor:"Force,DryRun"`
	KeepSeasons bool     `help:"Keep removed pickers active in seasons. WARNING: Specifying this option will leave undefined references in the database!"`
	Pickers     []string `arg:"" help:"LukeNames of pickers to remove." required:""`
}

func (a *rmPickersCmd) Run(g *globalCmd) error {
	ctx := editpickers.NewContext(context.Background())
	ctx.DryRun = a.DryRun
	ctx.Force = a.Force
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	pickers := make([]firestore.Picker, len(a.Pickers))
	for i, picker := range a.Pickers {
		pickers[i] = firestore.Picker{LukeName: picker}
	}
	ctx.Pickers = pickers
	return editpickers.RmPickers(ctx)
}

type lsPickersCmd struct{}

func (a *lsPickersCmd) Run(g *globalCmd) error {
	ctx := editpickers.NewContext(context.Background())
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	return editpickers.LsPickers(ctx)
}

type editPickerCmd struct {
	Force       bool      `help:"Force overwriting or deleting data in database." xor:"Force,DryRun"`
	DryRun      bool      `help:"Print database writes to log and exit without writing." xor:"Force,DryRun"`
	KeepSeasons bool      `help:"Do not update active pickers in seasons if LukeName changes. WARNING: Specifying this option will leave undefined references in the database!"`
	ID          string    `arg:"" help:"Database ID of picker to edit." required:""`
	LukeName    string    `help:"New LukeName for picker."`
	Name        string    `help:"New Name for picker."`
	JoinDate    time.Time `help:"New JoinDate for picker." format:"2006/01/02"`
}

func (a *editPickerCmd) Run(g *globalCmd) error {
	ctx := editpickers.NewContext(context.Background())
	ctx.DryRun = a.DryRun
	ctx.Force = a.Force
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	pickers := []firestore.Picker{
		{
			LukeName: a.LukeName,
			Name:     a.Name,
			Joined:   a.JoinDate,
		},
	}
	ctx.Pickers = pickers
	ctx.ID = a.ID
	return editpickers.EditPicker(ctx)
}

type activatePickersCmd struct {
	DryRun  bool     `help:"Print database writes to log and exit without writing."`
	Season  int      `arg:"" help:"Season in which to activate pickers" required:""`
	Pickers []string `arg:"" help:"LukeNames of pickers to activate." required:""`
}

func (a *activatePickersCmd) Run(g *globalCmd) error {
	ctx := editpickers.NewContext(context.Background())
	ctx.DryRun = a.DryRun
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	pickers := make([]firestore.Picker, len(a.Pickers))
	for i, picker := range a.Pickers {
		pickers[i] = firestore.Picker{LukeName: picker}
	}
	ctx.Pickers = pickers
	ctx.Season = a.Season
	return editpickers.ActivatePickers(ctx)
}

type deactivatePickersCmd struct {
	DryRun  bool     `help:"Print database writes to log and exit without writing."`
	Season  int      `arg:"" help:"Season from which to deactivate pickers" required:""`
	Pickers []string `arg:"" help:"LukeNames of pickers to deactivate." required:""`
}

func (a *deactivatePickersCmd) Run(g *globalCmd) error {
	ctx := editpickers.NewContext(context.Background())
	ctx.DryRun = a.DryRun
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	pickers := make([]firestore.Picker, len(a.Pickers))
	for i, picker := range a.Pickers {
		pickers[i] = firestore.Picker{LukeName: picker}
	}
	ctx.Pickers = pickers
	ctx.Season = a.Season
	return editpickers.DeactivatePickers(ctx)
}

var editPickersCLI struct {
	globalCmd

	Pickers struct {
		Add        addPickersCmd        `cmd:"" help:"Add pickers."`
		Rm         rmPickersCmd         `cmd:"" help:"Remove pickers."`
		Ls         lsPickersCmd         `cmd:"" help:"List all pickers."`
		Edit       editPickerCmd        `cmd:"" help:"Edit picker."`
		Activate   activatePickersCmd   `cmd:"" help:"Activate pickers for a season."`
		Deactivate deactivatePickersCmd `cmd:"" help:"Deactivate pickers for a season."`
	} `cmd:""`
}
