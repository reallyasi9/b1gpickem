package firestore

import (
	"time"

	"cloud.google.com/go/firestore"
)

// Schedule is a team's schedule in Firestore format
type Schedule struct {
	// Team is a reference to the team this schedule represents.
	Team *firestore.DocumentRef `firestore:"team"`

	// RelativeLocations are the locations relative to `Team`. Values are:
	// * 2: this game takes place in the home stadium of `Team`.
	// * 1: this game takes place at a neutral site close to `Team`'s home.
	// * 0: this game takes place at a truly neutral site.
	// * -1: this game takes place at a neutral site close to the opponent's home.
	// * -2: this game takes place in the opponent's stadium.
	// These are in schedule order.
	RelativeLocations []int `firestore:"locales"`

	// Opponents are references to teams that `Team` is playing. They are in schedule order.
	Opponents []*firestore.DocumentRef `firestore:"opponents"`
}

// SeasonSchedule represents a document in firestore that contains team schedules
type SeasonSchedule struct {
	// Season is a reference to the season.
	Season *firestore.DocumentRef `firestore:"season"`

	// Timestamp is the time this document is written to the server.
	Timestamp time.Time `firestore:"timestamp,serverTimestamp"`
}
