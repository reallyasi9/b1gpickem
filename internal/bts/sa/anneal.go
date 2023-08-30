package sa

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"sort"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/reallyasi9/b1gpickem/internal/bts"

	bpefs "github.com/reallyasi9/b1gpickem/internal/firestore"
)

// ByProbDesc sorts StreakPredictions by probability times spread (descending)
type ByProbDesc []bpefs.StreakPrediction

func (a ByProbDesc) Len() int { return len(a) }
func (a ByProbDesc) Less(i, j int) bool {
	psi := a[i].CumulativeProbability * a[i].CumulativeSpread
	psj := a[j].CumulativeProbability * a[j].CumulativeSpread
	if psi == psj {
		return a[i].CumulativeProbability > a[j].CumulativeProbability
	}
	return psi > psj
}
func (a ByProbDesc) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

func Anneal(ctx *Context) error {
	log.Print("Beating the streak")

	pickers := ctx.Streakers
	log.Printf("Beating the streak with streakers %s", pickers)
	weekNumber := ctx.Week
	pickerNames := pickers

	if len(pickerNames) == 0 && !ctx.All {
		return fmt.Errorf("Anneal: must supply at least one streaker if --all not set")
	}

	fs := ctx.FirestoreClient

	// Get season
	season, seasonRef, err := bpefs.GetSeason(ctx, fs, ctx.Season)
	if err != nil {
		return fmt.Errorf("Anneal: unable to get season: %v", err)
	}
	log.Printf("season discovered: %s", seasonRef.ID)

	// Get week
	_, weekRef, err := bpefs.GetWeek(ctx, seasonRef, weekNumber)
	if err != nil {
		return fmt.Errorf("Anneal: unable to get week: %v", err)
	}
	log.Printf("week discovered: %s", weekRef.ID)

	// Get most recent Sagarin Ratings proper
	// I can cheat because I know this already.
	sagPointsRef := weekRef.Collection("team-points").Doc("sagarin")
	sagSnaps, err := sagPointsRef.Collection("linesag").Documents(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("Anneal: unable to get sagarin ratings: %v", err)
	}
	sagarinRatings := make(map[string]bpefs.ModelTeamPoints)
	for _, s := range sagSnaps {
		var sag bpefs.ModelTeamPoints
		err = s.DataTo(&sag)
		if err != nil {
			return fmt.Errorf("Anneal: unable to get sagarin rating: %v", err)
		}
		// Sagarin has one nil team representing a non-recorded team. Don't keep that one.
		if sag.Team == nil {
			continue
		}
		sagarinRatings[sag.Team.ID] = sag
	}
	log.Printf("latest sagarin ratings discovered for %d teams", len(sagarinRatings))

	// Get the streakers for this week
	pickerDocs, pickerRefs, err := bpefs.GetPickers(ctx, ctx.FirestoreClient)
	if err != nil {
		return fmt.Errorf("Anneal: unable to load pickers for fast lookup")
	}
	pickerLookup := make(map[string]string)
	for i, d := range pickerDocs {
		pickerLookup[pickerRefs[i].ID] = d.LukeName
	}

	pickerIDMap, _, err := bpefs.GetRemainingStreaks(ctx, seasonRef, weekRef)
	if err != nil {
		return fmt.Errorf("Anneal: unable to get remaining streaks: %v", err)
	}

	// replace IDs with names
	pickerMap := make(map[string]bpefs.StreakTeamsRemaining)
	for id, str := range pickerIDMap {
		pickerMap[pickerLookup[id]] = str
	}
	log.Printf("pickers loaded: %+v", pickerMap)

	if !ctx.All {
		foundPickers := make(map[string]struct{})
		for _, name := range pickerNames {
			if _, ok := pickerMap[name]; !ok {
				return fmt.Errorf("Anneal: picker '%s' does not have an active streak", name)
			}
			foundPickers[name] = struct{}{}
		}
		for name := range pickerMap {
			if _, ok := foundPickers[name]; !ok {
				delete(pickerMap, name)
			}
		}
		log.Printf("pickers selected: %+v", pickerMap)
	} else {
		log.Printf("all pickers selected with --all flag")
	}

	// Get team names for pretty printing
	teamDocs, err := ctx.FirestoreClient.GetAll(ctx, season.StreakTeams)
	if err != nil {
		return fmt.Errorf("Anneal: unable to get teams for pretty printing: %w", err)
	}
	teamNamesByID := make(map[string]string)
	for _, td := range teamDocs {
		id := td.Ref.ID
		var t bpefs.Team
		err := td.DataTo(&t)
		if err != nil {
			return fmt.Errorf("Anneal: unable to translate team from %+v to Team object: %w", td, err)
		}
		teamNamesByID[id] = t.School
	}

	// Get most recent performances for sagarin
	performances, performanceRefs, err := bpefs.GetMostRecentModelPerformances(ctx, fs, weekRef)
	if err != nil {
		return fmt.Errorf("Anneal: unable to get model performances: %v", err)
	}
	var sagPerf bpefs.ModelPerformance
	var sagPerfRef *firestore.DocumentRef
	sagPerfFound := false
	for i, perf := range performances {
		if perf.Model.ID == "linesag" {
			sagPerf = perf
			sagPerfRef = performanceRefs[i]
			sagPerfFound = true
			break
		}
	}
	if !sagPerfFound {
		return fmt.Errorf("Anneal: unable to retrieve most recent Sagarin performance for the week")
	}
	log.Printf("Sagarin Ratings performance: %v", sagPerf)

	// Build the probability model
	model := bts.NewGaussianSpreadModel(sagarinRatings, sagPerf)
	log.Printf("Built model %v", model)

	// Get schedule from most recent season
	schedule, err := bts.MakeSchedule(ctx, seasonRef, weekNumber, season.StreakTeams)
	if err != nil {
		return fmt.Errorf("Anneal: unable to make schedule: %v", err)
	}
	log.Printf("Schedule built:\n%v", schedule)

	predictions := bts.MakePredictions(&schedule, *model)
	log.Printf("Made predictions\n%s", predictions)

	// for fast lookups later
	players := make(bts.PlayerMap)

	for id, str := range pickerMap {
		// convert for compatability
		remainingTeams := make(bts.Remaining, len(str.TeamsRemaining))
		for i, t := range str.TeamsRemaining {
			remainingTeams[i] = bts.Team(t.ID)
		}
		players[id], err = bts.NewPlayer(id, str.Picker, remainingTeams, str.TeamsRemaining, str.PickTypesRemaining)
		if err != nil {
			return fmt.Errorf("Anneal: unable to make player: %v", err)
		}
	}

	log.Printf("Pickers readied:\n%v", players)

	// Here we go.
	// Find the unique users.
	// Legacy code!
	duplicates := players.Duplicates()
	log.Println("The following pickers are unique clones of one another:")
	for user, clones := range duplicates {
		if len(clones) == 0 {
			log.Printf("%s is unique", user)
		} else {
			log.Printf("%s clones %v", user, clones)
		}
		for _, clone := range clones {
			delete(players, clone.Name())
		}
	}

	log.Println("Starting MC")

	// Loop through the unique users
	playerItr := playerIterator(players)

	// Loop through streaks
	ppts := perPlayerTeamStreaks(ctx, playerItr, predictions)

	// Update best
	bestStreaks := calculateBestStreaks(ppts)

	// Collect by player
	streakOptions := collectByPlayer(bestStreaks, players, predictions, &schedule, seasonRef, weekRef, duplicates)

	// Print results
	prettyPrint(streakOptions, teamNamesByID)

	output := weekRef.Collection(bpefs.STEAK_PREDICTIONS_COLLECTION)

	if ctx.DryRun {
		log.Print("DRY RUN: Would write the following:")
	}
	for _, streak := range streakOptions {
		streak.Model = sagPointsRef
		streak.PredictionTracker = sagPerfRef

		if ctx.DryRun {
			log.Printf("%s: add %+v", output.Path, streak)
			continue
		}

		_, _, err := output.Add(ctx, streak)
		if err != nil {
			return fmt.Errorf("Anneal: unable to write streak to Firestore: %v", err)
		}
	}

	return nil
}

func prettyPrint(streaks map[string]bpefs.StreakPredictions, teamNamesByID map[string]string) {
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(table.Row{"Picker", "Week", "Team", "Win Prob.", "Pred. Spread", "Cum. Prob.", "Cum. Spread"})
	t.SetColumnConfigs([]table.ColumnConfig{
		{Number: 1, AutoMerge: true},
		{Number: 2, AutoMerge: true},
	})
	for picker, streak := range streaks {
		// picker := pickerNamesByID[streak.Picker.ID]
		bestPick := streak.PossiblePicks[0]
		currentProb := float64(1)
		currentSpread := float64(0)
		for iwk, streak := range bestPick.Weeks {
			for ipk, pick := range streak.Pick {
				team := teamNamesByID[pick.ID]
				prob := streak.Probabilities[ipk]
				spread := streak.Spreads[ipk]
				currentProb *= prob
				currentSpread += spread
				t.AppendRow(table.Row{picker, iwk, team, fmt.Sprintf("%0.4f", prob), fmt.Sprintf("%0.2f", spread), fmt.Sprintf("%0.4f", currentProb), fmt.Sprintf("%0.2f", currentSpread)})
			}
		}
		t.AppendSeparator()
	}

	t.SetStyle(table.StyleLight)
	t.Render()
}

// StreakMap is a simple map of player names to streaks
type streakMap map[playerTeam]streakProb

type streakProb struct {
	streak *bts.Streak
	prob   float64
	spread float64
}

type playerTeam struct {
	player string
	team   bts.Team
}

func (sm *streakMap) update(player string, team bts.Team, spin streakProb) {
	pt := playerTeam{player: player, team: team}
	bestp := math.Inf(-1)
	bests := math.Inf(-1)
	if sp, ok := (*sm)[pt]; ok {
		bestp = sp.prob
		bests = sp.spread
	}
	if spin.prob > bestp || (spin.prob == bestp && spin.spread > bests) {
		(*sm)[pt] = streakProb{streak: spin.streak, prob: spin.prob, spread: spin.spread}
	}
}

type playerTeamStreakProb struct {
	player     *bts.Player
	team       bts.Team
	streakProb streakProb
}

func playerIterator(pm bts.PlayerMap) <-chan *bts.Player {
	out := make(chan *bts.Player)

	go func() {
		defer close(out)

		for _, player := range pm {
			out <- player
		}
	}()

	return out
}

func perPlayerTeamStreaks(ctx *Context, ps <-chan *bts.Player, predictions *bts.Predictions) <-chan playerTeamStreakProb {

	out := make(chan playerTeamStreakProb, 100)

	go func(out chan<- playerTeamStreakProb) {
		var wg sync.WaitGroup
		sd := ctx.Seed
		if sd < 0 {
			sd = time.Now().UnixNano()
		}
		src := rand.NewSource(sd)
		for p := range ps {
			for i := 0; i < ctx.Workers; i++ {
				wg.Add(1)
				mySeed := src.Int63()
				go func(worker int, p *bts.Player, out chan<- playerTeamStreakProb) {
					anneal(ctx, mySeed, worker, p, predictions, out)
					wg.Done()
				}(i, p, out)
			}
		}
		wg.Wait()
		close(out)
	}(out)

	return out
}

func anneal(ctx *Context, seed int64, worker int, p *bts.Player, predictions *bts.Predictions, out chan<- playerTeamStreakProb) {

	src := rand.NewSource(seed)
	rng := rand.New(src)

	maxIterations := ctx.Iterations
	tConst := ctx.C
	tExp := ctx.E
	maxDrift := ctx.WanderLimit
	countSinceReset := maxDrift

	s := bts.NewStreak(p.RemainingTeams(), p.WeekTypeIterator().Permutation())
	bestS := s.Clone()
	resetS := s.Clone()
	bestExp := 0.
	resetExp := 0.

	log.Printf("Player %s w %d start: streak=%s", p.Name(), worker, bestS)
	for i := 0; i < maxIterations; i++ {
		temperature := tConst * float64(maxIterations-i) / float64(maxIterations)
		temperature = math.Pow(temperature, tExp)

		s.Perturbate(src, true)
		newP, newSpread := bts.SummarizeStreak(predictions, s)

		// ignore impossible outcomes
		if newP == 0 {
			continue
		}

		expectedPoints := newP * newSpread
		denom := math.Max(math.Abs(bestExp+expectedPoints), 1.)
		fracChange := (bestExp - expectedPoints) / denom

		if expectedPoints > bestExp || fracChange > rng.Float64() {

			// if newP <= bestP {
			// 	log.Printf("Player %s accepted worse outcome due to temperature", p.Name())
			// }

			bestExp = expectedPoints
			bestS = s.Clone()

			if bestExp > resetExp {
				resetExp = bestExp
				resetS = bestS.Clone()
				countSinceReset = maxDrift

				for _, team := range resetS.GetWeek(0) {
					sp := streakProb{streak: resetS.Clone(), prob: newP, spread: newSpread}
					out <- playerTeamStreakProb{player: p, team: team, streakProb: sp}
				}

				log.Printf("Player %s w %d itr %d (temp %f): exp=%f, p=%f, s=%f, streak=%s", p.Name(), worker, i, temperature, bestExp, newP, newSpread, bestS)
			}

		} else if countSinceReset < 0 {
			countSinceReset = maxDrift
			bestExp = resetExp
			s = resetS.Clone()

			// log.Printf("Player %s reset at itr %d to p=%f, s=%f, streak=%s", p.Name(), i, bestP, bestSpread, bestS)
		}

		countSinceReset--
	}
}

func calculateBestStreaks(ppts <-chan playerTeamStreakProb) <-chan streakMap {
	out := make(chan streakMap, 100)

	sm := make(streakMap)
	go func() {
		defer close(out)

		for ptsp := range ppts {
			sm.update(ptsp.player.Name(), ptsp.team, ptsp.streakProb)
		}

		out <- sm
	}()

	return out
}

func collectByPlayer(sms <-chan streakMap, players bts.PlayerMap, predictions *bts.Predictions, schedule *bts.Schedule, seasonRef, weekRef *firestore.DocumentRef, duplicates map[string][]*bts.Player) map[string]bpefs.StreakPredictions {

	startTime := time.Now()

	// Collect streak options by player
	soByPlayer := make(map[string][]bpefs.StreakPrediction)
	for sm := range sms {

		for pt, sp := range sm {

			prob := sp.prob
			spread := sp.spread

			weeks := make([]bpefs.StreakWeek, sp.streak.NumWeeks())
			for iweek := 0; iweek < sp.streak.NumWeeks(); iweek++ {

				pickedTeams := make([]*firestore.DocumentRef, 0)
				pickedProbs := make([]float64, 0)
				pickedSpreads := make([]float64, 0)
				for _, team := range sp.streak.GetWeek(iweek) {
					probability := predictions.GetProbability(team, iweek)
					pickedProbs = append(pickedProbs, probability)

					spread := predictions.GetSpread(team, iweek)
					pickedSpreads = append(pickedSpreads, spread)

					if team == bts.BYE || team == bts.NONE {
						continue
					}
					// Cheat because I have the ID now
					teamRef := seasonRef.Collection(bpefs.TEAMS_COLLECTION).Doc(string(team))
					pickedTeams = append(pickedTeams, teamRef)
				}

				weeks[iweek] = bpefs.StreakWeek{Pick: pickedTeams, Probabilities: pickedProbs, Spreads: pickedSpreads}

			}

			so := bpefs.StreakPrediction{CumulativeProbability: prob, CumulativeSpread: spread, Weeks: weeks}
			soByPlayer[pt.player] = append(soByPlayer[pt.player], so)

			// duplicate results
			for _, dupPlayer := range duplicates[pt.player] {
				soByPlayer[dupPlayer.Name()] = append(soByPlayer[dupPlayer.Name()], so)
				// now that the simulation is done, add the duplicates back to the player map
				players[dupPlayer.Name()] = dupPlayer
			}
		}

	}

	// Run through players and calculate best option
	prs := make(map[string]bpefs.StreakPredictions)
	for picker, streakOptions := range soByPlayer {
		// TODO: look up player (key of soByPlayer)

		if len(streakOptions) == 0 {
			continue
		}

		sort.Sort(ByProbDesc(streakOptions))

		bestSelection := streakOptions[0].Weeks[0].Pick
		bestProb := streakOptions[0].CumulativeProbability
		bestSpread := streakOptions[0].CumulativeSpread

		prs[picker] = bpefs.StreakPredictions{
			Picker: players[picker].Ref(),

			TeamsRemaining:     players[picker].RemainingTeamsRefs(),
			PickTypesRemaining: players[picker].RemainingWeekTypes(),

			BestPick:             bestSelection,
			Probability:          bestProb,
			Spread:               bestSpread,
			PossiblePicks:        streakOptions,
			CalculationStartTime: startTime,
			CalculationEndTime:   time.Now(),
		}
	}

	return prs
}
