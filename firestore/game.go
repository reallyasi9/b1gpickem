package firestore

import "cloud.google.com/go/firestore"

// Game is a game's data for storing picks in Firestore.
type Game struct {
	// HomeTeam is the team that the slate calls the home team for this game. A value of `nil` will cause runtime errors.
	HomeTeam *firestore.DocumentRef `firestore:"home"`

	// AwayTeam is the team that the slate calls the away team for this game. A value of `nil` will cause runtime errors.
	AwayTeam *firestore.DocumentRef `firestore:"road"`

	// HomeRank is the rank given to `HomeTeam` by the slate. A rank of zero means the team is unranked.
	HomeRank int `firestore:"rank1"`

	// AwayRank is the rank given to `AwayTeam` by the slate. A rank of zero means the team is unranked.
	AwayRank int `firestore:"rank2"`

	// GOTW is true if this is a "game of the week."
	GOTW bool `firestore:"gotw"`

	// Superdog is true if this game is a "superdog pick."
	Superdog bool `firestore:"superdog"`

	// Overdog is the team favored to win a superdog pick game. Is `nil` if `!Superdog`, otherwise must be a valid document reference.
	Overdog *firestore.DocumentRef `firestore:"overdog"`

	// Underdog is the team not favored to win a superdog pick game. Is `nil` if `!Superdog`, otherwise must be a valid document reference.
	Underdog *firestore.DocumentRef `firestore:"underdog"`

	// Value is the point value of this game.
	Value int `firestore:"value"`

	// NeutralSite is true if the slate thinks this game takes place at a neutral site.
	NeutralSite bool `firestore:"neutral_site"`

	// NoisySpread is the spread against which the pickers are picking this game. A value of zero means a straight pick. Positive values favor `HomeTeam`.
	NoisySpread int `firestore:"noisy_spread"`

	// Row is the row in the original slate where the game was found when parsed. It is the row that will contain the pick when picks are made.
	Row int `firestore:"row"`
}
