package editpickers

import (
	"context"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

type Context struct {
	context.Context

	DryRun          bool
	Force           bool
	FirestoreClient *fs.Client
	Pickers         []firestore.Picker
	ID              string
	Season          int
	KeepSeasons     bool
}

func NewContext(ctx context.Context) *Context {
	return &Context{Context: ctx}
}
