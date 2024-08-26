package pyp

import (
	"context"

	fs "cloud.google.com/go/firestore"
)

type Context struct {
	context.Context
	FirestoreClient *fs.Client

	// Force  bool
	// DryRun bool

	Season     int
	Seed       int64
	Workers    int
	Iterations int
}

func NewContext(ctx context.Context) *Context {
	return &Context{Context: ctx}
}
