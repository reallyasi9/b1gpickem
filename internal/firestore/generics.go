package firestore

import (
	fs "cloud.google.com/go/firestore"
	"context"
	"fmt"
)

// GetAll gets all the documents from Firestore so long as they are all of the same type.
func GetAll[T Team | Picker | Game](ctx context.Context, client *fs.Client, refs []*fs.DocumentRef) ([]T, error) {
	out := make([]T, len(refs))
	
	snaps, err := client.GetAll(ctx, refs)
	if err != nil {
		return nil, fmt.Errorf("GetAll: unable to get documents from client: %w", err)
	}
	for i, snap := range snaps {
		var val T
		err := snap.DataTo(&val)
		if err != nil {
			return nil, fmt.Errorf("GetAll: unable to create type %T from doc %v: %w", val, snap, err)
		}
		out[i] = val
	}

	return out, nil
}