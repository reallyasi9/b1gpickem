package bts

import (
	"fmt"
	"strings"

	"github.com/atgjack/prob"
	bpefs "github.com/reallyasi9/b1gpickem/internal/firestore"
)

// PredictionModel describes an object that can predict the probability a given team will defeat another team, or the point spread if those teams were to play.
type PredictionModel interface {
	MostLikelyOutcome(*Game) (team Team, prob float64, spread float64)
	Predict(*Game) (prob float64, spread float64)
}

// GaussianSpreadModel implements PredictionModel and uses a normal distribution based on spreads to calculate win probabilities.
// The spread is determined by a team rating and where the game is being played (to account for bias).
type GaussianSpreadModel struct {
	dist    prob.Normal
	ratings map[string]bpefs.ModelTeamPoints
}

// NewGaussianSpreadModel makes a model.
func NewGaussianSpreadModel(ratings map[string]bpefs.ModelTeamPoints, perf bpefs.ModelPerformance) *GaussianSpreadModel {
	return &GaussianSpreadModel{ratings: ratings, dist: prob.Normal{Mu: perf.Bias, Sigma: perf.StdDev}}
}

// Predict returns the probability and spread for team1.
func (m GaussianSpreadModel) Predict(game *Game) (float64, float64) {
	if game.Team(0) == BYE || game.Team(1) == BYE {
		return 0., 0.
	}
	if game.Team(0) == NONE || game.Team(1) == NONE {
		return 1., 0.
	}
	spread := m.spread(game)
	prob := m.dist.Cdf(spread)

	return prob, spread
}

// MostLikelyOutcome returns the most likely team to win a given game, the probability of win, and the predicted spread.
func (m GaussianSpreadModel) MostLikelyOutcome(game *Game) (Team, float64, float64) {
	if game.Team(0) == BYE || game.Team(1) == BYE {
		return BYE, 0., 0.
	}
	if game.Team(0) == NONE || game.Team(1) == NONE {
		return NONE, 1., 0.
	}
	prob, spread := m.Predict(game)
	if spread < 0 {
		return game.Team(1), 1 - prob, -spread
	}
	return game.Team(0), prob, spread
}

func (m GaussianSpreadModel) spread(game *Game) float64 {
	diff := m.ratings[string(game.Team(0))].Points - m.ratings[string(game.Team(1))].Points
	if game.LocationRelativeToTeam(0) != 0 {
		diff += m.ratings[string(game.Team(0))].HomeAdvantage
	}
	return diff
}

func (m GaussianSpreadModel) String() string {
	var b strings.Builder
	for t, r := range m.ratings {
		b.WriteString(fmt.Sprintf("%5s: %0.3f (home adv %0.3f)\n", t, r.Points, r.HomeAdvantage))
	}
	return b.String()
}
