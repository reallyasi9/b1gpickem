package btsteams

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	fs "cloud.google.com/go/firestore"
	"github.com/AlecAivazis/survey/v2"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
	"github.com/reallyasi9/b1gpickem/internal/tools/editteams"
)

func AddTeams(ctx *Context) error {
	season, seasonRef, err := firestore.GetSeason(ctx.Context, ctx.FirestoreClient, ctx.Season)
	if err != nil {
		return fmt.Errorf("AddTeams: failed to get season %d: %w", ctx.Season, err)
	}
	teams, teamRefs, err := firestore.GetTeams(ctx.Context, seasonRef)
	if err != nil {
		return fmt.Errorf("AddTeams: failed to get teams: %w", err)
	}
	var lookup firestore.TeamRefsByName
	var err2 *firestore.DuplicateTeamNameError
	for {
		lookup, err2 = firestore.NewTeamRefsByOtherName(teams, teamRefs)
		if err2 == nil {
			break
		}

		fmt.Printf("An error occurred when creating a team lookup map.\nThe name \"%s\" is used by %d teams.\nYou must update the names used by the teams to correct this before continuing.", err2.Name, len(err2.Teams))
		teamsByName := make(map[string]firestore.Team)
		teamRefsByName := make(map[string]*fs.DocumentRef)
		teamNames := []string{}
		for i, team := range err2.Teams {
			dispName := fmt.Sprintf("(%s) %s", err2.Refs[i].ID, team.DisplayName())
			teamsByName[dispName] = team
			teamRefsByName[dispName] = err2.Refs[i]
			teamNames = append(teamNames, dispName)
		}
		q1 := &survey.MultiSelect{
			Message: "Which team(s) do you want to edit?",
			Options: teamNames,
		}
		a1 := []string{}
		err := survey.AskOne(q1, &a1, survey.WithRemoveSelectNone(), survey.WithValidator(survey.MinItems(1)))
		if err != nil {
			panic(err)
		}

		updateNames := make(map[*fs.DocumentRef]firestore.Team)
		for _, updateTeam := range a1 {
			q2 := &survey.Input{
				Message: fmt.Sprintf("Enter the name for team \"%s\" that will replace \"%s\" (leave blank to delete the name from the team)", updateTeam, err2.Name),
			}
			var a2 string
			err := survey.AskOne(q2, &a2, survey.WithValidator(func(val interface{}) error {
				if str, ok := val.(string); !ok || str == err2.Name {
					return errors.New("the new name cannot be the same as the old name")
				}
				return nil
			}))
			if err != nil {
				panic(err)
			}
			t := teamsByName[updateTeam]
			for i, n := range t.OtherNames {
				if n == err2.Name {
					if len(a2) == 0 {
						t.OtherNames = append(t.OtherNames[:i], t.OtherNames[i+1:]...)
					} else {
						t.OtherNames[i] = a2
					}
				}
			}
			updateNames[teamRefsByName[updateTeam]] = t
		}

		for ref, t := range updateNames {
			fmt.Printf("Updating %s to eliminate %s (names now [%s])\n", ref.ID, err2.Name, strings.Join(t.OtherNames, ", "))

			editContext := &editteams.Context{
				Context:         ctx.Context,
				Force:           ctx.Force,
				DryRun:          ctx.DryRun,
				FirestoreClient: ctx.FirestoreClient,
				ID:              ref.ID,
				Team:            t,
				Season:          ctx.Season,
				Append:          false,
			}
			err := editteams.EditTeam(editContext)
			if err != nil {
				panic(err)
			}
		}
	}

	teamsToAdd := make(map[string]*fs.DocumentRef)
	if ctx.Append {
		for _, ref := range season.StreakTeams {
			teamsToAdd[ref.ID] = ref
		}
	}
	for _, name := range ctx.TeamNames {
		ref, found := lookup[name]
		if !found {
			return fmt.Errorf("AddTeams: failed to find team with other name '%s'", name)
		}
		teamsToAdd[ref.ID] = ref
	}

	if ctx.DryRun {
		log.Printf("DRY RUN: would set the following streak teams for season %d:", ctx.Season)
		for id := range teamsToAdd {
			log.Print(id)
		}
		return nil
	}

	if !ctx.Append && !ctx.Force {
		return fmt.Errorf("AddTeams: refusing to overwrite streak teams: explicitly override with --force argument")
	}
	// one last error check: count teams and weeks and compare
	nPicks := 0
	for typ, n := range season.StreakPickTypes {
		nPicks += typ * n
	}
	if nPicks != len(teamsToAdd) && !ctx.Force {
		return fmt.Errorf("AddTeams: number of teams (%d) not equal to number of streak picks calculated from week types (%d): explicitly override with --force argument", len(teamsToAdd), nPicks)
	}

	newTeams := make([]*fs.DocumentRef, 0, len(teamsToAdd))
	for _, ref := range teamsToAdd {
		newTeams = append(newTeams, ref)
	}
	err = ctx.FirestoreClient.RunTransaction(ctx.Context, func(c context.Context, t *fs.Transaction) error {
		return t.Update(seasonRef, []fs.Update{{Path: "streak_teams", Value: &newTeams}})
	})

	if err != nil {
		return fmt.Errorf("AddTeams: failed to execute transaction: %w", err)
	}
	return nil
}
