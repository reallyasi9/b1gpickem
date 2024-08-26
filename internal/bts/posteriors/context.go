package posteriors

import (
	"context"

	fs "cloud.google.com/go/firestore"
)

type Context struct {
	context.Context
	FirestoreClient *fs.Client

	Force  bool
	DryRun bool

	Season       int
	Week         int
	Teams        []string
	Seed         int64
	Iterations   int
	Championship bool
}

func NewContext(ctx context.Context) *Context {
	return &Context{Context: ctx}
}
