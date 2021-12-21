package enumerate

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"sort"
	"strconv"
	"sync"

	"github.com/reallyasi9/b1gpickem/internal/bts"

	bpefs "github.com/reallyasi9/b1gpickem/internal/firestore"

	progressbar "github.com/schollz/progressbar/v3"
)

func Enumerate(ctx *Context) error {
	log.Print("Enumerating Streaks")

	fs := ctx.FirestoreClient

	// Get season
	season, seasonRef, err := bpefs.GetSeason(ctx, fs, ctx.Season)
	if err != nil {
		return fmt.Errorf("Enumerate: unable to get season: %w", err)
	}
	log.Printf("season discovered: %s", seasonRef.ID)

	// Get all weeks
	weeks, weekRefs, err := bpefs.GetWeeks(ctx, seasonRef)
	if err != nil {
		return fmt.Errorf("Enumerate: unable to get weeks: %w", err)
	}
	log.Printf("discovered %d weeks", len(weekRefs))

	// Get all games from all weeks (assuming the results are noted in the games)
	allGames := make([]bpefs.Game, 0)
	for _, ref := range weekRefs {
		games, _, err := bpefs.GetGames(ctx, ref)
		if err != nil {
			return fmt.Errorf("Enumerate: failed to get games for week '%s': %w", ref.ID, err)
		}
		allGames = append(allGames, games...)
	}
	log.Printf("discovered %d games", len(allGames))

	// Build the probability model
	model := bts.NewOracleModel(allGames)
	log.Printf("Built model %s", model)

	// Get schedule from most recent season
	firstWeekNumber := weeks[0].Number
	schedule, err := bts.MakeSchedule(ctx, fs, seasonRef, firstWeekNumber, season.StreakTeams)
	if err != nil {
		return fmt.Errorf("Enumerate: unable to make schedule: %w", err)
	}
	log.Printf("Schedule built:\n%v", schedule)

	// Make predictions for fast lookup
	predictions := bts.MakePredictions(&schedule, *model)
	log.Printf("Made predictions\n%s", predictions)

	// Make default remaining teams
	streakTeams := make(bts.Remaining, len(season.StreakTeams))
	for i, ref := range season.StreakTeams {
		streakTeams[i] = bts.Team(ref.ID)
	}

	// Count number of weeks
	nWeeks := 0
	for i, n := range season.StreakPickTypes {
		nWeeks += i * n
	}
	log.Printf("Counted %d pick weeks", nWeeks)

	log.Println("Starting Enumeration")

	// Outcomes
	spreadByTeam := makeTeamWeekMatrix(streakTeams, nWeeks)
	successByTeam := makeTeamWeekMatrix(streakTeams, nWeeks)
	spreadByWeekType := makeWeekTypeMatrix(season.StreakPickTypes, nWeeks)
	successByWeekType := makeWeekTypeMatrix(season.StreakPickTypes, nWeeks)
	totalStreaks := big.NewInt(0)

	// Reporting
	weekTypePermutor := bts.NewIdenticalPermutor(season.StreakPickTypes...)
	streakPermuter := bts.NewIndexPermutor(len(season.StreakTeams))
	maxPermutations := weekTypePermutor.NumberOfPermutations()
	maxPermutations.Mul(maxPermutations, streakPermuter.NumberOfPermutations())
	log.Printf("Maximum number of streaks to check: %s", maxPermutations)

	streakSpreads := make(chan streakSpread, 100)
	go func(in <-chan streakSpread) {
		var bar *progressbar.ProgressBar
		if ctx.NoProgress {
			bar = progressbar.NewOptions64(maxPermutations.Int64(), progressbar.OptionSetVisibility(!ctx.NoProgress))
		} else {
			bar = progressbar.Default(maxPermutations.Int64())
		}
		var best int64
		one := big.NewInt(1)
		for ss := range in {
			bar.Add(1)
			totalStreaks.Add(totalStreaks, one)

			prob := ss.prob
			if prob <= 0 {
				continue
			}

			streak := ss.streak
			spread := ss.spread

			if spread > best {
				best = spread
				log.Printf("Best streak so far: %v (spread: %d)", *streak, spread)
			}

			s := big.NewInt(int64(spread))
			spreadByTeam.Add(streak, s)
			successByTeam.Add(streak, one)
			spreadByWeekType.Add(streak, s)
			successByWeekType.Add(streak, one)
		}
		bar.Finish()
	}(streakSpreads)

	var wg sync.WaitGroup
	for streak := range weeksToStreaks(weekTypePermutor.Iterator(), streakTeams) {
		// Loop through the possible pick order
		streakPermutor := bts.NewIndexPermutor(len(season.StreakTeams))
		for streakOrder := range streakPermutor.Iterator() {
			// Count the streak
			wg.Add(1)
			go func(streak *bts.Streak, streakOrder []int) {
				streak.PermuteTeamOrder(streakOrder)
				prob, spread := bts.SummarizeStreak(predictions, streak)
				streakSpreads <- streakSpread{streak: streak, spread: int64(spread), prob: prob}
				wg.Done()
			}(streak.Clone(), streakOrder)
		}
	}
	wg.Wait()
	close(streakSpreads)

	log.Printf("Done")
	fmt.Printf("Success by team (of %s valid streaks):\n", totalStreaks.String())
	if err = successByTeam.CSV(os.Stdout); err != nil {
		return fmt.Errorf("Enumerate: failed writing output: %w", err)
	}
	fmt.Printf("Spread by team (of %s valid streaks):\n", totalStreaks.String())
	if err = spreadByTeam.CSV(os.Stdout); err != nil {
		return fmt.Errorf("Enumerate: failed writing output: %w", err)
	}
	fmt.Printf("Success by week type (of %s valid streaks):\n", totalStreaks.String())
	if err = successByWeekType.CSV(os.Stdout); err != nil {
		return fmt.Errorf("Enumerate: failed writing output: %w", err)
	}
	fmt.Printf("Spread by week type (of %s valid streaks):\n", totalStreaks.String())
	if err = spreadByWeekType.CSV(os.Stdout); err != nil {
		return fmt.Errorf("Enumerate: failed writing output: %w", err)
	}

	return nil
}

type teamWeekMatrix map[bts.Team][]*big.Int

func makeTeamWeekMatrix(teams bts.Remaining, weeks int) teamWeekMatrix {
	cm := make(teamWeekMatrix)
	for _, team := range teams {
		counts := make([]*big.Int, weeks)
		for i := 0; i < weeks; i++ {
			counts[i] = big.NewInt(0)
		}
		cm[team] = counts
	}
	return cm
}

func (cm *teamWeekMatrix) Add(streak *bts.Streak, weight *big.Int) {
	for i := 0; i < streak.NumWeeks(); i++ {
		teams := streak.GetWeek(i)
		for _, team := range teams {
			if team == bts.NONE || team == bts.BYE {
				continue
			}
			bi := (*cm)[team][i]
			bi.Add(bi, weight)
		}
	}
}

// CSV writes a CSV document to the given `io.Writer`
func (cm *teamWeekMatrix) CSV(b io.Writer) error {
	w := csv.NewWriter(b)

	// sort teams
	teams := make([]string, len(*cm))
	i := 0
	for team := range *cm {
		teams[i] = string(team)
		i++
	}
	sort.Strings(teams)
	// print counts per week
	record := []string{
		"team",
		"week",
		"count",
	}
	if err := w.Write(record); err != nil {
		return fmt.Errorf("making CSV header: %w", err)
	}
	for _, team := range teams {
		record[0] = team
		for week, count := range (*cm)[bts.Team(team)] {
			record[1] = strconv.Itoa(week)
			record[2] = count.String()
			if err := w.Write(record); err != nil {
				return fmt.Errorf("making CSV record: %w", err)
			}
		}
	}
	w.Flush()
	return nil
}

type weekTypeMatrix [][]*big.Int

func makeWeekTypeMatrix(weekTypes []int, weeks int) weekTypeMatrix {
	cm := make(weekTypeMatrix, len(weekTypes))
	for i := range weekTypes {
		counts := make([]*big.Int, weeks)
		for j := 0; j < weeks; j++ {
			counts[j] = big.NewInt(0)
		}
		cm[i] = counts
	}
	return cm
}

func (cm *weekTypeMatrix) Add(streak *bts.Streak, weight *big.Int) {
	ppw := streak.PicksPerWeek(nil)
	for week, n := range ppw {
		bi := (*cm)[n][week]
		bi.Add(bi, weight)
	}
}

// CSV writes a CSV document to the given `io.Writer`
func (cm *weekTypeMatrix) CSV(b io.Writer) error {
	w := csv.NewWriter(b)

	record := []string{
		"picks",
		"week",
		"count",
	}
	if err := w.Write(record); err != nil {
		return fmt.Errorf("making CSV header: %w", err)
	}
	for picks, counts := range *cm {
		record[0] = strconv.Itoa(picks)
		for week, count := range counts {
			record[1] = strconv.Itoa(week)
			record[2] = count.String()
			if err := w.Write(record); err != nil {
				return fmt.Errorf("making CSV record: %w", err)
			}
		}
	}
	w.Flush()
	return nil
}

type streakSpread struct {
	streak *bts.Streak
	prob   float64
	spread int64
}

func weeksToStreaks(in <-chan []int, streakTeams bts.Remaining) chan *bts.Streak {
	out := make(chan *bts.Streak, 100)
	go func() {
		for weekTypes := range in {
			out <- bts.NewStreak(streakTeams, weekTypes)
		}
		close(out)
	}()
	return out
}
