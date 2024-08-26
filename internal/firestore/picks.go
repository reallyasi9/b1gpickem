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

const PICKS_COLLECTION = "picks"
const STREAK_PICKS_COLLECTION = "streak-picks"

type SlateRowBuilder interface {
	// BuildSlateRow creates a row of strings for output into a slate spreadsheet.
	BuildSlateRows(ctx context.Context) ([][]string, error)
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

	// Timestamp is the time the picks were written to Firestore.
	Timestamp time.Time `firestore:"timestamp,serverTimestamp"`

	// Picker is a reference to the picker who made the picks.
	Picker *firestore.DocumentRef `firestore:"picker"`
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
	ss = append(ss, treeFloat64("PredictedProbability", 0, false, p.PredictedProbability))
	ss = append(ss, treeRef("Picker", 0, true, p.Picker))
	sb.WriteString(strings.Join(ss, "\n"))
	return sb.String()
}

// GetPicks gets a picker's picks for a given week.
func GetPicks(ctx context.Context, weekRef, pickerRef *firestore.DocumentRef) (picks []Pick, pickRefs []*firestore.DocumentRef, err error) {
	picks = make([]Pick, 0)
	pickRefs = make([]*firestore.DocumentRef, 0)

	snaps, err := weekRef.Collection(PICKS_COLLECTION).Where("picker", "==", pickerRef).Documents(ctx).GetAll()
	if err != nil {
		return
	}
	for _, snap := range snaps {
		var pick Pick
		if err = snap.DataTo(&pick); err != nil {
			return
		}
		picks = append(picks, pick)
		pickRefs = append(pickRefs, snap.Ref)
	}

	return
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

	// Timestamp is the time the picks were written to Firestore.
	Timestamp time.Time `firestore:"timestamp,serverTimestamp"`

	// Picker is a reference to the picker who made the picks.
	Picker *firestore.DocumentRef `firestore:"picker"`
}

// GetStreakPick gets a picker's BTS pick for a given week.
func GetStreakPick(ctx context.Context, weekRef, pickerRef *firestore.DocumentRef) (pick StreakPick, ref *firestore.DocumentRef, err error) {
	snaps, err := weekRef.Collection(STREAK_PICKS_COLLECTION).Where("picker", "==", pickerRef).Documents(ctx).GetAll()
	if err != nil {
		return
	}
	if len(snaps) == 0 {
		err = NoStreakPickError(fmt.Sprintf("picker %s has no streak pick for week %s", pickerRef.ID, weekRef.ID))
		return
	}
	if len(snaps) > 1 {
		err = fmt.Errorf("ambiguous streak pick for picker %s in week %s", pickerRef.ID, weekRef.ID)
		return
	}
	if err = snaps[0].DataTo(&pick); err != nil {
		return
	}
	ref = snaps[0].Ref

	return
}

// GetStreakPicks gets all pickers' BTS picks for a given week.
func GetStreakPicks(ctx context.Context, weekRef *firestore.DocumentRef) (picks []StreakPick, refs []*firestore.DocumentRef, err error) {
	snaps, err := weekRef.Collection(STREAK_PICKS_COLLECTION).Documents(ctx).GetAll()
	if err != nil {
		return
	}
	picks = make([]StreakPick, len(snaps))
	refs = make([]*firestore.DocumentRef, len(snaps))
	for i, snap := range snaps {
		var sp StreakPick
		if err = snap.DataTo(&sp); err != nil {
			return
		}
		picks[i] = sp
		refs[i] = snap.Ref
	}

	return
}

// BuildSlateRow fills out the remaining 4 cells for a pick in a slate.
func (p Pick) BuildSlateRows(ctx context.Context) ([][]string, error) {
	// game, instruction, pick, spread, notes, expected value
	output := make([][]string, 1)
	line := make([]string, 6)

	// need to know the game to get the notes right
	var sgameDoc *firestore.DocumentSnapshot
	var err error
	if sgameDoc, err = p.SlateGame.Get(ctx); err != nil {
		return nil, err
	}
	var sgame SlateGame
	if err = sgameDoc.DataTo(&sgame); err != nil {
		return nil, err
	}

	// need to know the home and away teams, so need to get the game proper.
	var gameDoc *firestore.DocumentSnapshot
	if gameDoc, err = sgame.Game.Get(ctx); err != nil {
		return nil, err
	}
	var game Game
	if err = gameDoc.DataTo(&game); err != nil {
		return nil, err
	}

	var homeTeamDoc *firestore.DocumentSnapshot
	if homeTeamDoc, err = game.HomeTeam.Get(ctx); err != nil {
		return nil, err
	}
	var homeTeam Team
	if err = homeTeamDoc.DataTo(&homeTeam); err != nil {
		return nil, err
	}
	var awayTeamDoc *firestore.DocumentSnapshot
	if awayTeamDoc, err = game.AwayTeam.Get(ctx); err != nil {
		return nil, err
	}
	var awayTeam Team
	if err = awayTeamDoc.DataTo(&awayTeam); err != nil {
		return nil, err
	}

	// Differentiate home and away team mascots if they are the same
	if homeTeam.Mascot == awayTeam.Mascot {
		homeTeam.Mascot = fmt.Sprintf("%s (of the %s variety)", homeTeam.Mascot, homeTeam.School)
		awayTeam.Mascot = fmt.Sprintf("%s (of the %s variety)", awayTeam.Mascot, awayTeam.School)
	}

	// Game is straight forward
	var gameSB strings.Builder
	if sgame.Superdog {
		if sgame.HomeFavored {
			if sgame.AwayRank > 0 {
				gameSB.WriteString(fmt.Sprintf("#%d ", sgame.AwayRank))
			}
			gameSB.WriteString(awayTeam.School)
		} else {
			if sgame.HomeRank > 0 {
				gameSB.WriteString(fmt.Sprintf("#%d ", sgame.HomeRank))
			}
			gameSB.WriteString(homeTeam.School)
		}
		gameSB.WriteString(" upsets ")
		if sgame.HomeFavored {
			if sgame.HomeRank > 0 {
				gameSB.WriteString(fmt.Sprintf("#%d ", sgame.HomeRank))
			}
			gameSB.WriteString(homeTeam.School)
			gameSB.WriteString(" on the road")
		} else {
			if sgame.AwayRank > 0 {
				gameSB.WriteString(fmt.Sprintf("#%d ", sgame.AwayRank))
			}
			gameSB.WriteString(awayTeam.School)
			gameSB.WriteString(" at home")
		}
	} else {
		if sgame.GOTW {
			gameSB.WriteString("** ")
		}
		if sgame.AwayRank > 0 {
			gameSB.WriteString(fmt.Sprintf("#%d ", sgame.AwayRank))
		}
		gameSB.WriteString(awayTeam.School)
		if game.NeutralSite {
			gameSB.WriteString(" vs ")
		} else {
			gameSB.WriteString(" @ ")
		}
		if sgame.HomeRank > 0 {
			gameSB.WriteString(fmt.Sprintf("#%d ", sgame.HomeRank))
		}
		gameSB.WriteString(homeTeam.School)
		if sgame.GOTW {
			gameSB.WriteString(" **")
		}
	}
	line[0] = gameSB.String()

	// Instructions only apply to noisy spreads
	var instructionsSB strings.Builder
	if sgame.NoisySpread != 0 {
		instructionsSB.WriteString("Pick ")
		if sgame.HomeFavored {
			instructionsSB.WriteString(homeTeam.School)
		} else {
			instructionsSB.WriteString(awayTeam.School)
		}
		ns := sgame.NoisySpread
		if ns < 0 {
			ns *= -1
		}
		instructionsSB.WriteString(fmt.Sprintf(" iff you think they will win by at least %d points", ns))
	}
	line[1] = instructionsSB.String()

	// only pick if the team is not nil (else this is a Superdog game that wasn't picked)
	if p.PickedTeam != nil {
		if p.PickedTeam.ID == homeTeamDoc.Ref.ID {
			line[2] = homeTeam.Mascot
		} else {
			line[2] = awayTeam.Mascot
		}
	}

	line[3] = fmt.Sprintf("%+0.2f", p.PredictedSpread)

	notes := make([]string, 0)
	if p.PredictedProbability > .8 {
		notes = append(notes, "Not even close.")
	}
	if sgame.Superdog && p.PredictedProbability > 0.5 {
		notes = append(notes, "The \"underdog\" might actually be favored!")
	}
	if sgame.NoisySpread == 0 && !sgame.Superdog && math.Abs(p.PredictedSpread) >= 14 {
		notes = append(notes, "Maybe this should have been a noisy spread game?")
	}
	if sgame.NeutralDisagreement {
		notes = append(notes, "NOTE: This game might not be where you think it is.")
	}
	if sgame.HomeDisagreement {
		notes = append(notes, "NOTE: The home team might not be who you think it is.")
	}
	line[4] = strings.Join(notes, "\n")

	line[5] = fmt.Sprintf("%0.3f", p.PredictedProbability*float64(sgame.Value))

	output[0] = line

	return output, nil
}

// BuildSlateRow creates a row of strings for direct output to a slate spreadsheet.
// TODO: still not printing DDs correctly.
func (sg StreakPick) BuildSlateRows(ctx context.Context) ([][]string, error) {
	tupleStrings := [...]string{
		"BEAT THE STREAK!",
		"DOUBLE DOWN!",
		"TRIPLE DOWN!",
		"QUADRUPLE DOWN!",
		"QUINTUPLE DOWN!",
		"SEXTUPLE DOWN!",
		"SEPTUPLE DOWN!",
		"OCTUPLE DOWN!",
		"NONTUPLE DOWN!",
		"DECTUPLE DOWN!",
	}

	// game, instruction, pick(s), total remaining spread, notes?, probability of beating the streak
	output := make([][]string, 0)

	if len(sg.PickedTeams) == 0 {
		line := make([]string, 6)
		line[0] = tupleStrings[0]
		line[2] = "BYE"
		line[3] = fmt.Sprintf("%0.12f", sg.PredictedSpread)
		line[5] = fmt.Sprintf("%0.4f", sg.PredictedProbability)
		output = append(output, line)
	} else {
		for i, teamRef := range sg.PickedTeams {
			var team Team
			snap, err := teamRef.Get(ctx)
			if err != nil {
				return nil, fmt.Errorf("unable to get team %s: %w", teamRef.ID, err)
			}
			err = snap.DataTo(&team)
			if err != nil {
				return nil, fmt.Errorf("unable to build team from data %+v: %w", snap, err)
			}

			line := make([]string, 6)
			if i > len(tupleStrings) {
				line[0] = fmt.Sprintf("%d-TUPLE DOWN!", i+1)
			} else {
				line[0] = tupleStrings[i]
			}
			line[2] = team.Mascot
			line[3] = fmt.Sprintf("%0.12f", sg.PredictedSpread)
			line[5] = fmt.Sprintf("%0.4f", sg.PredictedProbability)
			output = append(output, line)
		}
	}

	return output, nil
}
