package cfbdata

import (
	"context"
	"fmt"
	"io"

	fs "cloud.google.com/go/firestore"
)

type Collection interface {
	RefByID(uint64) (*fs.DocumentRef, bool)
	Ref(int) *fs.DocumentRef
	Datum(int) interface{}
	Len() int
}

// IterateWrite iterates the collection by `n` elements at a time and uses the given function to write to Firestore
func IterateWrite(ctx context.Context, client *fs.Client, c Collection, n int, f func(*fs.Transaction, *fs.DocumentRef, interface{}) error) <-chan error {
	out := make(chan error)

	go func() {
		defer close(out)
		for ll := 0; ll < c.Len(); ll += n {
			ul := ll + n
			if ul > c.Len() {
				ul = c.Len()
			}
			err := client.RunTransaction(ctx, func(ctx context.Context, tx *fs.Transaction) error {
				for i := ll; i < ul; i++ {
					ref := c.Ref(i)
					datum := c.Datum(i)
					if err := f(tx, ref, datum); err != nil {
						return err
					}
				}
				return nil
			})
			out <- err
		}
	}()

	return out
}

func DryRun(w io.Writer, c Collection) (int, error) {
	n := 0
	for i := 0; i < c.Len(); i++ {
		ref := c.Ref(i)
		val := c.Datum(i)
		nn, err := fmt.Fprintln(w, ref.Path)
		n += nn
		if err != nil {
			return n, err
		}
		nn, err = fmt.Fprintln(w, val)
		n += nn
		if err != nil {
			return n, err
		}
	}
	return n, nil
}
