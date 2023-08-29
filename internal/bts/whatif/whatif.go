package whatif

import (
	"fmt"
	"log"
	"strings"

	"cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/bts"
	"github.com/reallyasi9/b1gpickem/internal/tools/editteams"

	bpefs "github.com/reallyasi9/b1gpickem/internal/firestore"
)

func WhatIf(ctx *Context) error {
	log.Print("Computing What If? Scenario")

	fs := ctx.FirestoreClient

	// Get season
	_, seasonRef, err := bpefs.GetSeason(ctx, fs, ctx.Season)
	if err != nil {
		return fmt.Errorf("WhatIf: unable to get season: %w", err)
	}

	// Get week
	weekNumber := ctx.Week
	_, weekRef, err := bpefs.GetWeek(ctx, seasonRef, weekNumber)
	if err != nil {
		return fmt.Errorf("WhatIf: unable to get week: %v", err)
	}

	// Get most recent Sagarin Ratings proper
	// I can cheat because I know this already.
	sagPointsRef := weekRef.Collection("team-points").Doc("sagarin")
	sagSnaps, err := sagPointsRef.Collection("linesag").Documents(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("WhatIf: unable to get sagarin ratings: %v", err)
	}
	sagarinRatings := make(map[string]bpefs.ModelTeamPoints)
	for _, s := range sagSnaps {
		var sag bpefs.ModelTeamPoints
		err = s.DataTo(&sag)
		if err != nil {
			return fmt.Errorf("WhatIf: unable to get sagarin rating: %v", err)
		}
		// Sagarin has one nil team representing a non-recorded team. Don't keep that one.
		if sag.Team == nil {
			continue
		}
		sagarinRatings[sag.Team.ID] = sag
	}

	// Get most recent performances for sagarin
	performances, performanceRefs, err := bpefs.GetMostRecentModelPerformances(ctx, fs, weekRef)
	if err != nil {
		return fmt.Errorf("WhatIf: unable to get model performances: %v", err)
	}
	var sagPerf bpefs.ModelPerformance
	var sagPerfRef *firestore.DocumentRef
	for i, perf := range performances {
		if perf.Model.ID == "linesag" {
			sagPerf = perf
			sagPerfRef = performanceRefs[i]
			break
		}
	}
	if sagPerfRef == nil {
		return fmt.Errorf("WhatIf: unable to find most recent Sagarin performance for the week")
	}

	// Build the probability model
	model := bts.NewGaussianSpreadModel(sagarinRatings, sagPerf)

	// Get teams
	teams, teamRefs, err := bpefs.GetTeams(ctx, seasonRef)
	if err != nil {
		return fmt.Errorf("WhatIf: unable to retrieve team references: %w", err)
	}

	// Filter teams
	// Get teams by short name first
	teamsByShortName, errs := bpefs.MakeTeamRefCollection(teams, teamRefs, bpefs.ShortName)
	for _, err := range errs {
		idxs, names, err2 := editteams.SurveyReplaceName(teamsByShortName, err)
		if err2 != nil {
			return fmt.Errorf("Posteriors: failed to replace names: %w", err2)
		}

		for i := range idxs {
			tr := teamsByShortName.ValueAt(i)
			n := names[i]

			// Replace the name in the team
			for j, sn := range tr.Team.ShortNames {
				if sn == err.Name {
					tr.Team.ShortNames[j] = n
				}
			}

			fmt.Printf("Updating %s to change %s (names now [%s])\n", tr.Ref.ID, err.Name, strings.Join(tr.Team.ShortNames, ", "))

			editContext := &editteams.Context{
				Context:         ctx.Context,
				Force:           ctx.Force,
				DryRun:          ctx.DryRun,
				FirestoreClient: ctx.FirestoreClient,
				ID:              tr.Ref.ID,
				Team:            tr.Team,
				Season:          ctx.Season,
				Append:          false,
			}
			err3 := editteams.EditTeam(editContext)
			if err3 != nil {
				panic(err3)
			}

			teamsByShortName.UpdateMap(err.Name, n)
		}
	}

	// Filter here
	whatIfTeams := []*firestore.DocumentRef{}
	contextTeams := []string{ctx.Team1, ctx.Team2}
TeamNameLoop:
	for _, teamName := range contextTeams {
	TeamLookupLoop:
		for {
			if ref, ok := teamsByShortName[teamName]; ok {
				whatIfTeams = append(whatIfTeams, ref)
				continue TeamNameLoop
			} else {
				team, ref, err := editteams.SurveyAddName(teams, teamRefs, teamName, bpefs.ShortName)
				if err != nil {
					panic(err)
				}

				fmt.Printf("Updating %s to add short name %s (names now [%s])\n", ref.ID, teamName, strings.Join(team.ShortNames, ", "))

				editContext := &editteams.Context{
					Context:         ctx.Context,
					Force:           ctx.Force,
					DryRun:          ctx.DryRun,
					FirestoreClient: ctx.FirestoreClient,
					ID:              ref.ID,
					Team:            team,
					Season:          ctx.Season,
					Append:          false,
				}
				err = editteams.EditTeam(editContext)
				if err != nil {
					panic(err)
				}

				teamsByShortName[teamName] = ref
				continue TeamLookupLoop
			}
		}
	}

	// Build game
	var location bts.RelativeLocation
	switch strings.ToLower(ctx.Location) {
	case "home":
		location = bts.Home
	case "away":
		location = bts.Away
	case "near":
		location = bts.Near
	case "far":
		location = bts.Far
	case "neutral":
		fallthrough
	default:
		location = bts.Neutral
	}
	game := bts.NewGame(bts.Team(whatIfTeams[0].ID), bts.Team(whatIfTeams[1].ID), location)

	// Simulate
	p, s := model.Predict(game)
	var winner string
	if p > 0.5 {
		winner = ctx.Team1
	} else {
		winner = ctx.Team2
		p = 1 - p
	}
	fmt.Printf("In a match up between %s (%s) and %s in week %d of season %d,\n%s wins by %0.2f points (%0.3f probability of win)\n", ctx.Team1, ctx.Location, ctx.Team2, ctx.Week, ctx.Season, winner, s, p)

	log.Printf("Done")

	return nil
}
