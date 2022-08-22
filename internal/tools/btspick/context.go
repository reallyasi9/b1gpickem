package btspick

import (
	"context"

	fs "cloud.google.com/go/firestore"
)

type Context struct {
	context.Context
	FirestoreClient *fs.Client

	Force  bool
	DryRun bool

	Season int
	Week   int
	Picks  map[string]string
}

func NewContext(ctx context.Context) *Context {
	return &Context{Context: ctx}
}
