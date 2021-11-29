package btsteams

import (
	"fmt"
	"sort"

	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

type schoolID struct {
	School string
	ID     string
}

type bySchool []schoolID

func (a bySchool) Len() int           { return len(a) }
func (a bySchool) Less(i, j int) bool { return a[i].School < a[j].School }
func (a bySchool) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

func LsTeams(ctx *Context) error {
	season, seasonRef, err := firestore.GetSeason(ctx.Context, ctx.FirestoreClient, ctx.Season)
	if err != nil {
		return fmt.Errorf("LsTeams: failed to get season %d: %w", ctx.Season, err)
	}
	teams, teamRefs, err := firestore.GetTeams(ctx.Context, seasonRef)
	if err != nil {
		return fmt.Errorf("LsTeams: failed to get teams: %w", err)
	}
	lookup := make(map[string]string)
	for i, ref := range teamRefs {
		lookup[ref.ID] = teams[i].School
	}
	ls := make(bySchool, len(season.StreakTeams))
	for i, ref := range season.StreakTeams {
		school, found := lookup[ref.ID]
		if !found {
			return fmt.Errorf("LsTeams: season references unknown team with ID '%s'", ref.ID)
		}
		ls[i] = schoolID{School: school, ID: ref.ID}
	}
	sort.Sort(ls)

	for _, sid := range ls {
		fmt.Printf("%s (%s)\n", sid.School, sid.ID)
	}
	return nil
}
