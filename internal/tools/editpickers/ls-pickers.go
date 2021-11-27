package editpickers

import (
	"fmt"

	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

func LsPickers(ctx *Context) error {

	pickers, refs, err := firestore.GetPickers(ctx.Context, ctx.FirestoreClient)
	if err != nil {
		return fmt.Errorf("LsPickers: error getting pickers: %w", err)
	}
	for i, picker := range pickers {
		fmt.Printf("%s: %s\n", refs[i].Path, picker)
	}
	return nil
}
