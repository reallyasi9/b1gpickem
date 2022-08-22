package main

import (
	"context"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/tools/btsteams"
)

type addTeamsCmd struct {
	Season    int      `arg:"" help:"Season to modify. If negative, the current season will be guessed based on today's date."`
	Name      []string `arg:"" help:"Team other name to add to the competition."`
	DoNotKeep bool     `help:"Remove all teams from competition that are not supplied to this command."`
}

func (a *addTeamsCmd) Run(g *globalCmd) error {
	ctx := btsteams.NewContext(context.Background())
	ctx.DryRun = g.DryRun
	ctx.Force = g.Force
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Season = a.Season
	ctx.TeamNames = a.Name
	ctx.Append = !a.DoNotKeep
	return btsteams.AddTeams(ctx)
}

type rmTeamsCmd struct {
	Season int      `arg:"" help:"Season to modify. If negative, the current season will be guessed based on today's date."`
	Name   []string `arg:"" help:"Team other name to remove from the competition."`
}

func (a *rmTeamsCmd) Run(g *globalCmd) error {
	ctx := btsteams.NewContext(context.Background())
	ctx.DryRun = g.DryRun
	ctx.Force = g.Force
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Season = a.Season
	ctx.TeamNames = a.Name
	return btsteams.RmTeams(ctx)
}

type lsTeamsCmd struct {
	Season int `arg:"" help:"Season to list. If negative, the current season will be guessed based on today's date."`
}

func (a *lsTeamsCmd) Run(g *globalCmd) error {
	ctx := btsteams.NewContext(context.Background())
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Season = a.Season
	return btsteams.LsTeams(ctx)
}
