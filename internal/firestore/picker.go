package firestore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
)

const PICKERS_COLLECTION = "pickers"

// Picker represents a picker in the datastore.
type Picker struct {
	// Name is the picker's full name.
	Name string `firestore:"name"`

	// LukeName is the picker's name as used in the slates and recap emails.
	LukeName string `firestore:"name_luke"`

	// Joined is a timestamp marking when the picker joined Pick 'Em.
	Joined time.Time `firestore:"joined"`
}

// UnmarshalText implements the TextUnmarshaler interface
func (p *Picker) UnmarshalText(text []byte) error {
	shortDateFormat := "2006/01/02"

	s := string(text)
	splits := strings.Split(s, ":")
	if len(splits) > 3 {
		return fmt.Errorf("too many fields for picker: expected <= 3, got %d", len(splits))
	}
	shortName := splits[0]
	fullName := shortName
	joinDate := time.Now()
	if len(splits) > 1 && splits[1] != "" {
		fullName = splits[1]
	}
	if len(splits) > 2 && splits[2] != "" {
		var err error
		joinDate, err = time.Parse(shortDateFormat, splits[2])
		if err != nil {
			return err
		}
	}

	p.LukeName = shortName
	p.Name = fullName
	p.Joined = joinDate

	return nil
}

type PickerNotFound string

func (e PickerNotFound) Error() string {
	return string(e)
}

// GetPickerByLukeName does what it purports to do.
func GetPickerByLukeName(ctx context.Context, client *firestore.Client, name string) (Picker, *firestore.DocumentRef, error) {
	var p Picker
	pickerCol := client.Collection(PICKERS_COLLECTION)
	q := pickerCol.Where("name_luke", "==", name)
	docs, err := q.Documents(ctx).GetAll()
	if err != nil {
		return p, nil, err
	}
	if len(docs) == 0 {
		return p, nil, PickerNotFound(fmt.Sprintf("no picker with LukeName \"%s\" defined", name))
	}
	if len(docs) > 1 {
		return p, nil, fmt.Errorf("more than one picker with LukeName \"%s\" defined", name)
	}
	if err = docs[0].DataTo(&p); err != nil {
		return p, nil, err
	}
	return p, docs[0].Ref, nil
}

// GetPickers gets all pickers in the datastore.
func GetPickers(ctx context.Context, client *firestore.Client) ([]Picker, []*firestore.DocumentRef, error) {
	snaps, err := client.Collection(PICKERS_COLLECTION).Documents(ctx).GetAll()
	if err != nil {
		return nil, nil, err
	}
	pickers := make([]Picker, len(snaps))
	refs := make([]*firestore.DocumentRef, len(snaps))
	for i, snap := range snaps {
		var p Picker
		err = snap.DataTo(&p)
		if err != nil {
			return nil, nil, err
		}
		pickers[i] = p
		refs[i] = snap.Ref
	}
	return pickers, refs, nil
}
