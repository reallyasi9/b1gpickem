package main

import (
	"strings"
	"time"

	"github.com/reallyasi9/b1gpickem/firestore"
)

type Game struct {
	ID           uint64    `json:"id"`
	StartDate    time.Time `json:"start_date"`
	StartTimeTBD bool      `json:"start_time_tbd"`
	NeutralSite  bool      `json:"neutral_site"`
	VenueID      uint64    `json:"venue_id"`
	HomeID       uint64    `json:"home_team"`
	AwayID       uint64    `json:"away_team"`
	HomePoints   *int      `json:"home_points"`
	AwayPoints   *int      `json:"away_points"`
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

// ToFirestore does not link the Venue--that has to be done with an external lookup.
func (t Team) ToFirestore() (uint64, firestore.Team) {
	otherNames := make([]string, 0)
	otherNames = appendNonNilStrings(otherNames, t.Mascot, t.AltName1, t.AltName2, t.AltName3)
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

type Venues struct {
	ID          uint64 `json:"id"`
	Name        string `json:"string"`
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
