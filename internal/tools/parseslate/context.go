package parseslate

import (
	"context"

	fs "cloud.google.com/go/firestore"
)

type Context struct {
	context.Context

	Force  bool
	DryRun bool

	FirestoreClient *fs.Client

	Season int
	Week   int
	Slate  string
}

func NewContext(ctx context.Context) *Context {
	return &Context{Context: ctx}
}
