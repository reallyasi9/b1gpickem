package editteams

import (
	"context"
	"fmt"
	"log"
	"strconv"

	fs "cloud.google.com/go/firestore"
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
