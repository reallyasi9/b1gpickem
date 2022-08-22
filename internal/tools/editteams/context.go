package editteams

import (
	"context"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

type Context struct {
	context.Context

	Force  bool
	DryRun bool

	FirestoreClient *fs.Client

	ID     string
	Team   firestore.Team
	Season int
	Append bool
}

func NewContext(ctx context.Context) *Context {
	return &Context{Context: ctx}
}
