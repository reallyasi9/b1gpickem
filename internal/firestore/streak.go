package firestore

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
)

// StreakPredictions records the best predicted streak and the possible streaks for a given picker.
type StreakPredictions struct {
	// Picker is a reference to who is making the pick.
	Picker *firestore.DocumentRef `firestore:"picker"`

	// TeamsRemaining are the teams the picker has remaining to pick in the streak.
	TeamsRemaining []*firestore.DocumentRef `firestore:"remaining"`

	// PickTypesRemaining is an array slice of number of pick types remaining for the user.
	// The index of the array represents the number of picks per week for that type.
	// For instance, the first (index 0) element in the array represents the number of "bye" picks the user has remaining,
	// while the second (index 1) element represents the number of "single" picks remaining,
	// and the third (index 2) represents the number of "double down" weeks remaining.
	PickTypesRemaining []int `firestore:"pick_types_remaining"`

	// Schedule is a reference to the schedule of games used to make these predictions.
	Schedule *firestore.DocumentRef `firestore:"schedule"`

	// Sagarin is a reference to the Sagarin prediction model used to make these predictions.
	Sagarin *firestore.DocumentRef `firestore:"sagarin"`

	// PredictionTracker is a reference to the TPT data used to evaluate the performance of the Sagarin model used to make these predictions.
	PredictionTracker *firestore.DocumentRef `firestore:"prediction_tracker"`

	// CalculationStartTime is when the program that produced the results started.
	CalculationStartTime time.Time `firestore:"calculation_start_time"`

	// CalculationEndTime is when the results were generated and finalized.
	CalculationEndTime time.Time `firestore:"calculation_end_time"`

	// BestPick is a reference to the team to pick this week that the model thinks gives the picker the best chance of beating the streak.
	// Multiple picks are possible per week.
	BestPick []*firestore.DocumentRef `firestore:"best_pick"`

	// Probability is the total probability of beating the streak given optimal selection.
	Probability float64 `firestore:"probability"`

	// Spread is the sum total spread in the picked games given optimal selection.
	Spread float64 `firestore:"spread"`

	// PossiblePicks are the optimal streaks calculated for each possible remaining pick.
	PossiblePicks []StreakPrediction `firestore:"possible_picks"`
}

// StreakWeek is a week's worth of streak picks.
type StreakWeek struct {
	// Pick is a reference to the team to pick this week that the model thinks gives the picker the best chance of beating the streak.
	// Multiple picks are possible per week.
	Pick []*firestore.DocumentRef `firestore:"pick"`

	// Probabilities are the probabilities of each team in `Pick` winning this week.
	Probabilities []float64 `firestore:"probabilities"`

	// Spreads are the predicted spreads of each game in `Pick` (positive favoring the picked team).
	Spreads []float64 `firestore:"spreads"`
}

// StreakPrediction is a prediction for a complete streak.
type StreakPrediction struct {

	// CumulativeProbability is the total cumulative probability of streak win for all the picks in `Weeks`.
	CumulativeProbability float64 `firestore:"cumulative_probability"`

	// CumulativeSpread is the total cumulative spreads for all the picks in `Weeks`.
	CumulativeSpread float64 `firestore:"cumulative_spread"`

	// Weeks are the picked streak winners for all future weeks.
	Weeks []StreakWeek `firestore:"weeks"`
}

// StreakTeamsRemaining represents the remaining teams and pick types per picker
type StreakTeamsRemaining struct {
	// Picker is a reference to the picker.
	Picker *firestore.DocumentRef `firestore:"picker"`

	// TeamsRemaining is a list of references to remaining teams for that picker.
	TeamsRemaining []*firestore.DocumentRef `firestore:"remaining"`

	// PickTypesRemaining is an array slice of number of pick types remaining for the user.
	// The index of the array represents the number of picks per week for that type.
	// For instance, the first (index 0) element in the array represents the number of "bye" picks the user has remaining,
	// while the second (index 1) element represents the number of "single" picks remaining,
	// and the third (index 2) represents the number of "double down" weeks remaining.
	PickTypesRemaining []int `firestore:"pick_types_remaining"`
}

// GetStreakTeamsRemaining looks up the remaining streak teams for a given picker, week combination.
// If week is nil, returns the remaining streak teams based off the season information.
func GetStreakTeamsRemaining(ctx context.Context, client *firestore.Client, season, week, picker *firestore.DocumentRef) (str StreakTeamsRemaining, ref *firestore.DocumentRef, err error) {
	if week == nil {
		s, e := season.Get(ctx)
		if e != nil {
			err = e
			return
		}
		var se Season
		e = s.DataTo(&se)
		if e != nil {
			err = e
			return
		}
		str.Picker = picker
		str.PickTypesRemaining = se.StreakPickTypes
		str.TeamsRemaining = se.StreakTeams
		return
	}

	coll := week.Collection("streak-teams-remaining")
	s, err := coll.Where("picker", "==", picker).Limit(1).Documents(ctx).GetAll()
	if err != nil {
		return
	}
	if len(s) != 1 {
		err = fmt.Errorf("expected 1 streak teams remaining element for picker '%s' in week '%s', got %d", picker.ID, week.ID, len(s))
		return
	}
	ref = s[0].Ref
	err = s[0].DataTo(&str)
	return
}

// GetRemainingStreaks looks up the remaining streaks for a given week, indexed by picker short name.
// If week is nil, returns new StreakTeamsRemaining objects for all pickers based off the season information.
func GetRemainingStreaks(ctx context.Context, client *firestore.Client, season, week *firestore.DocumentRef) (strs map[string]StreakTeamsRemaining, refs map[string]*firestore.DocumentRef, err error) {
	if week == nil {
		s, e := season.Get(ctx)
		if e != nil {
			err = e
			return
		}
		var se Season
		e = s.DataTo(&se)
		if e != nil {
			err = e
			return
		}
		pickers := se.Pickers
		strs = make(map[string]StreakTeamsRemaining)
		refs = make(map[string]*firestore.DocumentRef)
		for name, ref := range pickers {
			strs[name] = StreakTeamsRemaining{
				Picker:             ref,
				PickTypesRemaining: se.StreakPickTypes,
				TeamsRemaining:     se.StreakTeams,
			}
			refs[name] = nil // remember to create this later!
		}
		return
	}

	ss, err := week.Collection("streak-teams-remaining").Documents(ctx).GetAll()
	if err != nil {
		return
	}
	strs = make(map[string]StreakTeamsRemaining)
	refs = make(map[string]*firestore.DocumentRef)
	for _, s := range ss {
		var str StreakTeamsRemaining
		err = s.DataTo(&str)
		if err != nil {
			return
		}
		strs[str.Picker.ID] = str
		refs[str.Picker.ID] = s.Ref
	}
	return
}
