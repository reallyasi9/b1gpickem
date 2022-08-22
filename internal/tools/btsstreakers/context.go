package btsstreakers

import (
	"context"

	fs "cloud.google.com/go/firestore"
)

type Context struct {
	context.Context
	FirestoreClient *fs.Client

	Force  bool
	DryRun bool

	Season        int
	StreakerNames []string
	Append        bool
}

func NewContext(ctx context.Context) *Context {
	return &Context{Context: ctx}
}
