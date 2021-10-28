package firestore

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"gonum.org/v1/gonum/stat/distuv"
)

type SlateRowBuilder interface {
	// BuildSlateRow creates a row of strings for output into a slate spreadsheet.
	BuildSlateRow(ctx context.Context) ([]string, error)
}

// Picks represents a collection of pickers' picks for the week.
type Picks struct {
	// Season is a reference to the season document for these picks.
	Season *firestore.DocumentRef `firestore:"season"`

	// Week is a reference to the week of the picks.
	Week *firestore.DocumentRef `firestore:"week"`

	// Slate is a reference to the slate containing the picks.
	Slate *firestore.DocumentRef `firestore:"slate"`

	// Timestamp is the time the picks were written to Firestore.
	Timestamp time.Time `firestore:"timestamp,serverTimestamp"`

	// Picker is a reference to the picker who made the picks.
	Picker *firestore.DocumentRef `firestore:"picker"`

	// Picks is a map of references to picks, indexed by row in the original slate. See: Slate.Games for the order.
	Picks map[int]*firestore.DocumentRef `firestore:"picks"`
}

// Pick is a pick on a game. See: SlateGame, ModelPrediction, and Team for references.
type Pick struct {
	// SlateGame is a reference to the picked game in the slate.
	SlateGame *firestore.DocumentRef `firestore:"game"`

	// ModelPrediction is a reference to the spread from the model used to make the pick
	ModelPrediction *firestore.DocumentRef `firestore:"model_prediction"`

	// PickedTeam is the Team the Picker picked, regardless of the model output. Can be nil if this pick is for a "superdog" game and the underdog was not picked.
	PickedTeam *firestore.DocumentRef `firestore:"pick"`

	// PredictedSpread is the spread as predicted by the selected model.
	PredictedSpread float64 `firestore:"predicted_spread"`

	// PredictedProbability is the probability the pick is correct (including possible noisy spread adjustments).
	PredictedProbability float64 `firestore:"predicted_probability"`
}

// String implements Stringer interface
func (p Pick) String() string {
	var sb strings.Builder
	sb.WriteString("Pick\n")
	ss := make([]string, 0)
	ss = append(ss, treeRef("SlateGame", 0, false, p.SlateGame))
	ss = append(ss, treeRef("ModelPrediction", 0, false, p.ModelPrediction))
	ss = append(ss, treeRef("PickedTeam", 0, false, p.PickedTeam))
	ss = append(ss, treeFloat64("PredictedSpread", 0, false, p.PredictedSpread))
	ss = append(ss, treeFloat64("PredictedProbability", 0, true, p.PredictedProbability))
	sb.WriteString(strings.Join(ss, "\n"))
	return sb.String()
}

// FillOut uses game and model performance information to fill out a pick
func (p *Pick) FillOut(game Game, perf ModelPerformance, pred ModelPrediction, predRef *firestore.DocumentRef, spread int) {
	dist := distuv.Normal{Mu: perf.Bias, Sigma: perf.StdDev}
	p.ModelPrediction = predRef
	p.PredictedSpread = pred.Spread
	p.PredictedProbability = dist.CDF(p.PredictedSpread - float64(spread))
	p.PickedTeam = game.HomeTeam
	if p.PredictedProbability < .5 {
		p.PredictedProbability = 1. - p.PredictedProbability
		p.PickedTeam = game.AwayTeam
	}
}

// StreakPick is a pick for Beat the Streak (BTS).
type StreakPick struct {
	// PickedTeams is what the user picked, regardless of the model output.
	// Note that there could be multiple picks per week.
	// An empty array represents a bye pick.
	PickedTeams []*firestore.DocumentRef `firestore:"picks"`

	// StreakPredictions is a reference to the full streak predictions document used to make the pick.
	StreakPredictions *firestore.DocumentRef `firestore:"streak_predictions"`

	// PredictedSpread is the spread of the remaining games in the optimal streak as predicted by the selected model.
	PredictedSpread float64 `firestore:"predicted_spread"`

	// PredictedProbability is the probability of beating the streak.
	PredictedProbability float64 `firestore:"predicted_probability"`
}

// BuildSlateRow fills out the remaining 4 cells for a pick in a slate.
func (p Pick) BuildSlateRow(ctx context.Context) ([]string, error) {
	// pick, spread, notes, expected value
	output := make([]string, 4)

	// need to know the game to get the notes right
	var gameDoc *firestore.DocumentSnapshot
	var err error
	if gameDoc, err = p.SlateGame.Get(ctx); err != nil {
		return nil, err
	}
	var game SlateGame
	if err = gameDoc.DataTo(&game); err != nil {
		return nil, err
	}

	// only pick if the team is not nil (else this is a Superdog game that wasn't picked)
	if p.PickedTeam != nil {
		var doc *firestore.DocumentSnapshot
		var err error
		if doc, err = p.PickedTeam.Get(ctx); err != nil {
			return nil, err
		}
		var pt Team
		if err = doc.DataTo(&pt); err != nil {
			return nil, err
		}

		output[0] = pt.School
	}

	output[1] = fmt.Sprintf("%+0.2f", p.PredictedSpread)

	notes := make([]string, 0)
	if p.PredictedProbability > .8 {
		notes = append(notes, "Not even close.")
	}
	if game.Superdog && p.PredictedProbability > 0.5 {
		notes = append(notes, "The \"underdog\" might actually be favored!")
	}
	if game.NoisySpread == 0 && math.Abs(p.PredictedSpread) >= 14 {
		notes = append(notes, "Maybe this should have been a noisy spread game?")
	}
	if game.NeutralDisagreement {
		notes = append(notes, "NOTE: This game might not be where you think it is.")
	}
	if game.HomeDisagreement {
		notes = append(notes, "NOTE: The slate seems to be incorrect about which team is home and which is away!")
	}
	output[2] = strings.Join(notes, "\n")

	output[3] = fmt.Sprintf("%0.3f", p.PredictedProbability*float64(game.Value))

	return output, nil
}

// BuildSlateRow creates a row of strings for direct output to a slate spreadsheet.
// TODO: still not printing DDs correctly.
func (sg StreakPick) BuildSlateRow(ctx context.Context) ([]string, error) {

	// pick(s), total remaining spread, notes?, probability of beating the streak
	output := make([]string, 4)

	pickedTeams := make([]string, len(sg.PickedTeams))
	for i, teamRef := range sg.PickedTeams {
		t, err := teamRef.Get(ctx)
		if err != nil {
			return nil, err
		}
		var team Team
		err = t.DataTo(&team)
		if err != nil {
			return nil, err
		}
		pickedTeams[i] = team.School
	}

	output[0] = strings.Join(pickedTeams, "\n")

	output[1] = fmt.Sprintf("%0.12f", sg.PredictedSpread)

	output[3] = fmt.Sprintf("%0.4f", sg.PredictedProbability)

	return output, nil
}
