package pyp

import (
	"fmt"
	"log"
	"math/rand"

	"sort"
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

func Simulate(ctx *Context) error {
	log.Print("Picking your ponies")

	fs := ctx.FirestoreClient

	// Get season
	season, seasonRef, err := bpefs.GetSeason(ctx, fs, ctx.Season)
	if err != nil {
		return fmt.Errorf("Simulate: unable to get season: %v", err)
	}
	log.Printf("season discovered: %s", seasonRef.ID)

	// Get the most recent week
	week, weekRef, err := bpefs.GetFirstWeek(ctx, seasonRef)
	if err != nil {
		return fmt.Errorf("Simulate: unable to get week: %v", err)
	}
	log.Printf("first week discovered: %s", weekRef.ID)

	// Get most recent Sagarin Ratings proper
	// I can cheat because I know this already.
	sagPointsRef := weekRef.Collection("team-points").Doc("sagarin")
	sagSnaps, err := sagPointsRef.Collection("linesag").Documents(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("Simulate: unable to get sagarin ratings: %v", err)
	}
	sagarinRatings := make(map[string]bpefs.ModelTeamPoints)
	for _, s := range sagSnaps {
		var sag bpefs.ModelTeamPoints
		err = s.DataTo(&sag)
		if err != nil {
			return fmt.Errorf("Simulate: unable to get sagarin rating: %v", err)
		}
		// Sagarin has one nil team representing a non-recorded team. Don't keep that one.
		if sag.Team == nil {
			continue
		}
		sagarinRatings[sag.Team.ID] = sag
	}
	log.Printf("latest sagarin ratings discovered: %v", sagarinRatings)

	// Get most recent performances for sagarin
	performances, _, err := bpefs.GetMostRecentModelPerformances(ctx, fs, weekRef)
	if err != nil {
		return fmt.Errorf("Simulate: unable to get model performances: %v", err)
	}
	var sagPerf bpefs.ModelPerformance
	sagPerfFound := false
	for _, perf := range performances {
		if perf.Model.ID == "linesag" {
			sagPerf = perf
			sagPerfFound = true
			break
		}
	}
	if !sagPerfFound {
		return fmt.Errorf("Simulate: unable to retrieve most recent Sagarin performance for the week")
	}
	log.Println("Sagarin Ratings acquired")

	// Build the probability model
	model := bts.NewGaussianSpreadModel(sagarinRatings, sagPerf)
	log.Println("Built model")

	// Get schedule from most recent season
	pypTeamRefs := make([]*firestore.DocumentRef, 0, len(season.PonyTeams))
	for id := range season.PonyTeams {
		pypTeamRefs = append(pypTeamRefs, seasonRef.Collection(bpefs.TEAMS_COLLECTION).Doc(id))
	}

	// FIXME: this takes forever. Why?
	schedule, err := bts.MakeSchedule(ctx, seasonRef, week.Number, pypTeamRefs)
	if err != nil {
		return fmt.Errorf("Simulate: unable to make schedule: %w", err)
	}
	log.Printf("Schedule built:\n%v", schedule)

	// Get names for human readability later
	teamObjs, err := bpefs.GetAll[bpefs.Team](ctx, fs, pypTeamRefs)
	if err != nil {
		return fmt.Errorf("Simulate: unable to get human-readable team objects: %w", err)
	}
	teamIDLookup := make(map[string]string)
	for i, teamRef := range pypTeamRefs {
		teamIDLookup[teamRef.ID] = teamObjs[i].School
	}

	games := schedule.UniqueGames()
	log.Printf("Found %d unique games", len(games))

	spreads := predictSpreads(games, model)
	log.Println("Made predictions")

	// Here we go.
	log.Println("Starting MC")

	var nGamesPerSeason int
	for _, week := range schedule {
		if len(week) > nGamesPerSeason {
			nGamesPerSeason = len(week)
		}
	}
	// output: histogram of wins per team for all simulations, runs from 0 to the number of games per season.
	winHists := make(map[string][]int)
	for team := range season.PonyTeams {
		// Seasons always include bye weeks, but just in case some team plays no byes, we need an extra value in the slice to account for that.
		winHists[team] = make([]int, nGamesPerSeason+1)
	}

	// Loop through games and draw a random outcome for each game
	seed := ctx.Seed
	if seed == -1 {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))
	for iter := 0; iter < ctx.Iterations; iter++ {
		teamWins := make(map[string]int)
		for igame, spread := range spreads {
			if games[igame].Team(0) == bts.BYE || games[igame].Team(1) == bts.BYE {
				continue // bye games do not count
			}
			outcome := rng.NormFloat64()*sagPerf.StdDev + spread
			winTeam := 0
			if outcome < 0 {
				winTeam = 1
			}
			winningTeam := string(games[igame].Team(winTeam))
			teamWins[winningTeam]++
			if teamWins[winningTeam] > 12 {
				winHists[winningTeam][100]++
			}
		}
		for team := range season.PonyTeams {
			winHists[team][teamWins[team]]++
		}
	}

	log.Printf("Predicted histograms:\n%v", winHists)

	// Calculate expected points and total upside risk, split by B1G and non-B1G
	expectedPoints := make(map[string]float64)
	upsideRisk := make(map[string]float64)
	b1gExpectedPoints := make(map[string]float64)
	b1gUpsideRisk := make(map[string]float64)
	for team, pred := range season.PonyTeams {
		hist := winHists[team]
		for nwins, nseasons := range hist {
			p := float64(nseasons) / float64(ctx.Iterations)
			if pred < 0 {
				pointsGained := -pred - float64(nwins)
				expectedPoints[team] += p * pointsGained
				if pointsGained > 0 {
					upsideRisk[team] += p
				}
			} else {
				pointsGained := float64(nwins) - pred
				b1gExpectedPoints[team] += p * pointsGained
				if pointsGained > 0 {
					b1gUpsideRisk[team] += p
				}
			}
		}
	}

	risks := make([]Risk, 0, len(expectedPoints))
	b1gRisks := make([]Risk, 0, len(b1gExpectedPoints))
	for name := range expectedPoints {
		risks = append(risks, Risk{Team: name, ExpectedPoints: expectedPoints[name], UpsideRisk: upsideRisk[name]})
	}
	for name := range b1gExpectedPoints {
		b1gRisks = append(b1gRisks, Risk{Team: name, ExpectedPoints: b1gExpectedPoints[name], UpsideRisk: b1gUpsideRisk[name]})
	}

	// Print results of simulation
	fmt.Println("Highest Expected Points (B1G):")
	sort.Sort(sort.Reverse(ByExpectedPoints(b1gRisks)))
	for _, risk := range b1gRisks {
		fmt.Printf("%s: %f\n", teamIDLookup[risk.Team], risk.ExpectedPoints)
	}
	fmt.Println("Highest Upside Risk (B1G):")
	sort.Sort(sort.Reverse(ByUpsideRisk(b1gRisks)))
	for _, risk := range b1gRisks {
		fmt.Printf("%s: %f\n", teamIDLookup[risk.Team], risk.UpsideRisk)
	}
	fmt.Println()
	fmt.Println("Highest Expected Points (Top 25):")
	sort.Sort(sort.Reverse(ByExpectedPoints(risks)))
	for _, risk := range risks {
		fmt.Printf("%s: %f\n", teamIDLookup[risk.Team], risk.ExpectedPoints)
	}
	fmt.Println("Highest Upside Risk (Top 25):")
	sort.Sort(sort.Reverse(ByUpsideRisk(risks)))
	for _, risk := range risks {
		fmt.Printf("%s: %f\n", teamIDLookup[risk.Team], risk.UpsideRisk)
	}

	return nil
}

type Risk struct {
	Team           string
	ExpectedPoints float64
	UpsideRisk     float64
}

type ByExpectedPoints []Risk
type ByUpsideRisk []Risk

func (x ByExpectedPoints) Len() int {
	return len(x)
}
func (x ByExpectedPoints) Less(i, j int) bool {
	return x[i].ExpectedPoints < x[j].ExpectedPoints
}
func (x ByExpectedPoints) Swap(i, j int) {
	x[i], x[j] = x[j], x[i]
}

func (x ByUpsideRisk) Len() int {
	return len(x)
}
func (x ByUpsideRisk) Less(i, j int) bool {
	return x[i].UpsideRisk < x[j].UpsideRisk
}
func (x ByUpsideRisk) Swap(i, j int) {
	x[i], x[j] = x[j], x[i]
}

func predictSpreads(games []*bts.Game, model bts.PredictionModel) []float64 {
	out := make([]float64, len(games))
	for i, game := range games {
		_, spread := model.Predict(game)
		out[i] = spread
	}
	return out
}
