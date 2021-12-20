package enumerate

import (
	"context"

	fs "cloud.google.com/go/firestore"
)

type Context struct {
	context.Context
	FirestoreClient *fs.Client

	Season     int
	NoProgress bool
}

func NewContext(ctx context.Context) *Context {
	return &Context{Context: ctx}
}
