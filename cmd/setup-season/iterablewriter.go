package main

import (
	"context"
	"io"

	fs "cloud.google.com/go/firestore"
)

// IterableWriter is an interface that gives collections of objects a way to write to a firestore transaction.
type IterableWriter interface {
	// IterableCreate forms a series of transactions to create objects in firestore in batches of less than 500 elements (the limit of a firestore transaction).
	IterableCreate(context.Context, *fs.Client, *fs.CollectionRef) error
	// IterableSet does the same as `IterableCreate`, but uses the Set operation to update elements rather than erroring out if the element already exists.
	IterableSet(context.Context, *fs.Client, *fs.CollectionRef) error
	// DryRun prints the elements that would be created to the given writable and returns the number of bytes written.
	DryRun(io.Writer, *fs.CollectionRef) (int, error)
}
