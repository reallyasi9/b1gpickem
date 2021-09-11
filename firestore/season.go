package firestore

import (
	"time"

	"cloud.google.com/go/firestore"
)

// Season represents a Pick 'Em season.
type Season struct {
	// Year acts like a name for the season. It is the year that the season begins.
	Year int `firestore:"year"`

	// StartTime is a nominal time when the season begins. It's typically kickoff of the first game of the season.
	StartTime time.Time `firestore:"start_time"`

	// Pickers is a map of LukeNames to references to Picker documents in Firestore. These are the pickers who are registered to play this season.
	Pickers map[string]*firestore.DocumentRef `firestore:"pickers"`

	// TeamsByOtherName is a map of OtherNames to references to Team documents in Firestore.
	TeamsByOtherName map[string]*firestore.DocumentRef `firestore:"teams_other"`

	// TeamsByLukeName is a map of LukeNames to references to Team documents in the Firestore.
	TeamsByLukeName map[string]*firestore.DocumentRef `firestore:"teams_luke"`

	// TeamsByName4 is a map of Name4 to references to Team documents in the Firestore.
	TeamsByName4 map[string]*firestore.DocumentRef `firestore:"teams"`

	// Weeks are references to Week documents in the Firestore.
	Weeks []*firestore.DocumentRef
}
