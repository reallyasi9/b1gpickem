package btsteams

import (
	"context"
	"fmt"
	"log"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

func RmTeams(ctx *Context) error {
	season, seasonRef, err := firestore.GetSeason(ctx.Context, ctx.FirestoreClient, ctx.Season)
	if err != nil {
		return fmt.Errorf("RmTeams: failed to get season %d: %w", ctx.Season, err)
	}
	teams, teamRefs, err := firestore.GetTeams(ctx.Context, seasonRef)
	if err != nil {
		return fmt.Errorf("RmTeams: failed to get teams: %w", err)
	}
	lookup := firestore.NewTeamRefsByOtherName(teams, teamRefs)

	teamsToKeep := make(map[string]*fs.DocumentRef)
	for _, ref := range season.StreakTeams {
		teamsToKeep[ref.ID] = ref
	}
	for _, name := range ctx.TeamNames {
		ref, found := lookup[name]
		if !found {
			return fmt.Errorf("RmTeams: failed to find team with other name '%s'", name)
		}
		delete(teamsToKeep, ref.ID)
	}

	if ctx.DryRun {
		log.Printf("DRY RUN: would set the following streak teams for season %d:", ctx.Season)
		for id := range teamsToKeep {
			log.Print(id)
		}
		return nil
	}

	if !ctx.Force {
		return fmt.Errorf("RmTeams: refusing to overwrite streak teams: explicitly override with --force argument")
	}
	// one last error check: count teams and weeks and compare
	nPicks := 0
	for typ, n := range season.StreakPickTypes {
		nPicks += typ * n
	}
	if nPicks != len(teamsToKeep) {
		return fmt.Errorf("RmTeams: number of teams (%d) not equal to number of streak picks calculated from week types (%d): explicitly override with --force argument", len(teamsToKeep), nPicks)
	}

	newTeams := make([]*fs.DocumentRef, 0, len(teamsToKeep))
	for _, ref := range teamsToKeep {
		newTeams = append(newTeams, ref)
	}
	err = ctx.FirestoreClient.RunTransaction(ctx.Context, func(c context.Context, t *fs.Transaction) error {
		return t.Update(seasonRef, []fs.Update{{Path: "streak_teams", Value: &newTeams}})
	})

	if err != nil {
		return fmt.Errorf("RmTeams: failed to execute transaction: %w", err)
	}
	return nil
}
