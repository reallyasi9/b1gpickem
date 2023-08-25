package posteriors

import (
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strings"
	"sync"

	"cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/bts"
	"github.com/reallyasi9/b1gpickem/internal/tools/editteams"
	"gonum.org/v1/gonum/stat"

	bpefs "github.com/reallyasi9/b1gpickem/internal/firestore"
)

func Posteriors(ctx *Context) error {
	log.Print("Computing Posterior Wins")

	fs := ctx.FirestoreClient

	// Get season
	_, seasonRef, err := bpefs.GetSeason(ctx, fs, ctx.Season)
	if err != nil {
		return fmt.Errorf("Posteriors: unable to get season: %w", err)
	}
	log.Printf("season discovered: %s", seasonRef.ID)

	// Get week
	weekNumber := ctx.Week
	_, weekRef, err := bpefs.GetWeek(ctx, seasonRef, weekNumber)
	if err != nil {
		return fmt.Errorf("Posteriors: unable to get week: %v", err)
	}
	log.Printf("week discovered: %s", weekRef.ID)

	// Get most recent Sagarin Ratings proper
	// I can cheat because I know this already.
	sagPointsRef := weekRef.Collection("team-points").Doc("sagarin")
	sagSnaps, err := sagPointsRef.Collection("linesag").Documents(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("Posteriors: unable to get sagarin ratings: %v", err)
	}
	sagarinRatings := make(map[string]bpefs.ModelTeamPoints)
	for _, s := range sagSnaps {
		var sag bpefs.ModelTeamPoints
		err = s.DataTo(&sag)
		if err != nil {
			return fmt.Errorf("Posteriors: unable to get sagarin rating: %v", err)
		}
		// Sagarin has one nil team representing a non-recorded team. Don't keep that one.
		if sag.Team == nil {
			continue
		}
		sagarinRatings[sag.Team.ID] = sag
	}
	log.Printf("latest sagarin ratings discovered: %v", sagarinRatings)

	// Get most recent performances for sagarin
	performances, performanceRefs, err := bpefs.GetMostRecentModelPerformances(ctx, fs, weekRef)
	if err != nil {
		return fmt.Errorf("Posteriors: unable to get model performances: %v", err)
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
		return fmt.Errorf("Posteriors: unable to find most recent Sagarin performance for the week")
	}
	log.Printf("Sagarin Ratings performance: %v", sagPerf)

	// Build the probability model
	model := bts.NewGaussianSpreadModel(sagarinRatings, sagPerf)
	log.Printf("Built model %v", model)

	// Get teams
	teams, teamRefs, err := bpefs.GetTeams(ctx, seasonRef)
	if err != nil {
		return fmt.Errorf("Posteriors: unable to retrieve team references: %w", err)
	}

	// Filter teams
	// Get teams by short name first
	var teamsByShortName bpefs.TeamRefsByName
	var err2 *bpefs.DuplicateTeamNameError
	for {
		teamsByShortName, err2 = bpefs.NewTeamRefsByShortName(teams, teamRefs)
		if err2 == nil {
			break
		}

		updateNames, err := editteams.SurveyReplaceName(teams, teamRefs, err2.Name, err2.Teams, err2.Refs, bpefs.ShortName)
		if err != nil {
			panic(err)
		}

		for ref, t := range updateNames {
			fmt.Printf("Updating %s to change %s (names now [%s])\n", ref.ID, err2.Name, strings.Join(t.ShortNames, ", "))

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

	// Filter here
	posteriorTeams := []*firestore.DocumentRef{}
	teamNamesByID := make(map[string]string)
TeamNameLoop:
	for _, teamName := range ctx.Teams {
	TeamLookupLoop:
		for {
			if ref, ok := teamsByShortName[teamName]; ok {
				posteriorTeams = append(posteriorTeams, ref)
				teamNamesByID[ref.ID] = teamName
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

	// Get schedule from most recent season
	log.Printf("Building schedule for %d teams", len(posteriorTeams))
	schedule, err := bts.MakeSchedule(ctx, seasonRef, weekNumber, posteriorTeams)
	if err != nil {
		return fmt.Errorf("Posteriors: unable to make schedule: %v", err)
	}
	log.Printf("Schedule built:\n%v", schedule)

	log.Printf("Here we go!")

	out := simulate(&schedule, model, ctx.Iterations, ctx.Championship)
	winMap := make(map[bts.Team][]float64)
	for result := range out {
		for t, wins := range result {
			winMap[t] = append(winMap[t], float64(wins))
		}
	}
	for t, wins := range winMap {
		sort.Float64s(wins)
		mean := stat.Mean(wins, nil)
		min := wins[0]
		q25 := stat.Quantile(0.25, stat.LinInterp, wins, nil)
		median := stat.Quantile(0.5, stat.LinInterp, wins, nil)
		q75 := stat.Quantile(0.75, stat.LinInterp, wins, nil)
		max := wins[len(wins)-1]
		fmt.Printf("%s: %0.3f [%0.0f ... %0.0f ... %0.0f ... %0.0f ... %0.0f]\n", teamNamesByID[string(t)], mean, min, q25, median, q75, max)
	}

	log.Printf("Done")

	return nil
}

func simulate(schedule *bts.Schedule, model bts.PredictionModel, seasons int, playChampionship bool) <-chan map[bts.Team]int {
	out := make(chan map[bts.Team]int, 100)

	var wg sync.WaitGroup
	go func() {
		defer close(out)

		for itr := 0; itr < seasons; itr++ {
			wg.Add(1)
			go func() {
				simulateSeason(schedule, model, playChampionship, out)
				wg.Done()
			}()
		}

		wg.Wait()
	}()

	return out
}

func simulateSeason(schedule *bts.Schedule, model bts.PredictionModel, playChampionship bool, out chan map[bts.Team]int) {
	nwk := schedule.NumWeeks()
	tl := schedule.TeamList()
	gameSeen := make(map[*bts.Game]struct{})
	wins := make(map[bts.Team]int)
	firstLevelTeamLookup := make(map[bts.Team]struct{})
	for _, t := range tl {
		firstLevelTeamLookup[t] = struct{}{}
	}

	for _, team := range tl {
		for wk := 0; wk < nwk; wk++ {

			game := schedule.Get(team, wk)
			if _, found := gameSeen[game]; found {
				continue
			}

			prob, _ := model.Predict(game)
			var winner bts.Team
			p := rand.Float64()
			if p < prob {
				winner = game.Team(0)
			} else {
				winner = game.Team(1)
			}
			if _, ok := firstLevelTeamLookup[winner]; ok {
				wins[winner] += 1
			}

			gameSeen[game] = struct{}{}
		}
	}

	if playChampionship {
		var topTeam bts.Team
		var nextTeam bts.Team
		topWins := 0
		for t, w := range wins {
			if w > topWins {
				topWins = w
				topTeam, nextTeam = t, topTeam
			}
		}
		championshipGame := bts.NewGame(topTeam, nextTeam, bts.Neutral)
		prob, _ := model.Predict(championshipGame)
		var winner bts.Team
		if rand.Float64() < prob {
			winner = topTeam
		} else {
			winner = nextTeam
		}
		wins[winner] += 1
	}

	out <- wins
}
