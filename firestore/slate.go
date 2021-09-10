package firestore

import (
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
)

// String implements the Stringer interface.
func (g Game) String() string {
	if g.Superdog {
		return fmt.Sprintf("%s over %s (%d points)", g.Teams[1-g.FavoredIndex].ID, g.Teams[g.FavoredIndex].ID, g.Value)
	}

	var sb strings.Builder
	if g.GOTW {
		sb.WriteString("** ")
	}

	if g.Ranks[0] > 0 {
		sb.WriteString(fmt.Sprintf("#%d ", g.Ranks[0]))
	}

	sb.WriteString(g.Teams[0].ID)

	if g.NeutralSite {
		sb.WriteString(" n ")
	} else if g.HomeIndex == 1 {
		sb.WriteString(" @ ")
	} else {
		sb.WriteString(" v ")
	}

	if g.Ranks[1] > 0 {
		sb.WriteString(fmt.Sprintf("#%d ", g.Ranks[1]))
	}

	sb.WriteString(g.Teams[1].ID)

	if g.GOTW {
		sb.WriteString(" **")
	}

	if g.NoisySpread != 0 {
		sb.WriteString(fmt.Sprintf(", %s by ≥ %d", g.Teams[g.FavoredIndex].ID, g.NoisySpread))
	}

	return sb.String()
}

// Slate represents how a slate is stored in Firestore.
type Slate struct {

	// Bucket is the name of the GC Storage bucket containing the original slate.
	Bucket string `firestore:"bucket_name"`

	// Created is the creation timestamp of the slate.
	Created time.Time `firestore:"created"`

	// FileName is the name of the slate file in `Bucket`.
	FileName string `firestore:"file"`

	// Season is a reference to the season document this slate refers to.
	Season *firestore.DocumentRef `firestore:"season"`

	// Week is a reference to the week in the season this slate refers to.
	Week *firestore.DocumentRef `firestore:"week"`

	// Games are references to the games of the slate, indexed by the row in the slate spreadsheet where the game was originally placed.
	Games map[int]*firestore.DocumentRef `firestore:"games"`
}
