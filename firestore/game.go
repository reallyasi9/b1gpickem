package firestore

import (
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/firestore"
)

// Game is a game's data for storing picks in Firestore.
type Game struct {
	// Teams are references to the teams playing in the game.
	Teams []*firestore.DocumentRef `firestore:"teams"`

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

	// NoisySpread is the spread against which the pickers are picking this game. A value of zero means a straight pick. Positive values favor `HomeTeam`.
	NoisySpread int `firestore:"noisy_spread"`

	// Predictions are references to predictions from the various models, indexed by model short name.
	Predictions map[string]*firestore.DocumentRef `firestore:"predictions"`
}

// BuildSlateRow creates a row of strings for direct output to a slate spreadsheet.
func (g Game) BuildSlateRow(ctx context.Context) ([]string, error) {
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
		team1Doc *firestore.DocumentSnapshot
		team2Doc *firestore.DocumentSnapshot
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
