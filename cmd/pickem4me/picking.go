package main

import (
	"context"
	"fmt"
	"sort"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/bts"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

type SlateGameRef struct {
	SlateGame firestore.SlateGame
	Ref       *fs.DocumentRef
}

type ModelPredictionRef struct {
	ModelPrediction firestore.ModelPrediction
	Ref             *fs.DocumentRef
}

type ModelPerformanceRef struct {
	ModelPerformance firestore.ModelPerformance
	Ref              *fs.DocumentRef
}

type GamePredictions struct {
	game         firestore.Game
	slateGameRef SlateGameRef
	predictions  map[string]ModelPredictionRef  // lookup by model name
	performances map[string]ModelPerformanceRef // lookup by model name
	perfs        []*ModelPerformanceRef         // sortable version for fallback
}

func NewGamePredictions(ctx context.Context, game firestore.Game, slateGame firestore.SlateGame, slateGameRef *fs.DocumentRef, performances []firestore.ModelPerformance, performanceRefs []*fs.DocumentRef) (*GamePredictions, error) {
	// with the slate game ref, we have the game ref, so we can get the predictions
	gameRef := slateGame.Game
	predSnaps, err := gameRef.Collection(firestore.PREDICTIONS_COLLECTION).Documents(ctx).GetAll()
	if err != nil {
		return nil, fmt.Errorf("failed to get prediction snapshots for game %s: %w", gameRef.ID, err)
	}
	predictions := make(map[string]ModelPredictionRef)
	for _, snap := range predSnaps {
		var mp firestore.ModelPrediction
		if err := snap.DataTo(&mp); err != nil {
			return nil, fmt.Errorf("failed to get data from prediction snapshot %s: %w", snap.Ref.ID, err)
		}
		pr := ModelPredictionRef{ModelPrediction: mp, Ref: snap.Ref}
		predictions[mp.Model.ID] = pr
	}
	perfRefs := make(map[string]ModelPerformanceRef)
	perfs := make([]*ModelPerformanceRef, len(performances))
	for i, p := range performances {
		r := performanceRefs[i]
		perfRef := ModelPerformanceRef{ModelPerformance: p, Ref: r}
		perfRefs[p.Model.ID] = perfRef
		perfs[i] = &perfRef
	}

	gp := &GamePredictions{
		game:         game,
		slateGameRef: SlateGameRef{slateGame, slateGameRef},
		predictions:  predictions,
		performances: perfRefs,
		perfs:        perfs,
	}
	return gp, nil
}

type ByMAE []*ModelPerformanceRef

func (x ByMAE) Len() int      { return len(x) }
func (x ByMAE) Swap(i, j int) { x[i], x[j] = x[j], x[i] }
func (x ByMAE) Less(i, j int) bool {
	return x[i].ModelPerformance.MAE < x[j].ModelPerformance.MAE
}

type ByWins []*ModelPerformanceRef

func (x ByWins) Len() int      { return len(x) }
func (x ByWins) Swap(i, j int) { x[i], x[j] = x[j], x[i] }
func (x ByWins) Less(i, j int) bool {
	return x[i].ModelPerformance.Wins < x[j].ModelPerformance.Wins
}

type ModelNotFoundError string

func (e ModelNotFoundError) Error() string {
	return fmt.Sprintf("model '%s' not found", string(e))
}

func (gp GamePredictions) Pick(preferredModel string) (*firestore.Pick, error) {
	var pred ModelPredictionRef
	var perf ModelPerformanceRef
	var found bool
	if pred, found = gp.predictions[preferredModel]; !found {
		return nil, ModelNotFoundError(preferredModel)
	}
	if perf, found = gp.performances[preferredModel]; !found {
		return nil, ModelNotFoundError(preferredModel)
	}

	p := &firestore.Pick{
		SlateGame: gp.slateGameRef.Ref,
	}
	p.FillOut(gp.game, perf.ModelPerformance, pred.ModelPrediction, pred.Ref, gp.slateGameRef.SlateGame.NoisySpread)
	return p, nil
}

func (gp GamePredictions) Fallback(sagarinModel *bts.GaussianSpreadModel) (*firestore.Pick, error) {
	fallbackOrder := gp.perfs
	if gp.slateGameRef.SlateGame.Superdog || gp.slateGameRef.SlateGame.NoisySpread != 0 {
		sort.Sort(ByMAE(fallbackOrder))
	} else {
		sort.Sort(sort.Reverse(ByWins(fallbackOrder)))
	}

	for _, mp := range fallbackOrder {
		if p, err := gp.Pick(mp.ModelPerformance.Model.ID); err == nil {
			return p, err
		}
		// keep trying!
	}
	// no predictions found matching fallback models, try Sagarin

	if sagarinModel == nil {
		return nil, fmt.Errorf("no fallback model and nil Sagarin model")
	}

	// I can cheat because I know the model I am looking for
	var perf ModelPerformanceRef
	var found bool
	if perf, found = gp.performances["linesag"]; !found {
		return nil, ModelNotFoundError("linesag")
	}

	loc := bts.Home
	if gp.game.NeutralSite {
		loc = bts.Neutral
	}
	game := bts.NewGame(bts.Team(gp.game.HomeTeam.ID), bts.Team(gp.game.AwayTeam.ID), loc)
	_, spread := sagarinModel.Predict(game)
	mp := firestore.ModelPrediction{
		Model:       perf.ModelPerformance.Model,
		HomeTeam:    gp.game.HomeTeam,
		AwayTeam:    gp.game.AwayTeam,
		NeutralSite: gp.game.NeutralSite,
		Spread:      spread,
	}

	p := &firestore.Pick{
		SlateGame: gp.slateGameRef.Ref,
	}
	p.FillOut(gp.game, perf.ModelPerformance, mp, nil, gp.slateGameRef.SlateGame.NoisySpread)

	return p, nil
}

type DogPick struct {
	teamID string
	points int
	prob   float64
}

type ByValue []DogPick

// Len implements Sortable interface
func (b ByValue) Len() int {
	return len(b)
}

// Less implements Sortable interface
func (b ByValue) Less(i, j int) bool {
	vi := b[i].prob * float64(b[i].points)
	vj := b[j].prob * float64(b[j].points)
	if vi == vj {
		return b[i].points < b[j].points
	}
	return vi < vj
}

// Swap implements Sortable interface
func (b ByValue) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}
