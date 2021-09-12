package main

import (
	"strings"
	"time"

	"github.com/reallyasi9/b1gpickem/firestore"
)

type Game struct {
	ID           uint64    `json:"id"`
	Week         int       `json:"week"`
	StartTime    time.Time `json:"start_date"`
	StartTimeTBD bool      `json:"start_time_tbd"`
	NeutralSite  bool      `json:"neutral_site"`
	VenueID      uint64    `json:"venue_id"`
	HomeID       uint64    `json:"home_id"`
	AwayID       uint64    `json:"away_id"`
	HomePoints   *int      `json:"home_points"`
	AwayPoints   *int      `json:"away_points"`
}

// ToFirestore does not link the teams--that has to be done with an external lookup.
// The same goes for the venue.
func (g Game) ToFirestore() (uint64, firestore.Game) {
	fg := firestore.Game{
		NeutralSite:  g.NeutralSite,
		StartTime:    g.StartTime,
		StartTimeTBD: g.StartTimeTBD,
		HomePoints:   g.HomePoints,
		AwayPoints:   g.AwayPoints,
	}
	return g.ID, fg
}

type Team struct {
	ID           uint64   `json:"id"`
	School       string   `json:"school"`
	Mascot       *string  `json:"mascot"`
	Abbreviation *string  `json:"abbreviation"`
	AltName1     *string  `json:"alt_name1"`
	AltName2     *string  `json:"alt_name2"`
	AltName3     *string  `json:"alt_name3"`
	Color        *string  `json:"color"`
	AltColor     *string  `json:"alt_color"`
	Logos        []string `json:"logos"`
	Location     struct {
		VenueID *uint64 `json:"venue_id"`
	}
}

func appendNonNilStrings(s []string, vals ...*string) []string {
	for _, v := range vals {
		if v == nil {
			continue
		}
		s = append(s, *v)
	}
	return s
}

func coalesceString(s *string, replacement string) string {
	if s == nil || *s == "" {
		return replacement
	}
	return *s
}

func distinctStrings(ss []string) []string {
	// defensive copy
	result := make([]string, len(ss))
	copy(result, ss)
	distinct := make(map[string]struct{})
	for i, s := range result {
		if _, ok := distinct[s]; ok {
			result = append(result[:i], result[i+1:]...)
			continue
		}
		distinct[s] = struct{}{}
	}
	return result
}

func abbreviate(s string) string {
	if len(s) < 5 {
		return strings.ToUpper(s)
	}
	splits := strings.Split(s, " ")
	if len(splits) == 1 {
		return strings.ToUpper(s[:4])
	}
	var sb strings.Builder
	for _, split := range splits {
		sb.WriteString(strings.ToUpper(split[:1]))
	}
	return sb.String()
}

// ToFirestore does not link the Venue--that has to be done with an external lookup.
func (t Team) ToFirestore() (uint64, firestore.Team) {
	otherNames := make([]string, 0)
	otherNames = appendNonNilStrings(otherNames, t.AltName1, t.AltName2, t.AltName3)
	otherNames = distinctStrings(otherNames)
	colors := make([]string, 0)
	colors = appendNonNilStrings(colors, t.Color, t.AltColor)

	abbr := coalesceString(t.Abbreviation, strings.ToUpper(t.School))
	ft := firestore.Team{
		Abbreviation: coalesceString(t.Abbreviation, strings.ToUpper(t.School)),
		ShortNames:   []string{abbr},
		OtherNames:   otherNames,
		School:       t.School,
		Mascot:       coalesceString(t.Mascot, "Football Team"),
		Colors:       colors,
	}
	return t.ID, ft
}

type Venue struct {
	ID          uint64 `json:"id"`
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

func (v Venue) ToFirestore() (uint64, firestore.Venue) {
	latlon := make([]float64, 0)
	if v.Location.X != 0 || v.Location.Y != 0 {
		// The CFBData calls latitude "X" and longitude "Y" for whatever reason
		latlon = []float64{v.Location.X, v.Location.Y}
	}
	fv := firestore.Venue{
		Name:        v.Name,
		Capacity:    v.Capacity,
		Grass:       v.Grass,
		City:        v.City,
		State:       v.City,
		Zip:         v.Zip,
		CountryCode: v.CountryCode,
		LatLon:      latlon,
		Year:        v.Year,
		Dome:        v.Dome,
		Timezone:    v.Timezone,
	}
	return v.ID, fv
}

type Week struct {
	Season         string    `json:"season"`
	Number         int       `json:"week"`
	FirstGameStart time.Time `json:"firstGameStart"`
	LastGameStart  time.Time `json:"lastGameStart"`
}

func (w Week) ToFirestore() firestore.Week {
	return firestore.Week{
		Number: w.Number,
	}
}
