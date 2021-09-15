package cfbdata

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/firestore"
)

type Game struct {
	ID           int64     `json:"id"`
	Week         int       `json:"week"`
	StartTime    time.Time `json:"start_date"`
	StartTimeTBD bool      `json:"start_time_tbd"`
	NeutralSite  bool      `json:"neutral_site"`
	VenueID      int64     `json:"venue_id"`
	HomeID       int64     `json:"home_id"`
	AwayID       int64     `json:"away_id"`
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
	games   []Game
	fsGames []firestore.Game
	refs    []*fs.DocumentRef
	ids     map[int64]int
}

func GetGames(client *http.Client, key string, year int) (GameCollection, error) {
	query := fmt.Sprintf("?year=%d", year)
	body, err := doRequest(client, key, "https://api.collegefootballdata.com/games"+query)
	if err != nil {
		return GameCollection{}, fmt.Errorf("failed to do game request: %v", err)
	}

	var games []Game
	err = json.Unmarshal(body, &games)
	if err != nil {
		return GameCollection{}, fmt.Errorf("failed to unmarshal games response body: %v", err)
	}

	f := make([]firestore.Game, len(games))
	refs := make([]*fs.DocumentRef, len(games))
	ids := make(map[int64]int)
	for i, g := range games {
		f[i] = g.toFirestore()
		ids[g.ID] = i
	}

	return GameCollection{games: games, fsGames: f, refs: refs, ids: ids}, nil
}

// Len gets the length of the collection
func (gc GameCollection) Len() int {
	return len(gc.games)
}

func (gc GameCollection) Ref(i int) *fs.DocumentRef {
	return gc.refs[i]
}

func (gc GameCollection) ID(i int) int64 {
	return gc.games[i].ID
}

func (gc GameCollection) Datum(i int) interface{} {
	return gc.fsGames[i]
}

func (gc GameCollection) RefByID(id int64) (*fs.DocumentRef, bool) {
	if i, ok := gc.ids[id]; ok {
		return gc.refs[i], true
	}
	return nil, false
}

// GetWeek splits the GameCollection into multiple GameCollections indexed by week.
func (gc GameCollection) GetWeek(week int) GameCollection {
	gw := make([]Game, 0)
	fw := make([]firestore.Game, 0)
	rw := make([]*fs.DocumentRef, 0)
	iw := make(map[int64]int)
	j := 0
	for i, g := range gc.games {
		if g.Week == week {
			gw = append(gw, g)
			fw = append(fw, gc.fsGames[i])
			rw = append(rw, gc.refs[i])
			iw[g.ID] = j
			j++
		}
	}
	return GameCollection{games: gw, fsGames: fw, refs: rw, ids: iw}
}

func (gc GameCollection) LinkRefs(tc TeamCollection, vc VenueCollection, col *fs.CollectionRef) error {
	for i, g := range gc.games {
		id := g.ID
		fsg := gc.fsGames[i]
		homeTeamID := g.HomeID
		awayTeamID := g.AwayID
		venueID := g.VenueID

		var ok bool
		if fsg.HomeTeam, ok = tc.RefByID(homeTeamID); !ok {
			return fmt.Errorf("home team %d in game %d not found in reference map", homeTeamID, id)
		}
		if fsg.AwayTeam, ok = tc.RefByID(awayTeamID); !ok {
			return fmt.Errorf("away team %d in game %d not found in reference map", awayTeamID, id)
		}
		if fsg.Venue, ok = vc.RefByID(venueID); !ok {
			return fmt.Errorf("venue %d for game %d not found in reference map", venueID, id)
		}

		gc.fsGames[i] = fsg
		gc.refs[i] = col.Doc(fmt.Sprintf("%d", id))
	}
	return nil
}

func (gc GameCollection) FprintDatum(w io.Writer, i int) (int, error) {
	return fmt.Fprint(w, gc.fsGames[i].String())
}
