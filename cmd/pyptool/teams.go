package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/tools/pypteams"
)

type addTeamsCmd struct {
	Season    int      `arg:"" help:"Season to modify. If negative, the current season will be guessed based on today's date."`
	TeamWin   []string `arg:"" help:"Teams and pre-season predicted wins to add to the competition. Add in OtherName:PreseasonWins format, with negative PreseasonWins for top 25 teams."`
	DoNotKeep bool     `help:"Remove all teams from competition that are not supplied to this command."`
}

func parseTeamWin(s string) (string, float64, error) {
	splits := strings.Split(s, ":")
	if len(splits) != 2 {
		return "", 0, fmt.Errorf("unable to parse Name:Wins from string '%s'", s)
	}
	name := splits[0]
	wins, err := strconv.ParseFloat(splits[1], 64)
	if err != nil {
		return "", 0, fmt.Errorf("unable to parse wins portion of string '%s': %w", s, err)
	}

	return name, wins, nil
}

func (a *addTeamsCmd) Run(g *globalCmd) error {
	ctx := pypteams.NewContext(context.Background())
	ctx.DryRun = g.DryRun
	ctx.Force = g.Force
	var err error
	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
	if err != nil {
		return err
	}
	ctx.Season = a.Season
	teamWins := make(map[string]float64)
	for _, s := range a.TeamWin {
		name, wins, err := parseTeamWin(s)
		if err != nil {
			return err
		}
		teamWins[name] = wins
	}
	ctx.TeamNameWins = teamWins
	ctx.Append = !a.DoNotKeep
	return pypteams.AddTeams(ctx)
}

// type rmTeamsCmd struct {
// 	Season int      `arg:"" help:"Season to modify. If negative, the current season will be guessed based on today's date."`
// 	Name   []string `arg:"" help:"Team other name to remove from the competition."`
// }

// func (a *rmTeamsCmd) Run(g *globalCmd) error {
// 	ctx := pypteams.NewContext(context.Background())
// 	ctx.DryRun = g.DryRun
// 	ctx.Force = g.Force
// 	var err error
// 	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
// 	if err != nil {
// 		return err
// 	}
// 	ctx.Season = a.Season
// 	ctx.TeamNames = a.Name
// 	return pypteams.RmTeams(ctx)
// }

// type lsTeamsCmd struct {
// 	Season int `arg:"" help:"Season to list. If negative, the current season will be guessed based on today's date."`
// }

// func (a *lsTeamsCmd) Run(g *globalCmd) error {
// 	ctx := pypteams.NewContext(context.Background())
// 	var err error
// 	ctx.FirestoreClient, err = fs.NewClient(ctx.Context, g.ProjectID)
// 	if err != nil {
// 		return err
// 	}
// 	ctx.Season = a.Season
// 	return pypteams.LsTeams(ctx)
// }
