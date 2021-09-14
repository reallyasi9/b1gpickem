package firestore

import (
	"strings"

	"cloud.google.com/go/firestore"
)

type Week struct {
	// Season is a back-reference to the season document in Firestore.
	Season *firestore.DocumentRef `firestore:"season"`

	// Number is the ordinal week number, starting with 0 for the first week of the season.
	Number int `firestore:"number"`

	// Picks are references to Firestore documents of the picker's picks for this week, indexed by LukeName.
	Picks map[string]*firestore.DocumentRef `firestore:"picks"`
}

func (w Week) String() string {
	var sb strings.Builder
	sb.WriteString("Week\n")
	sb.WriteString(treeRef("Season", 0, false, w.Season))
	sb.WriteRune('\n')
	sb.WriteString(treeInt("Number", 0, false, w.Number))
	sb.WriteRune('\n')
	sb.WriteString(treeStringRefMap("Picks", 0, true, w.Picks))
	return sb.String()
}
