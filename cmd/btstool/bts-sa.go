package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"sort"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
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

// btsMCFlagSet is a flag.FlagSet for parsing the bts-mc subcommand.
var btsMCFlagSet *flag.FlagSet

var seasonFlag int
var weekFlag int
var maxItr int
var tC float64
var tE float64
var resetItr int
var seed int64
var workers int
var doAll bool

// btsMCUsage is the usage documentation for the bts-mc subcommand.
func btsMCUsage() {
	fmt.Fprintf(flag.CommandLine.Output(), `Usage: btstool [global-flags] bts-sa [flags] [picker [picker ...]]
	
Run simulated anealing simulation of BTS streaks.

Arguments:
  picker string
      Picker to simulate. Multiple pickers can be specified.

Flags:
`)

	btsMCFlagSet.PrintDefaults()

	fmt.Fprint(flag.CommandLine.Output(), "\nGlobal Flags:\n")

	flag.PrintDefaults()

}

func init() {
	btsMCFlagSet = flag.NewFlagSet("bts-sa", flag.ExitOnError)
	btsMCFlagSet.SetOutput(flag.CommandLine.Output())
	btsMCFlagSet.Usage = btsMCUsage

	btsMCFlagSet.IntVar(&seasonFlag, "season", -1, "Season year. Negative values will calculate season based on today's date.")
	btsMCFlagSet.IntVar(&weekFlag, "week", -1, "Week number. Negative values will calculate week number based on today's date.")
	btsMCFlagSet.IntVar(&maxItr, "maxi", 100000000, "Number of simulated annealing iterations per worker.")
	btsMCFlagSet.Float64Var(&tC, "tc", 1., "Simulated annealing temperature constant: p = (tc * (maxi - i) / maxi)^te.")
	btsMCFlagSet.Float64Var(&tE, "te", 3., "Simulated annealing temperature exponent: p = (tc * (maxi - i) / maxi)^te.")
	btsMCFlagSet.IntVar(&resetItr, "reseti", 10000, "Maximum number of iterations to allow simulated annealing solution to wonder before resetting to best solution found so far.")
	btsMCFlagSet.Int64Var(&seed, "seed", -1, "Seed for RNG governing simulated annealing process. Negative values will use system clock to seed RNG.")
	btsMCFlagSet.IntVar(&workers, "workers", 1, "Number of workers per simulated picker. Increases odds of finding the global maximum.")
	btsMCFlagSet.BoolVar(&doAll, "all", false, "Ignore picker list and simulate all registered pickers still in the streak.")

	Commands["bts-sa"] = btsMC
	Usage["bts-sa"] = btsMCUsage
}

func btsMC() {
	err := btsMCFlagSet.Parse(flag.Args()[1:])
	if err != nil {
		log.Fatalf("Failed to parse bts-mc arguments: %v", err)
	}

	log.Print("Beating the streak")

	ctx := context.Background()
	pickers := btsMCFlagSet.Args()
	log.Printf("Beating the streak, pickers %s", pickers)
	weekNumber := weekFlag
	pickerNames := pickers

	if len(pickerNames) == 0 && !doAll {
		btsMCFlagSet.Usage()
		log.Fatal("Must supply at least one streaker if -all not set.")
	}

	fs, err := firestore.NewClient(ctx, ProjectID)
	if err != nil {
		log.Print(err)
		log.Fatalf("Check that the project ID \"%s\" is correctly specified (either the -project flag or the GCP_PROJECT environment variable)", ProjectID)
	}

	// Get season
	season, seasonRef, err := bpefs.GetSeason(ctx, fs, seasonFlag)
	if err != nil {
		log.Fatalf("Unable to get season: %v", err)
	}
	log.Printf("season discovered: %s", seasonRef.ID)

	// Get week
	_, weekRef, err := bpefs.GetWeek(ctx, fs, seasonRef, weekFlag)
	if err != nil {
		log.Fatalf("Unable to get week: %v", err)
	}
	log.Printf("week discovered: %s", weekRef.ID)

	// Get most recent Sagarin Ratings proper
	// I can cheat because I know this already.
	sagPointsRef := weekRef.Collection("team-points").Doc("sagarin")
	sagSnaps, err := sagPointsRef.Collection("linesag").Documents(ctx).GetAll()
	if err != nil {
		log.Fatalf("Unable to get sagarin ratings: %v", err)
	}
	sagarinRatings := make(map[string]bpefs.ModelTeamPoints)
	for _, s := range sagSnaps {
		var sag bpefs.ModelTeamPoints
		err = s.DataTo(&sag)
		if err != nil {
			log.Fatalf("Unable to get sagarin rating: %v", err)
		}
		// Sagarin has one nil team representing a non-recorded team. Don't keep that one.
		if sag.Team == nil {
			continue
		}
		sagarinRatings[sag.Team.ID] = sag
	}
	log.Printf("latest sagarin ratings discovered: %v", sagarinRatings)

	// Get the streakers for this week
	pickerMap, _, err := bpefs.GetRemainingStreaks(ctx, fs, seasonRef, weekRef)
	if err != nil {
		log.Fatalf("Unable to get remaining streaks: %v", err)
	}
	log.Printf("pickers loaded: %+v", pickerMap)
	if !doAll {
		foundPickers := make(map[string]struct{})
		for _, name := range pickerNames {
			if _, ok := pickerMap[name]; !ok {
				log.Fatalf("Picker '%s' does not have an active streak.", name)
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
		log.Printf("all pickers selected with -all flag")
	}

	// Get most recent performances for sagarin
	performances, performanceRefs, err := bpefs.GetMostRecentModelPerformances(ctx, fs, weekRef)
	if err != nil {
		log.Fatalf("Unable to get model performances: %v", err)
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
		log.Fatalf("Unable to retrieve most recent Sagarin performance for the week.")
	}
	log.Printf("Sagarin Ratings performance: %v", sagPerf)

	// Build the probability model
	model := bts.NewGaussianSpreadModel(sagarinRatings, sagPerf)
	log.Printf("Built model %v", model)

	// Get schedule from most recent season
	schedule, err := bts.MakeSchedule(ctx, fs, seasonRef, weekNumber, season.StreakTeams)
	if err != nil {
		log.Fatalf("Unable to make schedule: %v", err)
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
			log.Printf("Unable to make player: %v", err)
		}
	}

	log.Printf("Pickers loaded:\n%v", players)

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
	ppts := perPlayerTeamStreaks(playerItr, predictions)

	// Update best
	bestStreaks := calculateBestStreaks(ppts)

	// Collect by player
	streakOptions := collectByPlayer(bestStreaks, players, predictions, &schedule, seasonRef, weekRef, duplicates)

	// Print results
	output := weekRef.Collection("streak-predictions")

	if DryRun {
		log.Print("DRY RUN: Would write the following:")
	}
	for _, streak := range streakOptions {
		streak.Sagarin = sagPointsRef
		streak.PredictionTracker = sagPerfRef

		if DryRun {
			log.Printf("%s: add %+v", output.Path, streak)
			continue
		}

		log.Printf("Writing:\n%+v", streak)

		_, _, err := output.Add(ctx, streak)
		if err != nil {
			log.Fatalf("Unable to write streak to Firestore: %v", err)
		}
	}

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

func perPlayerTeamStreaks(ps <-chan *bts.Player, predictions *bts.Predictions) <-chan playerTeamStreakProb {

	out := make(chan playerTeamStreakProb, 100)

	go func(out chan<- playerTeamStreakProb) {
		var wg sync.WaitGroup
		sd := seed
		if sd < 0 {
			sd = time.Now().UnixNano()
		}
		src := rand.NewSource(sd)
		for p := range ps {
			for i := 0; i < workers; i++ {
				wg.Add(1)
				mySeed := src.Int63()
				go func(worker int, p *bts.Player, out chan<- playerTeamStreakProb) {
					anneal(mySeed, worker, p, predictions, out)
					wg.Done()
				}(i, p, out)
			}
		}
		wg.Wait()
		close(out)
	}(out)

	return out
}

func anneal(seed int64, worker int, p *bts.Player, predictions *bts.Predictions, out chan<- playerTeamStreakProb) {

	src := rand.NewSource(seed)
	rng := rand.New(src)

	maxIterations := maxItr
	tConst := tC
	tExp := tE
	maxDrift := resetItr
	countSinceReset := maxDrift

	s := bts.NewStreak(p.RemainingTeams(), <-p.WeekTypeIterator())
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

					// opponent := schedule.Get(team, iweek).Team(1)
					// Cheat because I have the ID
					teamRef := seasonRef.Collection("teams").Doc(string(team))
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
