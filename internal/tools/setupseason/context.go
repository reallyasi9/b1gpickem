package setupseason

import (
	"context"

	fs "cloud.google.com/go/firestore"
)

type Context struct {
	context.Context

	DryRun          bool
	Force           bool
	FirestoreClient *fs.Client
	ApiKey          string
	Season          int
	Weeks           []int
}

func NewContext(ctx context.Context) *Context {
	return &Context{Context: ctx}
}
