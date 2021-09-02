package firestore

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
)

type SlateRowBuilder interface {
	// BuildSlateRow creates a row of strings for output into a slate spreadsheet.
	BuildSlateRow(ctx context.Context) ([]string, error)
}

// Picks represents a collection of pickers' picks for the week.
type Picks struct {
	// Season is a reference to the season document for these picks.
	Season *firestore.DocumentRef `firestore:"season"`

	// Week is the week number of the picks.
	Week int `firestore:"week"`

	// Timestamp is the time the picks were written to Firestore.
	Timestamp time.Time `firestore:"timestamp,serverTimestamp"`

	// Picker is a reference to the picker.
	Picker *firestore.DocumentRef `firestore:"picker"`
}

// StraightUpPick is a pick on a game with no spread.
type StraightUpPick struct {
	// HomeTeam is the true home team (not what is listed in the slate).
	HomeTeam *firestore.DocumentRef `firestore:"home"`

	// AwayTeam is the true road team (not what is listed in the slate).
	AwayTeam *firestore.DocumentRef `firestore:"road"`

	// HomeRank is the rank of the home team (zero implies unranked).
	HomeRank int `firestore:"rank1"`

	// AwayRank is the rank of the away team (zero implies unranked).
	AwayRank int `firestore:"rank2"`

	// GOTW stands for "Game of the Week."
	GOTW bool `firestore:"gotw"`

	// NeutralSite is the true neutral site nature of the game (not is listed in the slate).
	NeutralSite bool `firestore:"neutral_site"`

	// NeutralDisagreement is whether or not the slate lied to us about the neutral site of the game.
	NeutralDisagreement bool `firestore:"neutral_disagreement"`

	// Swap is whether or not the slate lied to us about who are the home and road teams.
	HomeAwaySwap bool `firestore:"swap"`

	// Pick is what the user picked, regardless of the model output.
	Pick *firestore.DocumentRef `firestore:"pick"`

	// PredictedSpread is the spread as predicted by the selected model.
	PredictedSpread float64 `firestore:"predicted_spread"`

	// PredictedProbability is the probability the pick is correct (including possible noisy spread adjustments).
	PredictedProbability float64 `firestore:"predicted_probability"`

	// ModeledGame is a reference to the spread from the model used to make the pick
	ModeledGame *firestore.DocumentRef `firestore:"modeled_game"`

	// Row is the row in the slate whence the pick originated.
	Row int `firestore:"row"`
}

// NoisySpreadPick is a pick on a noisy spread game.
type NoisySpreadPick struct {
	// HomeTeam is the true home team (not what is listed in the slate).
	HomeTeam *firestore.DocumentRef `firestore:"home"`

	// AwayTeam is the true road team (not what is listed in the slate).
	AwayTeam *firestore.DocumentRef `firestore:"road"`

	// HomeRank is the rank of the home team (zero implies unranked).
	HomeRank int `firestore:"rank1"`

	// AwayRank is the rank of the away team (zero implies unranked).
	AwayRank int `firestore:"rank2"`

	// NoisySpread is the spread against which the game is picked.
	NoisySpread int `firestore:"noisy_spread"`

	// NeutralSite is the true neutral site nature of the game (not is listed in the slate).
	NeutralSite bool `firestore:"neutral_site"`

	// NeutralDisagreement is whether or not the slate lied to us about the neutral site of the game.
	NeutralDisagreement bool `firestore:"neutral_disagreement"`

	// Swap is whether or not the slate lied to us about who are the home and road teams.
	HomeAwaySwap bool `firestore:"swap"`

	// Pick is what the user picked, regardless of the model output.
	Pick *firestore.DocumentRef `firestore:"pick"`

	// PredictedSpread is the spread as predicted by the selected model.
	PredictedSpread float64 `firestore:"predicted_spread"`

	// PredictedProbability is the probability the pick is correct (including possible noisy spread adjustments).
	PredictedProbability float64 `firestore:"predicted_probability"`

	// ModeledGame is a reference to the spread from the model used to make the pick
	ModeledGame *firestore.DocumentRef `firestore:"modeled_game"`

	// Row is the row in the slate whence the pick originated.
	Row int `firestore:"row"`
}

// SuperDogPick is a pick on a superdog spread game.
type SuperDogPick struct {
	// Underdog is what the slate called the underdog, regardless of model predictions.
	Underdog *firestore.DocumentRef `firestore:"underdog"`

	// Overdog is what the slate called the overdog, regardless of model predictions.
	Overdog *firestore.DocumentRef `firestore:"overdog"`

	// UnderdogRank is the rank of the underdog team (zero implies unranked).
	UnderdogRank int `firestore:"rank1"`

	// Overdog is the rank of the overdog team (zero implies unranked).
	OverdogRank int `firestore:"rank2"`

	// Value is the point value of the game.
	Value int `firestore:"value"`

	// NeutralSite is the true neutral site nature of the game (not is listed in the slate).
	NeutralSite bool `firestore:"neutral_site"`

	// NeutralDisagreement is whether or not the slate lied to us about the neutral site of the game.
	NeutralDisagreement bool `firestore:"neutral_disagreement"`

	// Swap is whether or not the slate lied to us about who are the home and road teams.
	HomeAwaySwap bool `firestore:"swap"`

	// Pick is what the user picked, regardless of the model output.
	// A nil value means this game was not picked by the user.
	Pick *firestore.DocumentRef `firestore:"pick"`

	// PredictedSpread is the spread as predicted by the selected model.
	PredictedSpread float64 `firestore:"predicted_spread"`

	// PredictedProbability is the probability the pick is correct (including possible noisy spread adjustments).
	PredictedProbability float64 `firestore:"predicted_probability"`

	// ModeledGame is a reference to the spread from the model used to make the pick
	ModeledGame *firestore.DocumentRef `firestore:"modeled_game"`

	// Row is the row in the slate whence the pick originated.
	Row int `firestore:"row"`
}

// StreakPick is a pick for Beat the Streak (BTS).
type StreakPick struct {
	// Picks is what the user picked, regardless of the model output.
	// Note that there could be multiple picks per week.
	Picks []*firestore.DocumentRef `firestore:"picks"`

	// PredictedSpread is the spread of the remaining games in the optimal streak as predicted by the selected model.
	PredictedSpread float64 `firestore:"predicted_spread"`

	// PredictedProbability is the probability of beating the streak.
	PredictedProbability float64 `firestore:"predicted_probability"`
}

// BuildSlateRow creates a row of strings for direct output to a slate spreadsheet.
func (sg StraightUpPick) BuildSlateRow(ctx context.Context) ([]string, error) {
	// game, noise, pick, spread, notes, expected value
	output := make([]string, 6)

	homeDoc, err := sg.HomeTeam.Get(ctx)
	if err != nil {
		return nil, err
	}
	var homeTeam Team
	if err := homeDoc.DataTo(&homeTeam); err != nil {
		return nil, err
	}
	roadDoc, err := sg.AwayTeam.Get(ctx)
	if err != nil {
		return nil, err
	}
	var roadTeam Team
	if err := roadDoc.DataTo(&roadTeam); err != nil {
		return nil, err
	}

	var sb strings.Builder

	if sg.GOTW {
		sb.WriteString("** ")
	}

	if sg.AwayRank > 0 {
		sb.WriteString(fmt.Sprintf("#%d ", sg.AwayRank))
	}

	sb.WriteString(roadTeam.School)

	if sg.NeutralSite {
		sb.WriteString(" vs. ")
	} else {
		sb.WriteString(" @ ")
	}

	if sg.HomeRank > 0 {
		sb.WriteString(fmt.Sprintf("#%d ", sg.HomeRank))
	}

	sb.WriteString(homeTeam.School)

	if sg.GOTW {
		sb.WriteString(" **")
	}

	output[0] = sb.String()

	pickedTeam := homeTeam
	if sg.Pick.ID == sg.AwayTeam.ID {
		pickedTeam = roadTeam
	}

	output[2] = pickedTeam.Name
	if homeTeam.Name == roadTeam.Name {
		output[2] = pickedTeam.School
	}

	output[3] = fmt.Sprintf("%0.1f", sg.PredictedSpread)

	sb.Reset()
	prob := sg.PredictedProbability
	// flip for away team winning
	if prob < 0.5 {
		prob = 1 - prob
	}
	if pickedTeam.School == "Michigan" {
		sb.WriteString("HARBAUGH!!!\n")
	}
	if prob > .8 {
		sb.WriteString("Not even close.\n")
	}
	if math.Abs(sg.PredictedSpread) >= 14 {
		sb.WriteString("Probably should have been noisy.\n")
	}
	if sg.NeutralDisagreement {
		if sg.NeutralSite {
			sb.WriteString("NOTE:  This game is at a neutral site.\n")
		} else {
			sb.WriteString("NOTE:  This game isn't at a neutral site.\n")
		}
	} else if sg.HomeAwaySwap {
		sb.WriteString("NOTE:  The home and away teams are reversed from their actual values.\n")
	}
	output[4] = strings.Trim(sb.String(), "\n")

	value := 1.
	if sg.GOTW {
		value = 2.
	}
	output[5] = fmt.Sprintf("%0.3f", value*prob)

	return output, nil
}

// BuildSlateRow creates a row of strings for direct output to a slate spreadsheet.
func (sg NoisySpreadPick) BuildSlateRow(ctx context.Context) ([]string, error) {
	// game, noise, pick, spread, notes, expected value
	output := make([]string, 6)

	homeDoc, err := sg.HomeTeam.Get(ctx)
	if err != nil {
		return nil, err
	}
	var homeTeam Team
	if err := homeDoc.DataTo(&homeTeam); err != nil {
		return nil, err
	}
	roadDoc, err := sg.AwayTeam.Get(ctx)
	if err != nil {
		return nil, err
	}
	var roadTeam Team
	if err := roadDoc.DataTo(&roadTeam); err != nil {
		return nil, err
	}

	var sb strings.Builder

	if sg.AwayRank > 0 {
		sb.WriteString(fmt.Sprintf("#%d ", sg.AwayRank))
	}

	sb.WriteString(roadTeam.School)

	if sg.NeutralSite {
		sb.WriteString(" vs. ")
	} else {
		sb.WriteString(" @ ")
	}

	if sg.HomeRank > 0 {
		sb.WriteString(fmt.Sprintf("#%d ", sg.HomeRank))
	}

	sb.WriteString(homeTeam.School)

	output[0] = sb.String()

	favorite := homeTeam
	ns := sg.NoisySpread
	if ns < 0 {
		favorite = roadTeam
		ns *= -1
	}
	output[1] = fmt.Sprintf("%s by ≥ %d", favorite.School, ns)

	pickedTeam := homeTeam
	if sg.Pick.ID == sg.AwayTeam.ID {
		pickedTeam = roadTeam
	}

	output[2] = pickedTeam.Name
	if homeTeam.Name == roadTeam.Name {
		output[2] = pickedTeam.School
	}

	output[3] = fmt.Sprintf("%0.1f", sg.PredictedSpread)

	sb.Reset()
	prob := sg.PredictedProbability
	// flip for away team winning
	if prob < 0.5 {
		prob = 1 - prob
	}
	if pickedTeam.School == "Michigan" {
		sb.WriteString("HARBAUGH!!!\n")
	}
	if prob > .8 {
		sb.WriteString("Not even close.\n")
	}
	if math.Abs(sg.PredictedSpread) < 14 {
		sb.WriteString("This one will be closer than you think.\n")
	}
	if sg.NeutralDisagreement {
		if sg.NeutralSite {
			sb.WriteString("NOTE:  This game is at a neutral site.\n")
		} else {
			sb.WriteString("NOTE:  This game isn't at a neutral site.\n")
		}
	} else if sg.HomeAwaySwap {
		sb.WriteString("NOTE:  The home and away teams are reversed from their actual values.\n")
	}
	output[4] = strings.Trim(sb.String(), "\n")

	output[5] = fmt.Sprintf("%0.3f", prob)

	return output, nil
}

// BuildSlateRow creates a row of strings for direct output to a slate spreadsheet.
func (sg SuperDogPick) BuildSlateRow(ctx context.Context) ([]string, error) {

	underDoc, err := sg.Underdog.Get(ctx)
	if err != nil {
		return nil, err
	}
	var underdog Team
	if err := underDoc.DataTo(&underdog); err != nil {
		return nil, err
	}
	overDoc, err := sg.Overdog.Get(ctx)
	if err != nil {
		return nil, err
	}
	var overdog Team
	if err := overDoc.DataTo(&overdog); err != nil {
		return nil, err
	}

	// game, value, pick, spread, notes, expected value
	output := make([]string, 6)

	var sb strings.Builder

	sb.WriteString(underdog.School)
	sb.WriteString(" over ")
	sb.WriteString(overdog.School)

	output[0] = sb.String()
	sb.Reset()

	output[1] = fmt.Sprintf("(%d points)", sg.Value)

	if sg.Pick != nil {
		output[2] = underdog.Name
		if underdog.Name == overdog.Name {
			output[2] = underdog.School
		}
	}

	output[3] = fmt.Sprintf("%0.1f", sg.PredictedSpread)

	if sg.PredictedProbability > 0.5 {
		output[4] = "NOTE:  The \"underdog\" is favored to win!"
	}

	output[5] = fmt.Sprintf("%0.4f", float64(sg.Value)*sg.PredictedProbability)

	return output, nil
}

// BuildSlateRow creates a row of strings for direct output to a slate spreadsheet.
// TODO: still not printing DDs correctly.
func (sg StreakPick) BuildSlateRow(ctx context.Context) ([]string, error) {

	pickedTeams := make([]Team, len(sg.Picks))
	for i, teamRef := range sg.Picks {
		t, err := teamRef.Get(ctx)
		if err != nil {
			return nil, err
		}
		var team Team
		err = t.DataTo(&team)
		if err != nil {
			return nil, err
		}
		pickedTeams[i] = team
	}

	// nothing, instruction, pick, spread, notes, expected value
	output := make([]string, 6)

	output[1] = "BEAT THE STREAK!"

	output[2] = strings.Join(uniqueTeamNames(pickedTeams), " + ")

	output[3] = fmt.Sprintf("%0.1f", sg.PredictedSpread)

	output[5] = fmt.Sprintf("%0.4f", sg.PredictedProbability)

	return output, nil
}

func uniqueTeamNames(teams []Team) []string {
	uniqueNames := make([]string, len(teams))
	names := make(map[string]struct{})
	useSchools := false
	for _, t := range teams {
		if _, exists := names[t.Name]; exists {
			useSchools = true
			break
		}
		names[t.Name] = struct{}{}
	}
	for i, t := range teams {
		if useSchools {
			uniqueNames[i] = t.School
		} else {
			uniqueNames[i] = t.Name
		}
	}
	return uniqueNames
}
