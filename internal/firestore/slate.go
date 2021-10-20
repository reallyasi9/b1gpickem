package firestore

import (
	"fmt"
	"strings"
	"time"

	fs "cloud.google.com/go/firestore"
)

// Slate represents how a slate is stored in Firestore. Slates contain a collection of SlateGames.
type Slate struct {

	// Created is the creation timestamp of the slate.
	Created time.Time `firestore:"created"`

	// FileName is the full name of the parsed slate file. May be either a string representing a file location or a URL with a gs:// schema representing a Google Cloud Storage location.
	FileName string `firestore:"file"`
}

// SlateGame is a game's data as understood by the slate.
type SlateGame struct {
	// Row is the row in which the game appeared in the slate.
	Row int `firestore:"row"`

	// Game is a references to the actual game picked.
	Game *fs.DocumentRef `firestore:"game"`

	// HomeRank is the ranking of the _true_ home team. A rank of 0 means the team is unranked.
	HomeRank int `firestore:"home_rank"`

	// AwayRank is the ranking of the _true_ away team. A rank of 0 means the team is unranked.
	AwayRank int `firestore:"away_rank"`

	// HomeFavored tells whether or not the _true_ home team is favored.
	HomeFavored bool `firestore:"home_favored"`

	// GOTW is true if this is a "game of the week."
	GOTW bool `firestore:"gotw"`

	// Superdog is true if this game is a "superdog pick."
	Superdog bool `firestore:"superdog"`

	// Value is the point value of this game.
	Value int `firestore:"value"`

	// NeutralDisagreement is true if the slate disagrees with the _true_ venue of the game.
	NeutralDisagreement bool `firestore:"neutral_disagreement"`

	// HomeDisagreement is true if the slate disagrees with which team is the _true_ home team of the game.
	HomeDisagreement bool `firestore:"home_disagreement"`

	// NoisySpread is the spread against which the pickers are picking this game. A value of zero means a straight pick. Positive values favor `HomeTeam`.
	NoisySpread int `firestore:"noisy_spread"`
}

// String implements the Stringer interface.
func (g SlateGame) String() string {
	if g.Superdog {
		return fmt.Sprintf("game %s, home favored %t (%d points)", g.Game.ID, g.HomeFavored, g.Value)
	}

	var sb strings.Builder
	if g.GOTW {
		sb.WriteString("** ")
	}

	sb.WriteString(fmt.Sprintf("game %s, #%d @ #%d, home disagreement %t, neutral disagreement %t", g.Game.ID, g.AwayRank, g.HomeRank, g.HomeDisagreement, g.NeutralDisagreement))

	if g.GOTW {
		sb.WriteString(" **")
	}

	if g.NoisySpread != 0 {
		sb.WriteString(fmt.Sprintf(", home favored %t by ≥ %d", g.HomeFavored, g.NoisySpread))
	}

	return sb.String()
}

// BuildSlateRow creates a row of strings for direct output to a slate spreadsheet.
// func (g SlateGame) BuildSlateRow(ctx context.Context) ([]string, error) {
// 	// error checks
// 	if len(g.Teams) != 2 {
// 		return nil, fmt.Errorf("illegal number of teams %d", len(g.Teams))
// 	}
// 	if g.HomeIndex < 0 || g.HomeIndex >= len(g.Teams) {
// 		return nil, fmt.Errorf("illegal home index value %d", g.HomeIndex)
// 	}
// 	if g.FavoredIndex < 0 || g.FavoredIndex >= len(g.Teams) {
// 		return nil, fmt.Errorf("illegal favored index value %d", g.FavoredIndex)
// 	}
// 	if len(g.Teams) != len(g.Ranks) {
// 		return nil, fmt.Errorf("teams and ranks slice have different lengths: %d != %d", len(g.Teams), len(g.Ranks))
// 	}

// 	// game, noise, pick, spread, notes, expected value
// 	output := make([]string, 2)

// 	idx2 := g.HomeIndex
// 	if g.Superdog {
// 		idx2 = g.FavoredIndex
// 	}
// 	idx1 := 1 - idx2

// 	rank1 := g.Ranks[idx1]
// 	rank2 := g.Ranks[idx2]

// 	team1Ref := g.Teams[idx1]
// 	team2Ref := g.Teams[idx2]

// 	var (
// 		team1Doc *fs.DocumentSnapshot
// 		team2Doc *fs.DocumentSnapshot
// 		err      error
// 	)

// 	if team1Doc, err = team1Ref.Get(ctx); err != nil {
// 		return nil, err
// 	}
// 	if team2Doc, err = team2Ref.Get(ctx); err != nil {
// 		return nil, err
// 	}

// 	var (
// 		team1    Team
// 		team2    Team
// 		favorite Team
// 	)

// 	if err = team1Doc.DataTo(&team1); err != nil {
// 		return nil, err
// 	}
// 	if err = team2Doc.DataTo(&team2); err != nil {
// 		return nil, err
// 	}
// 	favorite = team1
// 	if idx2 == g.FavoredIndex {
// 		favorite = team2
// 	}

// 	var sb strings.Builder

// 	// Straight-up and Noisy Spread: "[** ][#X ]Team 1 {@|vs} [#X ]Team 2[ **]"
// 	// Superdog:                     nothing
// 	if g.GOTW {
// 		sb.WriteString("** ")
// 	}

// 	if rank1 > 0 {
// 		sb.WriteString(fmt.Sprintf("#%d ", rank1))
// 	}

// 	sb.WriteString(team1.ShortNames[0])

// 	if g.Superdog {
// 		sb.WriteString(" over ")
// 	} else if g.NeutralSite {
// 		sb.WriteString(" vs. ")
// 	} else {
// 		sb.WriteString(" @ ")
// 	}

// 	if rank2 > 0 {
// 		sb.WriteString(fmt.Sprintf("#%d ", rank2))
// 	}

// 	sb.WriteString(team2.School)

// 	if g.GOTW {
// 		sb.WriteString(" **")
// 	}

// 	if g.Superdog {
// 		sb.WriteString(fmt.Sprintf(" (%d points, if correct)", g.Value))
// 		output[1] = sb.String()
// 	} else {
// 		output[0] = sb.String()
// 	}

// 	// Straight-up:  "Enter name of predicted winner"
// 	// Superdog:     "[#X ]Team 1 over [#X ]Team 2 (X points, if correct)" -- already written
// 	// Noisy spread: "Enter {Team 1|Team 2} iff you predict {Team 1|Team 2} wins by at least X points."
// 	if g.NoisySpread != 0 {
// 		ns := g.NoisySpread
// 		if ns < 0 {
// 			ns = -ns
// 		}
// 		output[1] = fmt.Sprintf("Enter %s iff you predict %s wins by at least %d points.", favorite.ShortNames[0], favorite.ShortNames[0], ns)
// 	} else if !g.Superdog {
// 		output[1] = "Enter name of predicted winner"
// 	}

// 	// Other output is determined by the pick.

// 	return output, nil
// }
