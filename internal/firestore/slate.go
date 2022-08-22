package firestore

import (
	"context"
	"fmt"
	"strings"
	"time"

	fs "cloud.google.com/go/firestore"
)

const SLATES_COLLECTION = "slates"
const SLATE_GAMES_COLLECTION = "games"

// Slate represents how a slate is stored in Firestore. Slates contain a collection of SlateGames.
type Slate struct {

	// Created is the creation timestamp of the slate.
	Created time.Time `firestore:"created"`

	// Parsed is the parse timestamp of the slate.
	Parsed time.Time `firestore:"parsed,serverTimestamp"`

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

type NoSlateError string

func (e NoSlateError) Error() string {
	return string(e)
}

func GetSlateGames(ctx context.Context, weekRef *fs.DocumentRef) (sgs []SlateGame, refs []*fs.DocumentRef, err error) {
	var snaps []*fs.DocumentSnapshot
	snaps, err = weekRef.Collection(SLATES_COLLECTION).OrderBy("parsed", fs.Desc).Limit(1).Documents(ctx).GetAll()
	if err != nil {
		return
	}
	if len(snaps) == 0 {
		err = NoSlateError("no slate in collection")
		return
	}

	snaps, err = snaps[0].Ref.Collection(SLATE_GAMES_COLLECTION).Documents(ctx).GetAll()
	if err != nil {
		return
	}

	sgs = make([]SlateGame, len(snaps))
	refs = make([]*fs.DocumentRef, len(snaps))
	for i, snap := range snaps {
		var sg SlateGame
		err = snap.DataTo(&sg)
		if err != nil {
			return
		}
		sgs[i] = sg
		refs[i] = snap.Ref
	}
	return
}
