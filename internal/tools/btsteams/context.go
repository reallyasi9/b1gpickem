package btsteams

import (
	"context"

	fs "cloud.google.com/go/firestore"
)

// Context represents a set of options passed to the BTS teams commands.
type Context struct {
	context.Context
	FirestoreClient *fs.Client

	Force  bool
	DryRun bool

	Season            int
	TeamNames         []string
	TeamPreseasonWins []float64
	Append            bool
}

// NewContext creates and returns a btsteams Context from a base context object.
func NewContext(ctx context.Context) *Context {
	return &Context{Context: ctx}
}
