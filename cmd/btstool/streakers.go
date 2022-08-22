package main

import (
	"context"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/tools/btsstreakers"
)

type activateStreakersCmd struct {
	Season int      `arg:"" help:"Season to modify. If negative, the current season will be guessed based on today's date."`
	Name   []string `arg:"" help:"Streaker Luke name to activate in the competition."`
}

func (a *activateStreakersCmd) Run(g *globalCmd) error {
	ctx := btsstreakers.NewContext(context.Background())
	ctx.DryRun = g.DryRun
	ctx.Force = g.Force
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Season = a.Season
	ctx.StreakerNames = a.Name
	return btsstreakers.ActivateStreakers(ctx)
}

type deactivateStreakersCmd struct {
	Season int      `arg:"" help:"Season to modify. If negative, the current season will be guessed based on today's date."`
	Name   []string `arg:"" help:"Streaker Luke name to deactivate from the competition."`
}

func (a *deactivateStreakersCmd) Run(g *globalCmd) error {
	ctx := btsstreakers.NewContext(context.Background())
	ctx.DryRun = g.DryRun
	ctx.Force = g.Force
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Season = a.Season
	ctx.StreakerNames = a.Name
	return btsstreakers.DeactivateStreakers(ctx)
}

type lsStreakersCmd struct {
	Season int `arg:"" help:"Season to list. If negative, the current season will be guessed based on today's date."`
}

func (a *lsStreakersCmd) Run(g *globalCmd) error {
	ctx := btsstreakers.NewContext(context.Background())
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Season = a.Season
	return btsstreakers.LsStreakers(ctx)
}
