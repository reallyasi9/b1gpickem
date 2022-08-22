package editteams

import (
	"fmt"

	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

func LsTeams(ctx *Context) error {
	_, seasonRef, err := firestore.GetSeason(ctx, ctx.FirestoreClient, ctx.Season)
	if err != nil {
		return fmt.Errorf("LsTeams: failed to get season: %w", err)
	}
	teams, teamRefs, err := firestore.GetTeams(ctx, seasonRef)
	if err != nil {
		return fmt.Errorf("LsTeams: failed to get teams: %w", err)
	}

	for i, team := range teams {
		fmt.Printf("%s -> %s\n", teamRefs[i].ID, team)
	}
	return nil
}
