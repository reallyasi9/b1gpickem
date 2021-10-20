package firestore

import (
	"time"

	"cloud.google.com/go/firestore"
)

// StreakPredictions records the best predicted streak and the possible streaks for a given picker.
type StreakPredictions struct {
	// Picker is a reference to who is making the pick.
	Picker *firestore.DocumentRef `firestore:"picker"`

	// Season is a reference to the season of the pick.
	Season *firestore.DocumentRef `firestore:"season"`

	// Week is the week number of the pick (0 is just before the first week of the season).
	Week int `firestore:"week"`

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
	// Week is the week number of the pick (0 is just before the first week of the season).
	Week int `firestore:"week"`

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

// StreakTeamsRemainingWeek is a container for the remaining teams by picker.
type StreakTeamsRemainingWeek struct {
	// Season is a reference to the season of the pick.
	Season *firestore.DocumentRef `firestore:"season"`

	// Week is the week number of the pick (0 is just before the first week of the season).
	Week int `firestore:"week"`

	// Timestamp is the time the document was written to Firestore.
	Timestamp time.Time `firestore:"timestamp,serverTimestamp"`
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
