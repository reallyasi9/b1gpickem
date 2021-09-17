package firestore

import (
	fs "cloud.google.com/go/firestore"
)

// Model contains the information necessary to identify an NCAA football prediction model
// as defined by ThePredictionTracker.com.
type Model struct {
	// System is a long descriptive name of the model.
	// It is human-readable, and is chiefly used to identify the model on ThePredictionTracker.com public-facing web pages.
	System string `firestore:"system,omitempty"`

	// ShortName is a short label given to the model.
	// It is not always human-readable, and is used to identify the model on ThePredictionTracker.com's downloadable CSV files.
	// All begin with the string "line".
	ShortName string `firestore:"short_name,omitempty"`
}

// ModelPerformance contains information about how the model has performed to date during a given NCAA football season.
type ModelPerformance struct {
	// Rank is the weekly performance rank of the model as calculated by ThePredictionTracker.com using `PercentCorrect`.
	// The best performing model of the season to date is given `Rank = 1`.
	Rank int `firestore:"rank"`

	// PercentCorrect is the percent of games the model made a prediction for that were predicted correctly.
	// Because different models choose to predict only certain games every week, the denominator of this percentage
	// may not be consistent across models.
	PercentCorrect float64 `firestore:"pct_correct"`

	// PercentATS is the percent of games the model has predicted correctly against the opening line (Against The Spread).
	// This is only defined for models that predict the score of games rather than a boolean predicting who wins and who loses.
	// This is also not defined for the opening line model for obvious reasons.
	// For example of a correct pick against the opening line, imagine teams A and B are playing against each other and the opening
	// line is -5 (5 points in favor of team A). If the model predicts a spread of -7 (7 points in favor of team A) and the
	// final score of the game is 21-18 (team A wins by 3), then the model will be given a point for `PercentCorrect`, but not for
	// `PercentATS` because it predicted that team A would win by more than the opening line, but team A won by less than the opening line.
	PercentATS float64 `firestore:"pct_against_spread"`

	// MAE is the Mean Absolute Error in predicted score for games where the model has made a prediction.
	// The value is always non-negative.
	MAE float64 `firestore:"mae"`

	// MSE is the Mean Squared Error in predicted score for games where the model has made a prediction.
	// The value is always non-negative.
	MSE float64 `firestore:"mse"`

	// Bias is the mean error in predicted score for games where the model has made a prediction.
	// A positive value is a bias in favor of the home team (or the nominal home team if the game is played at a neutral site).
	Bias float64 `firestore:"bias"`

	// GamesPredicted are the number of games for which the model made a prediction. It is the denominator of the measures above.
	GamesPredicted int `firestore:"games"`

	// Wins is the number of correctly predicted game outcomes.
	Wins int `firestore:"suw"`

	// Losses is the number of incorrectly predicted game outcomes. Equal to `GamesPredicted - Wins`.
	Losses int `firestore:"sul"`

	// WinsATS are the wins "Against The Spread". It is the number of games in which the model correctly predicts whether the
	// difference in scores is on one side or the other of the opening line.
	// For example of a correct pick against the opening line, imagine teams A and B are playing against each other and the opening
	// line is -5 (5 points in favor of team A). If the model predicts a spread of -7 (7 points in favor of team A) and the
	// final score of the game is 21-18 (team A wins by 3), then the model will be given a point for `Wins`, but not for
	// `WinsATS` because it predicted that team A would win by more than the opening line, but team A won by less than the opening line.
	WinsATS int `firestore:"atsw"`

	// LossesATS are the losses "Against The Spread". It is the number of games in which the model incorrectly predicts whether the
	// difference in scores is on one side or the other of the opening line. Equal to `GamesPredicted - WinsATS`.
	LossesATS int `firestore:"atsl"`

	// StdDev is the standard deviation of the prediction errors.
	StdDev float64 `firestore:"std_dev"` // calculated

	// Model is a pointer to the Firestore model object it references for easy access.
	Model *fs.DocumentRef `firestore:"model"` // discovered
}

// ModelPrediction is a prediction made by a certain Model for a certain Game.
type ModelPrediction struct {
	// Model is a reference to the model that is making the prediction.
	Model *fs.DocumentRef `firestore:"model"`

	// HomeTeam is a reference to the Firestore Team the model thinks is the home team for the game.
	HomeTeam *fs.DocumentRef `firestore:"home_team"`

	// AwayTeam is a reference to the Firestore Team the model thinks is the away team for the game.
	AwayTeam *fs.DocumentRef `firestore:"away_team"`

	// NeutralSite flags if the model thinks the teams are playing at a neutral site.
	NeutralSite bool `firestore:"neutral"`

	// Spread is the predicted number of points in favor of `HomeTeam`.
	// Negative points reflect a prediction of `AwayTeam` winning.
	Spread float64 `firestore:"spread"`
}

// ModelTeamPoints represents a modeled number of points that a given team is expected to score against an average opponent.
// Some models model the team's scoring potential directly rather than the spread of a given game. This is extremely useful
// for predicting the spread of unscheduled or hypothetical games that other models do not attempt to predict.
// Only Sagarin and FPI models team scoring potential directly.
type ModelTeamPoints struct {
	// Model is a reference to the model that generates these scores.
	Model *fs.DocumentRef `firestore:"model"`

	// Team is a reference to the team.
	Team *fs.DocumentRef `firestore:"team"`

	// Points are the predicted points against an average team at a neutral site.
	Points float64 `firestore:"points"`

	// HomeAdvantage are the number of points added to the predicted points if this team is the home team.
	HomeAdvantage float64 `firestore:"home_advantage"`
}
