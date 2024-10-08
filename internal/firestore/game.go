package firestore

import (
	"context"
	"fmt"
	"time"

	fs "cloud.google.com/go/firestore"
)

// GAMES_COLLECTION is the path to the games collection in Firestore.
const GAMES_COLLECTION = "games"

// Game is a ground truth game.
type Game struct {
	// HomeTeam is the nominal home team in the game.
	HomeTeam *fs.DocumentRef `firestore:"home_team"`

	// AwayTeam is the nominal away team in the game.
	AwayTeam *fs.DocumentRef `firestore:"away_team"`

	// StartTime is the nominal kickoff time of the game.
	StartTime time.Time `firestore:"start_time"`

	// StartTimeTBD is a flag that reports whether or not `StartTime` can be trusted.
	StartTimeTBD bool `firestore:"start_time_tbd"`

	// NeutralSite is true if the game is played at neither the home nor away team's venue.
	NeutralSite bool `firestore:"neutral_site"`

	// Venue is the venue of the game.
	Venue *fs.DocumentRef `firestore:"venue"`

	// HomePoints is the number of points earned by the home team at end of game.
	HomePoints *int `firestore:"home_points"`

	// AwayPoints is the number of points earned by the away team at end of game.
	AwayPoints *int `firestore:"away_points"`
}

func (g Game) String() string {
	vs := "@"
	if g.NeutralSite {
		vs = "vs."
	}
	t := ""
	if g.StartTimeTBD {
		t = "time TBD"
	} else {
		t = g.StartTime.Format(time.UnixDate)
	}
	ap := ""
	if g.AwayPoints != nil {
		ap = fmt.Sprintf(" (%d)", *g.AwayPoints)
	}
	hp := ""
	if g.HomePoints != nil {
		hp = fmt.Sprintf(" (%d)", *g.HomePoints)
	}
	return fmt.Sprintf("%s%s %s %s%s at %s (%s)", g.AwayTeam.ID, ap, vs, g.HomeTeam.ID, hp, g.Venue.ID, t)
}

// GetGames returns a collection of games for a given week.
func GetGames(ctx context.Context, week *fs.DocumentRef) ([]Game, []*fs.DocumentRef, error) {
	gameSnaps, err := week.Collection(GAMES_COLLECTION).Documents(ctx).GetAll()
	if err != nil {
		return nil, nil, fmt.Errorf("error getting game documents for week %s: %w", week.ID, err)
	}
	games := make([]Game, len(gameSnaps))
	refs := make([]*fs.DocumentRef, len(gameSnaps))
	for i, ss := range gameSnaps {
		var g Game
		err = ss.DataTo(&g)
		if err != nil {
			return nil, nil, fmt.Errorf("error getting game snapshot data %s: %w", ss.Ref.ID, err)
		}
		games[i] = g
		refs[i] = ss.Ref
	}
	return games, refs, nil
}

// GetGamesByStartTime returns games that fall between two times (inclusive of lower bound, exclusive of upper).
func GetGamesByStartTime(ctx context.Context, season *fs.DocumentRef, from, to time.Time) (games []Game, refs []*fs.DocumentRef, err error) {
	weekRefs, err := season.Collection(WEEKS_COLLECTION).DocumentRefs(ctx).GetAll()
	if err != nil {
		return
	}
	games = make([]Game, 0)
	refs = make([]*fs.DocumentRef, 0)
	for _, ref := range weekRefs {
		weekGames, weekGameRefs, e := GetGames(ctx, ref)
		if e != nil {
			err = e
			return
		}
		for i, game := range weekGames {
			if (game.StartTime.Equal(from) || game.StartTime.After(from)) && game.StartTime.Before(to) {
				games = append(games, game)
				refs = append(refs, weekGameRefs[i])
			}
		}
	}
	return
}

type Matchup struct {
	Home    string
	Away    string
	Neutral bool
}

// GameRefsByMatchup is a struct for quick lookups of games by home/away teams and for correcting who is home, who is away, and whether the game is at a neutral site.
type GameRefsByMatchup map[Matchup]*fs.DocumentRef

func NewGameRefsByMatchup(games []Game, refs []*fs.DocumentRef) GameRefsByMatchup {
	m := make(GameRefsByMatchup)
	for i, g := range games {
		matchup := Matchup{
			Home:    g.HomeTeam.ID,
			Away:    g.AwayTeam.ID,
			Neutral: g.NeutralSite,
		}
		m[matchup] = refs[i]
	}
	return m
}

func (g GameRefsByMatchup) LookupCorrectMatchup(m Matchup) (game *fs.DocumentRef, swap bool, wrongNeutral bool, ok bool) {
	if game, ok = g[m]; ok {
		return
	}

	m.Neutral = !m.Neutral
	if game, ok = g[m]; ok {
		wrongNeutral = true
		return
	}

	m.Home, m.Away = m.Away, m.Home
	if game, ok = g[m]; ok {
		swap = true
		wrongNeutral = true
		return
	}

	m.Neutral = !m.Neutral
	if game, ok = g[m]; ok {
		swap = true
		return
	}

	return
}

func (g GameRefsByMatchup) LookupTeam(t string) (*fs.DocumentRef, bool) {
	for m, g := range g {
		if m.Home == t || m.Away == t {
			return g, true
		}
	}
	return nil, false
}
