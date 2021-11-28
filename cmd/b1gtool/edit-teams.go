package main

import (
	"context"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
	"github.com/reallyasi9/b1gpickem/internal/tools/editteams"
)

type editTeamCmd struct {
	DryRun       bool     `help:"Print database writes to log and exit without writing." xor:"Force,DryRun"`
	Force        bool     `help:"Force overwriting or deleting data in database." xor:"Force,DryRun"`
	Season       int      `help:"Season where team is defined." required:""`
	ID           string   `arg:"" help:"ID of team to edit." required:""`
	Abbreviation string   `help:"Team 4-letter abbreviation."`
	School       string   `help:"Team school name."`
	Mascot       string   `help:"Team mascot name."`
	ShortName    []string `help:"Team short (Luke) name."`
	OtherName    []string `help:"Team other (model) name."`
	Color        []string `help:"Team color string."`
	Logo         []string `help:"Location of team logo."`
	Append       bool     `help:"Append short names, other names, colors, and logos to extant values."`
}

func (a *editTeamCmd) Run(g *globalCmd) error {
	ctx := editteams.NewContext(context.Background())
	ctx.DryRun = a.DryRun
	ctx.Force = a.Force
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Season = a.Season
	ctx.ID = a.ID
	ctx.Team = firestore.Team{
		Abbreviation: a.Abbreviation,
		School:       a.School,
		Mascot:       a.Mascot,
		ShortNames:   a.ShortName,
		OtherNames:   a.OtherName,
		Colors:       a.Color,
		Logos:        a.Logo,
	}
	ctx.Append = a.Append
	return editteams.EditTeam(ctx)
}
