package main

import (
	"context"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/bts/enumerate"
	"github.com/reallyasi9/b1gpickem/internal/bts/posteriors"
	"github.com/reallyasi9/b1gpickem/internal/bts/sa"
	"github.com/reallyasi9/b1gpickem/internal/bts/whatif"
)

type annealCmd struct {
	Season    int      `arg:"" help:"Season to simulate. If negative, the current season will be guessed based on today's date."`
	Week      int      `arg:"" help:"Week to simulate. If negative, the current week will be guessed based on today's date."`
	Streakers []string `arg:"" help:"Streakers to simulate." xor:"streakers" required:""`

	All bool `help:"Ignore streakers list and simulate all registered pickers still streaking in the given week." xor:"streakers" required:""`

	Seed        int64   `help:"Random seed. Negative values will use the system clock to seed the RNG." default:"-1"`
	Workers     int     `help:"Number of workers per simulated streaker." short:"n" default:"1"`
	Iterations  int     `help:"Number of simulated annealing iterations per worker." short:"i" default:"1000000"`
	WanderLimit int     `help:"Number of iterations to allow solution to wander from the best discovered before being reset to the best solution." short:"w" default:"10000"`
	C           float64 `help:"Simulated annealing temperature linear constant: p = (C * (Iterations - i) / Iterations)^E." default:"1"`
	E           float64 `help:"Simulated annealing temperature exponent: p = (C * (Iterations - i) / Iterations)^E." default:"3"`
}

func (a *annealCmd) Run(g *globalCmd) error {
	ctx := sa.NewContext(context.Background())
	ctx.DryRun = g.DryRun
	ctx.Force = g.Force
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Season = a.Season
	ctx.Week = a.Week
	ctx.Streakers = a.Streakers
	ctx.All = a.All
	ctx.Seed = a.Seed
	ctx.Workers = a.Workers
	ctx.Iterations = a.Iterations
	ctx.WanderLimit = a.WanderLimit
	ctx.C = a.C
	ctx.E = a.E
	return sa.Anneal(ctx)
}

type enumerateCmd struct {
	Season int `arg:"" help:"Season to simulate. If negative, the current season will be guessed based on today's date."`
}

func (a *enumerateCmd) Run(g *globalCmd) error {
	ctx := enumerate.NewContext(context.Background())
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Season = a.Season
	ctx.NoProgress = g.NoProgress
	return enumerate.Enumerate(ctx)
}

type posteriorsCmd struct {
	Season int      `arg:"" help:"Season to simulate. If negative, the current season will be guessed based on today's date."`
	Week   int      `arg:"" help:"Week to simulate. If negative, the current week will be guessed based on today's date."`
	Teams  []string `arg:"" help:"Teams to simulate."`

	Seed         int64 `help:"Random seed. Negative values will use the system clock to seed the RNG." default:"-1"`
	Iterations   int   `help:"Number of seasons to stimulate." short:"i" default:"10000"`
	Championship bool  `help:"Pit the two highest-performing teams against each other in an extra championship game." short:"c"`
}

func (a *posteriorsCmd) Run(g *globalCmd) error {
	ctx := posteriors.NewContext(context.Background())
	ctx.DryRun = g.DryRun
	ctx.Force = g.Force
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Season = a.Season
	ctx.Week = a.Week
	ctx.Teams = a.Teams
	ctx.Seed = a.Seed
	ctx.Iterations = a.Iterations
	ctx.Championship = a.Championship
	return posteriors.Posteriors(ctx)
}

type whatIfCmd struct {
	Season   int    `arg:"" help:"Season to simulate. If negative, the current season will be guessed based on today's date."`
	Week     int    `arg:"" help:"Week to simulate. If negative, the current week will be guessed based on today's date."`
	Team1    string `arg:"" help:"First team to simulate."`
	Team2    string `arg:"" help:"Second team to simulate."`
	Location string `help:"Location of game relative to first team (home, near, neutral, far, or away.)" short:"l" enum:"home,near,neutral,far,away" default:"home"`
}

func (a *whatIfCmd) Run(g *globalCmd) error {
	ctx := whatif.NewContext(context.Background())
	ctx.DryRun = g.DryRun
	ctx.Force = g.Force
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Season = a.Season
	ctx.Week = a.Week
	ctx.Team1 = a.Team1
	ctx.Team2 = a.Team2
	ctx.Location = a.Location
	return whatif.WhatIf(ctx)
}
