package firestore

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
)

const SEASONS_COLLECTION = "seasons"

// Season represents a Pick 'Em season.
type Season struct {
	// Year acts like a name for the season. It is the year that the season begins.
	Year int `firestore:"year"`

	// StartTime is a nominal time when the season begins. It's typically kickoff of the first game of the season.
	StartTime time.Time `firestore:"start_time"`

	// Pickers is a map of LukeNames to references to Picker documents in Firestore. These are the pickers who are registered to play this season.
	Pickers map[string]*firestore.DocumentRef `firestore:"pickers"`

	// StreakTeams is an array of teams available for the BTS competition.
	StreakTeams []*firestore.DocumentRef `firestore:"streak_teams"`

	// StreakPickTypes is an array of pick types available for the BTS competition.
	// The indices of the array represent the following:
	//   0: the number of bye weeks
	//   1: the number of single-team pick weeks
	//   2: the number of double-down pick weeks
	//   ...
	StreakPickTypes []int `firestore:"streak_pick_types"`
}

// GetSeason gets the season defined by `year`. If `year<0`, the most recent season (by `start_time`) is returned.
func GetSeason(ctx context.Context, client *firestore.Client, year int) (Season, *firestore.DocumentRef, error) {
	var s Season
	seasonCol := client.Collection(SEASONS_COLLECTION)
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

// GetSeasons gets all seasons
func GetSeasons(ctx context.Context, client *firestore.Client) ([]Season, []*firestore.DocumentRef, error) {
	snaps, err := client.Collection(SEASONS_COLLECTION).Documents(ctx).GetAll()
	if err != nil {
		return nil, nil, err
	}
	seasons := make([]Season, len(snaps))
	refs := make([]*firestore.DocumentRef, len(snaps))
	for i, snap := range snaps {
		var s Season
		err = snap.DataTo(&s)
		if err != nil {
			return nil, nil, err
		}
		seasons[i] = s
		refs[i] = snap.Ref
	}
	return seasons, refs, nil
}
