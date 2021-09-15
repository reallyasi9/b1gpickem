package firestore

import (
	"strings"

	"cloud.google.com/go/firestore"
)

type Week struct {
	// Picks are references to Firestore documents of the picker's picks for this week, indexed by LukeName.
	Picks map[string]*firestore.DocumentRef `firestore:"picks"`
}

func (w Week) String() string {
	var sb strings.Builder
	sb.WriteString("Week\n")
	sb.WriteString(treeStringRefMap("Picks", 0, true, w.Picks))
	return sb.String()
}
