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
	MostLikelyNoisySpreadOutcome(*Game, float64) (team Team, prob float64, spread float64)
	PredictNoisySpread(*Game, float64) (prob float64, spread float64)
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

// PredictNoisySpread returns the probability that team1 beats the given spread and the predicted spread for team1.
// The noisy spread is relative to team1.
func (m GaussianSpreadModel) PredictNoisySpread(game *Game, noisySpread float64) (float64, float64) {
	if game.Team(0) == BYE || game.Team(1) == BYE {
		return 0., 0.
	}
	if game.Team(0) == NONE || game.Team(1) == NONE {
		return 1., 0.
	}
	spread := m.spread(game)
	prob := m.dist.Cdf(spread - noisySpread)

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

// MostLikelyNoisySpreadOutcome returns the most likely team to win a given noisy spread game, the probability of beating the spread, and the predicted spread.
func (m GaussianSpreadModel) MostLikelyNoisySpreadOutcome(game *Game, noisySpread float64) (Team, float64, float64) {
	if game.Team(0) == BYE || game.Team(1) == BYE {
		return BYE, 0., 0.
	}
	if game.Team(0) == NONE || game.Team(1) == NONE {
		return NONE, 1., 0.
	}
	prob, spread := m.PredictNoisySpread(game, noisySpread)
	if prob < 0.5 {
		return game.Team(1), 1 - prob, -spread
	}
	return game.Team(0), prob, spread
}

func (m GaussianSpreadModel) spread(game *Game) float64 {
	diff := m.ratings[string(game.Team(0))].Points - m.ratings[string(game.Team(1))].Points
	homeAdv := 0.
	switch game.LocationRelativeToTeam(0) {
	case Home:
		homeAdv = m.ratings[string(game.Team(0))].HomeAdvantage
	case Near:
		homeAdv = m.ratings[string(game.Team(0))].HomeAdvantage / 2.
	case Far:
		homeAdv = -m.ratings[string(game.Team(1))].HomeAdvantage / .2
	case Away:
		homeAdv = -m.ratings[string(game.Team(1))].HomeAdvantage
	}
	diff += homeAdv
	return diff
}

func (m GaussianSpreadModel) String() string {
	var b strings.Builder
	for t, r := range m.ratings {
		b.WriteString(fmt.Sprintf("%5s: %0.3f (home adv %0.3f)\n", t, r.Points, r.HomeAdvantage))
	}
	return b.String()
}

// OracleModel represents a Model that knows who won each matchup, so always returns a probability of win of either 1 or 0,
// and a spread equal to the scoring margin in the game.
type OracleModel struct {
	results map[Game]float64
}

// NewOracleModel makes a model.
func NewOracleModel(results []bpefs.Game) *OracleModel {
	out := make(map[Game]float64)
	for _, g := range results {
		// Games that are not completed do not get added to the model.
		if (g.HomePoints == nil) || (g.AwayPoints == nil) {
			continue
		}

		ht := g.HomeTeam.ID
		at := g.AwayTeam.ID
		hl := Home
		al := Away
		if g.NeutralSite {
			hl = Neutral
			al = Neutral
		}
		hg := Game{
			team1:    Team(ht),
			team2:    Team(at),
			location: hl,
		}
		ag := Game{
			team1:    Team(at),
			team2:    Team(ht),
			location: al,
		}

		out[hg] = float64(*g.HomePoints - *g.AwayPoints)
		out[ag] = float64(*g.AwayPoints - *g.HomePoints)
	}
	return &OracleModel{results: out}
}

// MostLikelyOutcome returns the historical winner of g, a probability of 1, and a spread equal to the scoring margin.
// If the two teams did not play each other, returns g.team1 and a probability and spread of zero.
func (m OracleModel) MostLikelyOutcome(g *Game) (team Team, prob float64, spread float64) {
	if g.Team(0) == BYE || g.Team(1) == BYE {
		return BYE, 0., 0.
	}
	if g.Team(0) == NONE || g.Team(1) == NONE {
		return NONE, 1., 0.
	}

	var ok bool
	team = g.Team(0)
	spread, ok = m.results[*g]
	if !ok {
		return
	}
	prob = 1
	if spread < 0 {
		team = g.Team(1)
		spread *= -1
	}
	return
}

// MostLikelyNoisySpreadOutcome returns whether or not team1 historically beat the given spread, a probability of 1, and the actual spread.
// If the two teams did not play each other, returns g.team1 and a probability and spread of zero.
// The noisy spread is positive in the direction of team1 winning.
func (m OracleModel) MostLikelyNoisySpreadOutcome(g *Game, noisySpread float64) (team Team, prob float64, spread float64) {
	if g.Team(0) == BYE || g.Team(1) == BYE {
		return BYE, 0., 0.
	}
	if g.Team(0) == NONE || g.Team(1) == NONE {
		return NONE, 1., 0.
	}

	var ok bool
	team = g.Team(0)
	spread, ok = m.results[*g]
	if !ok {
		return
	}
	prob = 1
	if spread < noisySpread {
		team = g.Team(1)
		spread *= -1
	}
	return
}

// Predict returns a probability of 1 if g.team1 won the game, a probability of 0 if g.team1 lost (or the game did not happen), and a spread equal to the scoring margin (or zero if the game did not happen).
func (m OracleModel) Predict(g *Game) (prob float64, spread float64) {
	if g.Team(0) == BYE || g.Team(1) == BYE {
		return 0., 0.
	}
	if g.Team(0) == NONE || g.Team(1) == NONE {
		return 1., 0.
	}

	var ok bool
	spread, ok = m.results[*g]
	if !ok {
		return
	}
	if spread > 0 {
		prob = 1
	}
	return
}

// PredictNoisySpread returns a probability of 1 if g.team1 beat the given spread, a probability of 0 if g.team1 did not (or the game did not happen), and a spread equal to the scoring margin (or zero if the game did not happen).
func (m OracleModel) PredictNoisySpread(g *Game, noisySpread float64) (prob float64, spread float64) {
	if g.Team(0) == BYE || g.Team(1) == BYE {
		return 0., 0.
	}
	if g.Team(0) == NONE || g.Team(1) == NONE {
		return 1., 0.
	}

	var ok bool
	spread, ok = m.results[*g]
	if !ok {
		return
	}
	if spread > noisySpread {
		prob = 1
	}
	return
}

// String implements the Stringer interface.
func (m OracleModel) String() string {
	nGames := len(m.results)
	uniqueTeams := make(map[Team]struct{})
	for game := range m.results {
		uniqueTeams[game.team1] = struct{}{}
		uniqueTeams[game.team2] = struct{}{}
	}
	nTeams := len(uniqueTeams)
	return fmt.Sprintf("OracleModel of %d teams playing %d games", nTeams, nGames/2)
}
