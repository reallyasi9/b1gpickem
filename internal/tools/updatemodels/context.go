package updatemodels

import (
	"context"

	fs "cloud.google.com/go/firestore"
)

type Context struct {
	context.Context

	DryRun          bool
	Force           bool
	FirestoreClient *fs.Client
	Season          int
	Week            int
	ModelNames      []string
	SystemNames     []string
}

func NewContext(ctx context.Context) *Context {
	return &Context{Context: ctx}
}
