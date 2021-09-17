package firestore

import (
	"strings"
	"time"
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
