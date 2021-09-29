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

// Game is a game's data for storing picks in fs.
type SlateGame struct {
	// Teams are references to the teams playing in the game.
	Teams []*fs.DocumentRef `firestore:"teams"`

	// Ranks are the rankings of the teams playing the game. The ranks correspond to the teams in the Teams array. A rank of zero means the team is unranked.
	Ranks []int `firestore:"ranks"`

	// HomeIndex is the index in `Teams` and `Ranks` that represents the nominal home team as given in the slate.
	HomeIndex int `firestore:"home"`

	// FavoredIndex is the index of the "overdog" in `Teams` and `Ranks`. Used only in "superdog" games.
	FavoredIndex int `firestore:"overdog"`

	// GOTW is true if this is a "game of the week."
	GOTW bool `firestore:"gotw"`

	// Superdog is true if this game is a "superdog pick."
	Superdog bool `firestore:"superdog"`

	// Value is the point value of this game.
	Value int `firestore:"value"`

	// NeutralSite is true if the slate thinks this game takes place at a neutral site.
	NeutralSite bool `firestore:"neutral_site"`

	// Venue is a reference to a Venue document for this game.
	Venue *fs.DocumentRef `firestore:"venue"`

	// NoisySpread is the spread against which the pickers are picking this game. A value of zero means a straight pick. Positive values favor `HomeTeam`.
	NoisySpread int `firestore:"noisy_spread"`

	// Predictions are references to predictions from the various models, indexed by model short name.
	Predictions map[string]*fs.DocumentRef `firestore:"predictions"`
}

// String implements the Stringer interface.
func (g SlateGame) String() string {
	if g.Superdog {
		return fmt.Sprintf("%s over %s (%d points)", g.Teams[1-g.FavoredIndex].ID, g.Teams[g.FavoredIndex].ID, g.Value)
	}

	var sb strings.Builder
	if g.GOTW {
		sb.WriteString("** ")
	}

	if g.Ranks[0] > 0 {
		sb.WriteString(fmt.Sprintf("#%d ", g.Ranks[0]))
	}

	sb.WriteString(g.Teams[0].ID)

	if g.NeutralSite {
		sb.WriteString(" n ")
	} else if g.HomeIndex == 1 {
		sb.WriteString(" @ ")
	} else {
		sb.WriteString(" v ")
	}

	if g.Ranks[1] > 0 {
		sb.WriteString(fmt.Sprintf("#%d ", g.Ranks[1]))
	}

	sb.WriteString(g.Teams[1].ID)

	if g.GOTW {
		sb.WriteString(" **")
	}

	if g.NoisySpread != 0 {
		sb.WriteString(fmt.Sprintf(", %s by ≥ %d", g.Teams[g.FavoredIndex].ID, g.NoisySpread))
	}

	return sb.String()
}

// BuildSlateRow creates a row of strings for direct output to a slate spreadsheet.
func (g SlateGame) BuildSlateRow(ctx context.Context) ([]string, error) {
	// error checks
	if len(g.Teams) != 2 {
		return nil, fmt.Errorf("illegal number of teams %d", len(g.Teams))
	}
	if g.HomeIndex < 0 || g.HomeIndex >= len(g.Teams) {
		return nil, fmt.Errorf("illegal home index value %d", g.HomeIndex)
	}
	if g.FavoredIndex < 0 || g.FavoredIndex >= len(g.Teams) {
		return nil, fmt.Errorf("illegal favored index value %d", g.FavoredIndex)
	}
	if len(g.Teams) != len(g.Ranks) {
		return nil, fmt.Errorf("teams and ranks slice have different lengths: %d != %d", len(g.Teams), len(g.Ranks))
	}

	// game, noise, pick, spread, notes, expected value
	output := make([]string, 2)

	idx2 := g.HomeIndex
	if g.Superdog {
		idx2 = g.FavoredIndex
	}
	idx1 := 1 - idx2

	rank1 := g.Ranks[idx1]
	rank2 := g.Ranks[idx2]

	team1Ref := g.Teams[idx1]
	team2Ref := g.Teams[idx2]

	var (
		team1Doc *fs.DocumentSnapshot
		team2Doc *fs.DocumentSnapshot
		err      error
	)

	if team1Doc, err = team1Ref.Get(ctx); err != nil {
		return nil, err
	}
	if team2Doc, err = team2Ref.Get(ctx); err != nil {
		return nil, err
	}

	var (
		team1    Team
		team2    Team
		favorite Team
	)

	if err = team1Doc.DataTo(&team1); err != nil {
		return nil, err
	}
	if err = team2Doc.DataTo(&team2); err != nil {
		return nil, err
	}
	favorite = team1
	if idx2 == g.FavoredIndex {
		favorite = team2
	}

	var sb strings.Builder

	// Straight-up and Noisy Spread: "[** ][#X ]Team 1 {@|vs} [#X ]Team 2[ **]"
	// Superdog:                     nothing
	if g.GOTW {
		sb.WriteString("** ")
	}

	if rank1 > 0 {
		sb.WriteString(fmt.Sprintf("#%d ", rank1))
	}

	sb.WriteString(team1.ShortNames[0])

	if g.Superdog {
		sb.WriteString(" over ")
	} else if g.NeutralSite {
		sb.WriteString(" vs. ")
	} else {
		sb.WriteString(" @ ")
	}

	if rank2 > 0 {
		sb.WriteString(fmt.Sprintf("#%d ", rank2))
	}

	sb.WriteString(team2.School)

	if g.GOTW {
		sb.WriteString(" **")
	}

	if g.Superdog {
		sb.WriteString(fmt.Sprintf(" (%d points, if correct)", g.Value))
		output[1] = sb.String()
	} else {
		output[0] = sb.String()
	}

	// Straight-up:  "Enter name of predicted winner"
	// Superdog:     "[#X ]Team 1 over [#X ]Team 2 (X points, if correct)" -- already written
	// Noisy spread: "Enter {Team 1|Team 2} iff you predict {Team 1|Team 2} wins by at least X points."
	if g.NoisySpread != 0 {
		ns := g.NoisySpread
		if ns < 0 {
			ns = -ns
		}
		output[1] = fmt.Sprintf("Enter %s iff you predict %s wins by at least %d points.", favorite.ShortNames[0], favorite.ShortNames[0], ns)
	} else if !g.Superdog {
		output[1] = "Enter name of predicted winner"
	}

	// Other output is determined by the pick.

	return output, nil
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
