package firestore

import (
	"context"
	"fmt"
	"strings"
	"time"

	fs "cloud.google.com/go/firestore"
)

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
	Venue *fs.DocumentRef `jsfirestoreon:"venue"`

	// HomePoints is the number of points earned by the home team at end of game.
	HomePoints *int `firestore:"home_points"`

	// AwayPoints is the number of points earned by the away team at end of game.
	AwayPoints *int `firestore:"away_points"`
}

func (g Game) String() string {
	var sb strings.Builder
	sb.WriteString("Game\n")
	sb.WriteString(treeRef("HomeTeam", 0, false, g.HomeTeam))
	sb.WriteRune('\n')
	sb.WriteString(treeRef("AwayTeam", 0, false, g.AwayTeam))
	sb.WriteRune('\n')
	sb.WriteString(treeString("StartTime", 0, false, g.StartTime.Format(time.UnixDate)))
	sb.WriteRune('\n')
	sb.WriteString(treeBool("StartTimeTBD", 0, false, g.StartTimeTBD))
	sb.WriteRune('\n')
	sb.WriteString(treeBool("NeutralSite", 0, false, g.NeutralSite))
	sb.WriteRune('\n')
	sb.WriteString(treeRef("Venue", 0, false, g.Venue))
	sb.WriteRune('\n')
	sb.WriteString(treeIntPtr("HomePoints", 0, false, g.HomePoints))
	sb.WriteRune('\n')
	sb.WriteString(treeIntPtr("AwayPoints", 0, true, g.AwayPoints))
	return sb.String()
}

// GetGames returns a collection of teams for a given season.
func GetGames(ctx context.Context, client *fs.Client, week *fs.DocumentRef) ([]Game, []*fs.DocumentRef, error) {
	refs, err := week.Collection("games").DocumentRefs(ctx).GetAll()
	if err != nil {
		return nil, nil, fmt.Errorf("error getting game document refs for week %s: %w", week.ID, err)
	}
	games := make([]Game, len(refs))
	for i, r := range refs {
		ss, err := r.Get(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("error getting game snapshot %s: %w", r.ID, err)
		}
		var g Game
		err = ss.DataTo(&g)
		if err != nil {
			return nil, nil, fmt.Errorf("error getting game snapshot data %s: %w", r.ID, err)
		}
		games[i] = g
	}
	return games, refs, nil
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
