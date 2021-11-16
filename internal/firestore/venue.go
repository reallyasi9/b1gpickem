package firestore

import "strings"

const VENUES_COLLECTION = "venues"

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

func (v Venue) String() string {
	var sb strings.Builder
	sb.WriteString("Venue\n")
	ss := make([]string, 0)
	ss = append(ss, treeString("Name", 0, false, v.Name))
	ss = append(ss, treeInt("Capacity", 0, false, v.Capacity))
	ss = append(ss, treeBool("Grass", 0, false, v.Grass))
	ss = append(ss, treeString("City", 0, false, v.City))
	ss = append(ss, treeString("State", 0, false, v.State))
	ss = append(ss, treeString("Zip", 0, false, v.Zip))
	ss = append(ss, treeString("CountryCode", 0, false, v.CountryCode))
	ss = append(ss, treeFloat64Slice("LatLon", 0, false, v.LatLon))
	ss = append(ss, treeInt("Year", 0, false, v.Year))
	ss = append(ss, treeBool("Dome", 0, false, v.Dome))
	ss = append(ss, treeString("Timezone", 0, true, v.Timezone))
	sb.WriteString(strings.Join(ss, "\n"))
	return sb.String()
}
