package firestore

import (
	"context"
	"fmt"
	"strings"
	"time"

	firestore "cloud.google.com/go/firestore"
)

type Week struct {
	// Number is the week number.
	Number int `firestore:"number"`

	// FirstGameStart is the start time of the first game of the week.
	FirstGameStart time.Time `firestore:"first_game_start"`
}

func (w Week) String() string {
	var sb strings.Builder
	sb.WriteString("Week\n")
	sb.WriteString(treeInt("Number", 0, false, w.Number))
	sb.WriteRune('\n')
	sb.WriteString(treeString("FirstGameStart", 0, true, w.FirstGameStart.GoString()))
	return sb.String()
}

// GetWeek returns the week object and document ref pointer matching the given season document ref and week number.
// If `week<0`, the week is calculated based on today's date and the week's `first_game_start` field.
func GetWeek(ctx context.Context, client *firestore.Client, season *firestore.DocumentRef, week int) (Week, *firestore.DocumentRef, error) {
	var w Week
	weekCol := season.Collection("weeks")
	var q firestore.Query
	if week < 0 {
		now := time.Now()
		q = weekCol.Where("first_game_start", ">=", now).OrderBy("first_game_start", firestore.Asc).Limit(1)
	} else {
		q = weekCol.Where("number", "==", week)
	}
	docs, err := q.Documents(ctx).GetAll()
	if err != nil {
		return w, nil, err
	}
	if len(docs) == 0 {
		return w, nil, fmt.Errorf("no weeks defined for season %s", season.ID)
	}
	if err = docs[0].DataTo(&w); err != nil {
		return w, nil, err
	}
	return w, docs[0].Ref, nil
}
