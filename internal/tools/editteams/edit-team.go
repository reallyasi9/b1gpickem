package editteams

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"

	fs "cloud.google.com/go/firestore"
	"github.com/AlecAivazis/survey/v2"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

func distinct(s []string) []string {
	out := make([]string, len(s))
	n := 0
	seen := make(map[string]struct{})
	for _, x := range s {
		if _, ok := seen[x]; ok {
			continue
		}
		out[n] = x
		n++
	}
	return out[:n]
}

func EditTeam(ctx *Context) error {

	newTeam := ctx.Team
	if newTeam.Abbreviation == "" && newTeam.Mascot == "" && newTeam.School == "" && len(newTeam.Logos) == 0 && len(newTeam.Colors) == 0 && len(newTeam.OtherNames) == 0 && len(newTeam.ShortNames) == 0 {
		return fmt.Errorf("EditTeam: at least one field to edit must be specified")
	}

	seasonStr := strconv.Itoa(ctx.Season)
	snap, err := ctx.FirestoreClient.Collection(firestore.SEASONS_COLLECTION).Doc(seasonStr).Collection(firestore.TEAMS_COLLECTION).Doc(ctx.ID).Get(ctx)
	if err != nil {
		return fmt.Errorf("EditTeam: error getting team with ID '%s' in season %d: %w", ctx.ID, ctx.Season, err)
	}

	var team firestore.Team
	err = snap.DataTo(&team)
	if err != nil {
		return fmt.Errorf("EditTeam: error converting team: %w", err)
	}

	if ctx.DryRun {
		log.Print("DRY RUN: would make the following edits:")
		log.Printf("%s: %s", snap.Ref.Path, team)
		if newTeam.Abbreviation != "" {
			log.Printf("Abbreviation to '%s'", newTeam.Abbreviation)
		}
		if newTeam.Mascot != "" {
			log.Printf("Mascot to '%s'", newTeam.Mascot)
		}
		if newTeam.School != "" {
			log.Printf("School to '%s'", newTeam.School)
		}
		if ctx.Append {
			log.Print("(All list edits will be appends)")
		}
		if len(newTeam.Logos) != 0 {
			log.Printf("Logos to '%v'", newTeam.Logos)
		}
		if len(newTeam.Colors) != 0 {
			log.Printf("Colors to '%v'", newTeam.Colors)
		}
		if len(newTeam.OtherNames) != 0 {
			log.Printf("OtherNames to '%v'", newTeam.OtherNames)
		}
		if len(newTeam.ShortNames) != 0 {
			log.Printf("ShortNames to '%v'", newTeam.ShortNames)
		}
		return nil
	}

	if !ctx.Force {
		return fmt.Errorf("EditTeam: edit of teams is dangerous: use force flag to force edit")
	}

	err = ctx.FirestoreClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
		updates := make([]fs.Update, 0, 7)
		if newTeam.Abbreviation != "" {
			updates = append(updates, fs.Update{Path: "abbreviation", Value: newTeam.Abbreviation})
		}
		if newTeam.Mascot != "" {
			updates = append(updates, fs.Update{Path: "mascot", Value: newTeam.Mascot})
		}
		if newTeam.School != "" {
			updates = append(updates, fs.Update{Path: "school", Value: newTeam.School})
		}
		if len(newTeam.Logos) != 0 {
			logos := newTeam.Logos
			if ctx.Append {
				logos = append(logos, team.Logos...)
			}
			logos = distinct(logos)
			updates = append(updates, fs.Update{Path: "logos", Value: logos})
		}
		if len(newTeam.Colors) != 0 {
			colors := newTeam.Colors
			if ctx.Append {
				colors = append(colors, team.Colors...)
			}
			colors = distinct(colors)
			updates = append(updates, fs.Update{Path: "colors", Value: colors})
		}
		if len(newTeam.OtherNames) != 0 {
			otherNames := newTeam.OtherNames
			if ctx.Append {
				otherNames = append(otherNames, team.OtherNames...)
			}
			otherNames = distinct(otherNames)
			updates = append(updates, fs.Update{Path: "other_names", Value: otherNames})
		}
		if len(newTeam.ShortNames) != 0 {
			shortNames := newTeam.ShortNames
			if ctx.Append {
				shortNames = append(shortNames, team.ShortNames...)
			}
			shortNames = distinct(shortNames)
			updates = append(updates, fs.Update{Path: "short_names", Value: shortNames})
		}
		return t.Update(snap.Ref, updates)
	})

	if err != nil {
		return fmt.Errorf("EditTeam: error running transaction: %w", err)
	}
	return err
}

type NameType int64

const (
	ShortName NameType = 0
	OtherName NameType = 1
)

func SurveyTeamNames(teams []firestore.Team, teamRefs []*fs.DocumentRef, teamlist firestore.TeamRefsByName, errName string, errTeams []firestore.Team, errRefs []*fs.DocumentRef, nameType NameType) (map[*fs.DocumentRef]firestore.Team, error) {
	fmt.Printf("An error occurred when creating a team lookup map.\nThe name \"%s\" is used by %d teams.\nYou must update the names used by the teams to correct this before continuing.", errName, len(errTeams))
	teamsByName := make(map[string]firestore.Team)
	teamRefsByName := make(map[string]*fs.DocumentRef)
	teamNames := []string{}
	for i, team := range errTeams {
		dispName := fmt.Sprintf("(%s) %s", errRefs[i].ID, team.DisplayName())
		teamsByName[dispName] = team
		teamRefsByName[dispName] = errRefs[i]
		teamNames = append(teamNames, dispName)
	}
	q1 := &survey.MultiSelect{
		Message: "Which team(s) do you want to edit?",
		Options: teamNames,
	}
	a1 := []string{}
	err := survey.AskOne(q1, &a1, survey.WithRemoveSelectNone(), survey.WithValidator(survey.MinItems(1)))
	if err != nil {
		return nil, err
	}

	updateNames := make(map[*fs.DocumentRef]firestore.Team)
	for _, updateTeam := range a1 {
		q2 := &survey.Input{
			Message: fmt.Sprintf("Enter the name for team \"%s\" that will replace \"%s\" (leave blank to delete the name from the team)", updateTeam, errName),
		}
		var a2 string
		err := survey.AskOne(q2, &a2, survey.WithValidator(func(val interface{}) error {
			if str, ok := val.(string); !ok || str == errName {
				return errors.New("the new name cannot be the same as the old name")
			}
			return nil
		}))
		if err != nil {
			panic(err)
		}
		t := teamsByName[updateTeam]
		if nameType == ShortName {
			for i, n := range t.ShortNames {
				if n == errName {
					if len(a2) == 0 {
						t.ShortNames = append(t.ShortNames[:i], t.ShortNames[i+1:]...)
					} else {
						t.ShortNames[i] = a2
					}
				}
			}
		} else {
			for i, n := range t.OtherNames {
				if n == errName {
					if len(a2) == 0 {
						t.OtherNames = append(t.OtherNames[:i], t.OtherNames[i+1:]...)
					} else {
						t.OtherNames[i] = a2
					}
				}
			}
		}
		updateNames[teamRefsByName[updateTeam]] = t
	}

	return updateNames, nil
}
