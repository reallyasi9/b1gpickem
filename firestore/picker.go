package firestore

import "time"

// Picker represents a picker in the datastore.
type Picker struct {
	// Name is the picker's full name.
	Name string `firestore:"name"`

	// LukeName is the picker's name as used in the slates and recap emails.
	LukeName string `firestore:"name_luke"`

	// Joined is a timestamp marking when the picker joined Pick 'Em.
	Joined time.Time `firestore:"joined"`
}
