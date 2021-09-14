package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	fs "cloud.google.com/go/firestore"
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

// toFirestore does not link the teams--that has to be done with an external lookup.
// The same goes for the venue.
func (g Game) toFirestore() firestore.Game {
	fg := firestore.Game{
		NeutralSite:  g.NeutralSite,
		StartTime:    g.StartTime,
		StartTimeTBD: g.StartTimeTBD,
		HomePoints:   g.HomePoints,
		AwayPoints:   g.AwayPoints,
	}
	return fg
}

// GameCollection is a collection of games meeting the IterableWriter interface.
type GameCollection struct {
	games   map[uint64]Game
	fsGames map[uint64]firestore.Game
}

// Len gets the length of the collection
func (gc GameCollection) Len() int {
	return len(gc.games)
}

// ByWeek splits the GameCollection into multiple GameCollections indexed by week.
func (gc GameCollection) GetWeek(week int) GameCollection {
	gw := make(map[uint64]Game)
	fw := make(map[uint64]firestore.Game)
	for id, g := range gc.games {
		if g.Week == week {
			gw[id] = g
			if gc.fsGames == nil {
				continue
			}
			if f, ok := gc.fsGames[id]; ok {
				fw[id] = f
			}
		}
	}
	return GameCollection{games: gw, fsGames: fw}
}

func (gc GameCollection) LinkRefs(teamMap, venueMap map[uint64]*fs.DocumentRef, col *fs.CollectionRef) (map[uint64]*fs.DocumentRef, error) {
	gc.fsGames = make(map[uint64]firestore.Game)
	refs := make(map[uint64]*fs.DocumentRef)
	for id, g := range gc.games {
		fsg := g.toFirestore()
		homeTeamID := g.HomeID
		awayTeamID := g.AwayID
		venueID := g.VenueID

		var ok bool
		if fsg.HomeTeam, ok = teamMap[homeTeamID]; !ok {
			return nil, fmt.Errorf("home team %d in game %d not found in reference map", homeTeamID, id)
		}
		if fsg.AwayTeam, ok = teamMap[awayTeamID]; !ok {
			return nil, fmt.Errorf("away team %d in game %d not found in reference map", awayTeamID, id)
		}
		if fsg.Venue, ok = venueMap[venueID]; !ok {
			return nil, fmt.Errorf("venue %d for game %d not found in reference map", venueID, id)
		}

		gc.fsGames[id] = fsg
		refs[id] = col.Doc(fmt.Sprintf("%d", id))
	}
	return refs, nil
}

func (gc GameCollection) IterableCreate(ctx context.Context, client *fs.Client, col *fs.CollectionRef) error {
	games := make([]*firestore.Game, 0, len(gc.fsGames))
	ids := make([]uint64, 0, len(gc.fsGames))
	for id, g := range gc.fsGames {
		games = append(games, &g)
		ids = append(ids, id)
	}
	for ll := 0; ll < len(games); ll += 500 {
		ul := ll + 500
		if ul > len(games) {
			ul = len(games)
		}
		gr := games[ll:ul]
		ir := ids[ll:ul]
		err := client.RunTransaction(ctx, func(ctx context.Context, tx *fs.Transaction) error {
			for i, data := range gr {
				id := ir[i]
				ref := col.Doc(fmt.Sprintf("%d", id))
				if err := tx.Create(ref, data); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (gc GameCollection) IterableSet(ctx context.Context, client *fs.Client, col *fs.CollectionRef) error {
	games := make([]*firestore.Game, 0, len(gc.fsGames))
	ids := make([]uint64, 0, len(gc.fsGames))
	for id, g := range gc.fsGames {
		games = append(games, &g)
		ids = append(ids, id)
	}
	for ll := 0; ll < len(games); ll += 500 {
		ul := ll + 500
		if ul > len(games) {
			ul = len(games)
		}
		gr := games[ll:ul]
		ir := ids[ll:ul]
		err := client.RunTransaction(ctx, func(ctx context.Context, tx *fs.Transaction) error {
			for i, data := range gr {
				id := ir[i]
				ref := col.Doc(fmt.Sprintf("%d", id))
				if err := tx.Create(ref, data); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (gc GameCollection) DryRun(w io.Writer, col *fs.CollectionRef) (int, error) {
	n := 0
	for id, g := range gc.fsGames {
		ref := col.Doc(fmt.Sprintf("%d", id))
		nn, err := fmt.Fprintln(w, ref.Path)
		n += nn
		if err != nil {
			return n, err
		}
		nn, err = fmt.Fprintln(w, g)
		n += nn
		if err != nil {
			return n, err
		}
	}
	return n, nil
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
	venues   map[uint64]Venue
	fsVenues map[uint64]firestore.Venue
}

func (vc VenueCollection) Len() int {
	return len(vc.venues)
}

func (vc VenueCollection) LinkRefs(col *fs.CollectionRef) (map[uint64]*fs.DocumentRef, error) {
	refs := make(map[uint64]*fs.DocumentRef)
	for id, venue := range vc.venues {
		fsVenue := venue.toFirestore()
		vc.fsVenues[id] = fsVenue
		refs[id] = col.Doc(fmt.Sprintf("%d", id))
	}
	return refs, nil
}

func (vc VenueCollection) IterableCreate(ctx context.Context, client *fs.Client, col *fs.CollectionRef) error {
	vens := make([]*firestore.Venue, 0, len(vc.fsVenues))
	ids := make([]uint64, 0, len(vc.fsVenues))
	for id, v := range vc.fsVenues {
		vens = append(vens, &v)
		ids = append(ids, id)
	}
	for ll := 0; ll < len(vens); ll += 500 {
		ul := ll + 500
		if ul > len(vens) {
			ul = len(vens)
		}
		wr := vens[ll:ul]
		ir := ids[ll:ul]
		err := client.RunTransaction(ctx, func(ctx context.Context, tx *fs.Transaction) error {
			for i, data := range wr {
				id := ir[i]
				ref := col.Doc(fmt.Sprintf("%d", id))
				if err := tx.Create(ref, data); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (vc VenueCollection) IterableSet(ctx context.Context, client *fs.Client, col *fs.CollectionRef) error {
	vens := make([]*firestore.Venue, 0, len(vc.fsVenues))
	ids := make([]uint64, 0, len(vc.fsVenues))
	for id, v := range vc.fsVenues {
		vens = append(vens, &v)
		ids = append(ids, id)
	}
	for ll := 0; ll < len(vens); ll += 500 {
		ul := ll + 500
		if ul > len(vens) {
			ul = len(vens)
		}
		wr := vens[ll:ul]
		ir := ids[ll:ul]
		err := client.RunTransaction(ctx, func(ctx context.Context, tx *fs.Transaction) error {
			for i, data := range wr {
				id := ir[i]
				ref := col.Doc(fmt.Sprintf("%d", id))
				if err := tx.Set(ref, data); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (vc VenueCollection) DryRun(w io.Writer, col *fs.CollectionRef) (int, error) {
	n := 0
	for id, v := range vc.fsVenues {
		ref := col.Doc(fmt.Sprintf("%d", id))
		nn, err := fmt.Fprintln(w, ref.Path)
		n += nn
		if err != nil {
			return n, err
		}
		nn, err = fmt.Fprintln(w, v)
		n += nn
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

type Week struct {
	Season         string    `json:"season"`
	Number         int       `json:"week"`
	FirstGameStart time.Time `json:"firstGameStart"`
	LastGameStart  time.Time `json:"lastGameStart"`
}

type WeekCollection struct {
	weeks   map[int]Week
	fsWeeks map[int]firestore.Week
}

// Len returns the number of weeks in the collection
func (wc WeekCollection) Len() int {
	return len(wc.weeks)
}

func (w Week) toFirestore() firestore.Week {
	return firestore.Week{
		Number: w.Number,
	}
}

func (wc WeekCollection) LinkRefs(sr *fs.DocumentRef, col *fs.CollectionRef) (map[int]*fs.DocumentRef, error) {
	refs := make(map[int]*fs.DocumentRef)
	for id, week := range wc.weeks {
		fsWeek := week.toFirestore()
		fsWeek.Season = sr
		wc.fsWeeks[id] = fsWeek
		refs[id] = col.Doc(fmt.Sprintf("%d", id))
	}
	return refs, nil
}

func (wc WeekCollection) IterableCreate(ctx context.Context, client *fs.Client, col *fs.CollectionRef) error {
	weeks := make([]*firestore.Week, 0, len(wc.fsWeeks))
	ids := make([]int, 0, len(wc.fsWeeks))
	for id, w := range wc.fsWeeks {
		weeks = append(weeks, &w)
		ids = append(ids, id)
	}
	for ll := 0; ll < len(weeks); ll += 500 {
		ul := ll + 500
		if ul > len(weeks) {
			ul = len(weeks)
		}
		wr := weeks[ll:ul]
		ir := ids[ll:ul]
		err := client.RunTransaction(ctx, func(ctx context.Context, tx *fs.Transaction) error {
			for i, data := range wr {
				id := ir[i]
				ref := col.Doc(fmt.Sprintf("%d", id))
				if err := tx.Create(ref, data); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (wc WeekCollection) IterableSet(ctx context.Context, client *fs.Client, col *fs.CollectionRef) error {
	weeks := make([]*firestore.Week, 0, len(wc.fsWeeks))
	ids := make([]int, 0, len(wc.fsWeeks))
	for id, w := range wc.fsWeeks {
		weeks = append(weeks, &w)
		ids = append(ids, id)
	}
	for ll := 0; ll < len(weeks); ll += 500 {
		ul := ll + 500
		if ul > len(weeks) {
			ul = len(weeks)
		}
		wr := weeks[ll:ul]
		ir := ids[ll:ul]
		err := client.RunTransaction(ctx, func(ctx context.Context, tx *fs.Transaction) error {
			for i, data := range wr {
				id := ir[i]
				ref := col.Doc(fmt.Sprintf("%d", id))
				if err := tx.Set(ref, data); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (wc WeekCollection) DryRun(w io.Writer, col *fs.CollectionRef) (int, error) {
	n := 0
	for id, wk := range wc.fsWeeks {
		ref := col.Doc(fmt.Sprintf("%d", id))
		nn, err := fmt.Fprintln(w, ref.Path)
		n += nn
		if err != nil {
			return n, err
		}
		nn, err = fmt.Fprintln(w, wk)
		n += nn
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

func (wc WeekCollection) Select(n int) (WeekCollection, bool) {
	if week, ok := wc.weeks[n]; ok {
		return WeekCollection{weeks: map[int]Week{n: week}, fsWeeks: map[int]firestore.Week{}}, true
	}
	return WeekCollection{}, false
}
