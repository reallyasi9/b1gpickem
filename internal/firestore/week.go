package firestore

import (
	"context"
	"fmt"
	"strings"
	"time"

	firestore "cloud.google.com/go/firestore"
)

// WEEKS_COLLECTION is the path to the weeks collection in Firestore. It is a child of a season.
const WEEKS_COLLECTION = "weeks"

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

type NoWeekError int

func (e NoWeekError) Error() string {
	return fmt.Sprintf("no week %d exists", e)
}

// GetWeek returns the week object and document ref pointer matching the given season document ref and week number.
// If `week<0`, the week is calculated based on today's date and the week's `first_game_start` field.
func GetWeek(ctx context.Context, season *firestore.DocumentRef, week int) (Week, *firestore.DocumentRef, error) {
	var w Week
	weekCol := season.Collection(WEEKS_COLLECTION)
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
		return w, nil, NoWeekError(week)
	}
	if err = docs[0].DataTo(&w); err != nil {
		return w, nil, err
	}
	return w, docs[0].Ref, nil
}

// GetFirstWeek returns the week object and document ref pointer with the earliest value of `first_game_start` in the season.
func GetFirstWeek(ctx context.Context, season *firestore.DocumentRef) (Week, *firestore.DocumentRef, error) {
	var w Week
	weekCol := season.Collection(WEEKS_COLLECTION)
	q := weekCol.OrderBy("first_game_start", firestore.Asc).Limit(1)
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
