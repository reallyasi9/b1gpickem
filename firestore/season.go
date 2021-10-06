package firestore

import (
	"context"
	"fmt"
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
}

// GetSeason gets the season defined by `year`. If `year<0`, the most recent season (by `start_time`) is returned.
func GetSeason(ctx context.Context, client *firestore.Client, year int) (Season, *firestore.DocumentRef, error) {
	var s Season
	seasonCol := client.Collection("seasons")
	var q firestore.Query
	if year < 0 {
		q = seasonCol.OrderBy("start_time", firestore.Desc).Limit(1)
	} else {
		q = seasonCol.Where("year", "==", year).Limit(1)
	}
	docs, err := q.Documents(ctx).GetAll()
	if err != nil {
		return s, nil, err
	}
	if len(docs) == 0 {
		return s, nil, fmt.Errorf("no seasons defined")
	}
	if err = docs[0].DataTo(&s); err != nil {
		return s, nil, err
	}
	return s, docs[0].Ref, nil
}
