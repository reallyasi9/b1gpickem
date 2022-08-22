package cfbdata

import (
	"context"
	"fmt"
	"io"

	fs "cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Collection interface {
	RefByID(int64) (*fs.DocumentRef, bool)
	Ref(int) *fs.DocumentRef
	ID(int) int64
	Datum(int) interface{}
	FprintDatum(io.Writer, int) (int, error)
	Len() int
}

type TransactionIterator struct {
	UpdateFcn func(*fs.Transaction, *fs.DocumentRef, interface{}) error
}

// IterateTransaction iterates the collection by `n` elements at a time and uses the given function to write to Firestore
func (ti TransactionIterator) IterateTransaction(ctx context.Context, client *fs.Client, c Collection, n int) <-chan error {
	out := make(chan error)

	go func() {
		defer close(out)
		for ll := 0; ll < c.Len(); ll += n {
			ul := ll + n
			if ul > c.Len() {
				ul = c.Len()
			}
			err := client.RunTransaction(ctx, func(ctx context.Context, tx *fs.Transaction) error {
				// determine who is created and who is updated
				creates := make(map[*fs.DocumentRef]interface{})
				updates := make(map[*fs.DocumentRef]interface{})
				for i := ll; i < ul; i++ {
					ref := c.Ref(i)
					datum := c.Datum(i)
					if _, err := ref.Get(ctx); status.Code(err) == codes.NotFound {
						creates[ref] = datum
					} else {
						updates[ref] = datum
					}
				}
				for ref, datum := range creates {
					err := tx.Create(ref, datum)
					if err != nil {
						return err
					}
				}
				for ref, datum := range updates {
					err := ti.UpdateFcn(tx, ref, datum)
					if err != nil {
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
		nn := 0
		var err error
		if ref == nil {
			nn, err = fmt.Println(w, "(nil ref)")
		} else {
			nn, err = fmt.Fprintln(w, ref.Path)
		}
		n += nn
		if err != nil {
			return n, err
		}
		nn, err = c.FprintDatum(w, i)
		n += nn
		if err != nil {
			return n, err
		}
		nn, _ = fmt.Fprintln(w)
		n += nn
	}
	return n, nil
}
