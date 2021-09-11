package firestore

type Venue struct {
	Name        string    `firestore:"name"`
	Capacity    int       `firestore:"capacity"`
	Grass       bool      `firestore:"grass"`
	City        string    `firestore:"city"`
	State       string    `firestore:"state"`
	Zip         string    `firestore:"zip"`
	CountryCode string    `firestore:"country_code"`
	LatLon      []float64 `firestore:"latlon"`
	Year        int       `firestore:"year"`
	Dome        bool      `firestore:"dome"`
	Timezone    string    `firestore:"timezone"`
}
