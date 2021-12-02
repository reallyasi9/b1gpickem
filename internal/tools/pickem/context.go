package pickem

import (
	"context"

	fs "cloud.google.com/go/firestore"
)

type Context struct {
	context.Context

	Force  bool
	DryRun bool

	FirestoreClient *fs.Client

	Season   int
	Week     int
	Picker   string
	Picks    []string
	SuperDog string

	Output string
}

func NewContext(ctx context.Context) *Context {
	return &Context{Context: ctx}
}
