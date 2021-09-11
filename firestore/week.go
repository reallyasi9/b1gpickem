package firestore

import "cloud.google.com/go/firestore"

type Week struct {
	// Season is a back-reference to the season document in Firestore.
	Season *firestore.DocumentRef `firestore:"season"`

	// Number is the ordinal week number, starting with 0 for the first week of the season.
	Number int `firestore:"number"`

	// Games are references to Firestore documents of the games played (and picked) this week.
	Games []*firestore.DocumentRef `firestore:"games"`

	// Picks are references to Firestore documents of the picker's picks for this week, indexed by LukeName.
	Picks map[string]*firestore.DocumentRef `firestore:"picks"`
}
