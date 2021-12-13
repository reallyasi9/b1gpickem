package sa

import (
	"context"

	fs "cloud.google.com/go/firestore"
)

type Context struct {
	context.Context
	FirestoreClient *fs.Client

	Force  bool
	DryRun bool

	Season      int
	Week        int
	Streakers   []string
	All         bool
	Seed        int64
	Workers     int
	Iterations  int
	WanderLimit int
	C           float64
	E           float64
}

func NewContext(ctx context.Context) *Context {
	return &Context{Context: ctx}
}
