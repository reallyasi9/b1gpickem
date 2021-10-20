package firestore

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
)

// Picker represents a picker in the datastore.
type Picker struct {
	// Name is the picker's full name.
	Name string `firestore:"name"`

	// LukeName is the picker's name as used in the slates and recap emails.
	LukeName string `firestore:"name_luke"`

	// Joined is a timestamp marking when the picker joined Pick 'Em.
	Joined time.Time `firestore:"joined"`
}

// GetPickerByLukeName does what it purports to do.
func GetPickerByLukeName(ctx context.Context, client *firestore.Client, name string) (Picker, *firestore.DocumentRef, error) {
	var p Picker
	pickerCol := client.Collection("pickers")
	q := pickerCol.Where("name_luke", "==", name)
	docs, err := q.Documents(ctx).GetAll()
	if err != nil {
		return p, nil, err
	}
	if len(docs) == 0 {
		return p, nil, fmt.Errorf("no picker with LukeName \"%s\" defined", name)
	}
	if len(docs) > 1 {
		return p, nil, fmt.Errorf("more than one picker with LukeName \"%s\" defined", name)
	}
	if err = docs[0].DataTo(&p); err != nil {
		return p, nil, err
	}
	return p, docs[0].Ref, nil
}
