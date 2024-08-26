package pypteams

import (
	"context"
	"fmt"
	"log"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

// AddTeams adds teams to the PYP competition for a season.
func AddTeams(ctx *Context) error {
	if !ctx.Append && !ctx.Force && !ctx.DryRun {
		return fmt.Errorf("AddTeams: refusing to overwrite pony teams: explicitly override with --force argument")
	}

	season, seasonRef, err := firestore.GetSeason(ctx.Context, ctx.FirestoreClient, ctx.Season)
	if err != nil {
		return fmt.Errorf("AddTeams: failed to get season %d: %w", ctx.Season, err)
	}

	teams, teamRefs, err := firestore.GetTeams(ctx.Context, seasonRef)
	if err != nil {
		return fmt.Errorf("AddTeams: failed to get teams: %w", err)
	}
	lookup := firestore.NewTeamRefsByOtherName(teams, teamRefs)

	teamsToAdd := make(map[string]float64)
	if ctx.Append {
		for id, wins := range season.PonyTeams {
			teamsToAdd[id] = wins
		}
	}
	for name, wins := range ctx.TeamNameWins {
		ref, found := lookup[name]
		if !found {
			return fmt.Errorf("AddTeams: failed to find team with other name '%s'", name)
		}
		teamsToAdd[ref.ID] = wins
	}

	if ctx.DryRun {
		log.Printf("DRY RUN: would set the following pony teams for season %d:", ctx.Season)
		for id, wins := range teamsToAdd {
			log.Printf("%s: %f", id, wins)
		}
		return nil
	}

	err = ctx.FirestoreClient.RunTransaction(ctx.Context, func(c context.Context, t *fs.Transaction) error {
		return t.Update(seasonRef, []fs.Update{{Path: "pony_teams", Value: &teamsToAdd}})
	})

	if err != nil {
		return fmt.Errorf("AddTeams: failed to execute transaction: %w", err)
	}
	return nil
}
