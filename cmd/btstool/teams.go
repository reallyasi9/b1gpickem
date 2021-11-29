package main

import (
	"context"
	"flag"
	"log"
	"strconv"
	"strings"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
	bpefs "github.com/reallyasi9/b1gpickem/internal/firestore"
	"github.com/reallyasi9/b1gpickem/internal/tools/btsteams"
)

type addTeamsCmd struct {
	Season    int      `arg:"" help:"Season to modify. If negative, the current season will be guessed based on today's date."`
	Names     []string `arg:"" help:"Team other names to add to the competition."`
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
	ctx.TeamNames = a.Names
	ctx.Append = !a.DoNotKeep
	return btsteams.AddTeams(ctx)
}

type rmTeamsCmd struct {
	Season int      `arg:"" help:"Season to modify. If negative, the current season will be guessed based on today's date."`
	Names  []string `arg:"" help:"Team other names to remove from the competition."`
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
	ctx.TeamNames = a.Names
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

func setupTeams() {
	ctx := context.Background()

	err := teamsFlagSet.Parse(flag.Args()[1:])
	if err != nil {
		log.Fatalf("Failed to parse setup-teams arguments: %v", err)
	}

	if teamsFlagSet.NArg() < 2 {
		teamsFlagSet.Usage()
		log.Fatal("Pick types and at least one team required")
	}

	pickTypes, err := splitPickTypes(teamsFlagSet.Arg(0))
	if err != nil {
		log.Print(err)
		log.Fatalf("Failed to parse pick types: %v", err)
	}

	pickTypeWeeks := 0
	pickTypeTeams := 0
	for i, n := range pickTypes {
		pickTypeWeeks += n
		pickTypeTeams += i * n
	}

	if teamsFlagSet.NArg()-1 != pickTypeTeams {
		log.Printf("WARNING: Total teams inferred by pick types (%d) not equal to number of teams in competition (%d)", pickTypeTeams, teamsFlagSet.NArg()-1)
	}

	fsclient, err := firestore.NewClient(ctx, ProjectID)
	if err != nil {
		log.Print(err)
		log.Fatalf("Check that the project ID \"%s\" is correctly specified (either the -project flag or the GCP_PROJECT environment variable)", ProjectID)
	}

	season, seasonRef, err := bpefs.GetSeason(ctx, fsclient, teamsSeason)
	if err != nil {
		log.Fatal(err)
	}

	weeks, err := seasonRef.Collection("weeks").DocumentRefs(ctx).GetAll()
	if err != nil {
		log.Fatalf("Unable to check number of weeks in season %d: %v", teamsSeason, err)
	}
	if len(weeks) != pickTypeWeeks {
		log.Printf("WARNING: Total weeks inferred by pick types (%d) not equal to number of weeks in season (%d)", pickTypeWeeks, len(weeks))
	}

	teams, teamRefs, err := bpefs.GetTeams(ctx, seasonRef)
	if err != nil {
		panic(err)
	}
	teamRefsByOtherName := bpefs.NewTeamRefsByOtherName(teams, teamRefs)

	streakTeams := make([]*firestore.DocumentRef, teamsFlagSet.NArg()-1)
	for i, name := range teamsFlagSet.Args()[1:] {
		var teamRef *firestore.DocumentRef
		var ok bool
		if teamRef, ok = teamRefsByOtherName[name]; !ok {
			log.Fatalf("Team '%s' not found", name)
		}
		streakTeams[i] = teamRef
	}

	if DryRun {
		log.Print("DRY RUN: would write the following to Firestore:")
		log.Printf("StreakTeams: %v", streakTeams)
		log.Printf("StreakPickTypes: %v", pickTypes)
		return
	}
	if len(season.StreakTeams) > 0 && !Force {
		log.Fatal("Refusing to overwrite streak teams remaining: pass -force flag to override this behavior")
	}
	season.StreakTeams = streakTeams
	season.StreakPickTypes = pickTypes
	_, err = seasonRef.Set(ctx, &season)
	if err != nil {
		log.Fatal(err)
	}
}

func splitPickTypes(arg string) ([]int, error) {
	values := strings.Split(arg, ":")
	out := make([]int, len(values))
	for i, val := range values {
		var parsed int
		var err error
		if parsed, err = strconv.Atoi(val); err != nil {
			return nil, err
		}
		out[i] = parsed
	}
	return out, nil
}
