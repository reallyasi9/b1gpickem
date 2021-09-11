package firestore

import (
	"time"

	"cloud.google.com/go/firestore"
)

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
