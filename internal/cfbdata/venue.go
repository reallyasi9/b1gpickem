package cfbdata

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

type Venue struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Capacity    int    `json:"capacity"`
	Grass       bool   `json:"grass"`
	City        string `json:"city"`
	State       string `json:"state"`
	Zip         string `json:"zip"`
	CountryCode string `json:"country_code"`
	Location    struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	} `json:"location"`
	Year     int    `json:"year"`
	Dome     bool   `json:"dome"`
	Timezone string `json:"timezone"`
}

func (v Venue) toFirestore() firestore.Venue {
	latlon := make([]float64, 0)
	if v.Location.X != 0 || v.Location.Y != 0 {
		// The CFBData calls latitude "X" and longitude "Y" for whatever reason
		latlon = []float64{v.Location.X, v.Location.Y}
	}
	return firestore.Venue{
		Name:        v.Name,
		Capacity:    v.Capacity,
		Grass:       v.Grass,
		City:        v.City,
		State:       v.State,
		Zip:         v.Zip,
		CountryCode: v.CountryCode,
		LatLon:      latlon,
		Year:        v.Year,
		Dome:        v.Dome,
		Timezone:    v.Timezone,
	}
}

type VenueCollection struct {
	venues   []Venue
	fsVenues []firestore.Venue
	refs     []*fs.DocumentRef
	ids      map[int64]int
}

func GetVenues(client *http.Client, key string) (VenueCollection, error) {
	body, err := DoRequest(client, key, "https://api.collegefootballdata.com/venues")
	if err != nil {
		return VenueCollection{}, fmt.Errorf("failed to do venues request: %v", err)
	}

	var venues []Venue
	err = json.Unmarshal(body, &venues)
	if err != nil {
		return VenueCollection{}, fmt.Errorf("failed to unmarshal venues response body: %v", err)
	}

	f := make([]firestore.Venue, len(venues))
	refs := make([]*fs.DocumentRef, len(venues))
	ids := make(map[int64]int)
	for i, v := range venues {
		f[i] = v.toFirestore()
		ids[v.ID] = i
	}

	return VenueCollection{venues: venues, fsVenues: f, refs: refs, ids: ids}, nil
}

func (vc VenueCollection) Len() int {
	return len(vc.venues)
}

func (vc VenueCollection) Ref(i int) *fs.DocumentRef {
	return vc.refs[i]
}

func (vc VenueCollection) ID(i int) int64 {
	return vc.venues[i].ID
}

func (vc VenueCollection) Datum(i int) interface{} {
	return vc.fsVenues[i]
}

func (vc VenueCollection) RefByID(id int64) (*fs.DocumentRef, bool) {
	if i, ok := vc.ids[id]; ok {
		return vc.refs[i], true
	}
	return nil, false
}

func (vc VenueCollection) LinkRefs(col *fs.CollectionRef) error {
	for i, venue := range vc.venues {
		fsVenue := venue.toFirestore()
		vc.fsVenues[i] = fsVenue
		vc.refs[i] = col.Doc(fmt.Sprintf("%d", venue.ID))
	}
	return nil
}

func (vc VenueCollection) FprintDatum(w io.Writer, i int) (int, error) {
	return fmt.Fprint(w, vc.fsVenues[i])
}
