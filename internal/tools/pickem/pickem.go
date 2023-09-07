package pickem

import (
	"context"
	"fmt"
	"log"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

func Pickem(ctx *Context) error {

	_, seasonRef, err := firestore.GetSeason(ctx, ctx.FirestoreClient, ctx.Season)
	if err != nil {
		return fmt.Errorf("Pickem: failed to get season: %w", err)
	}
	_, weekRef, err := firestore.GetWeek(ctx, seasonRef, ctx.Week)
	if err != nil {
		return fmt.Errorf("Pickem: failed to get week: %w", err)
	}
	_, pickerRef, err := firestore.GetPickerByLukeName(ctx, ctx.FirestoreClient, ctx.Picker)
	if err != nil {
		return fmt.Errorf("Pickem: failed to get picker '%s': %w", ctx.Picker, err)
	}

	slateGames, gameRefs, err := firestore.GetSlateGames(ctx, weekRef)
	if err != nil {
		return fmt.Errorf("Pickem: failed to get slate games: %w", err)
	}

	gameLookup, err := newSlateGamesByTeam(ctx, slateGames, gameRefs)
	if err != nil {
		return fmt.Errorf("Pickem: failed to build slate game lookup: %w", err)
	}
	sdGames := make(map[string]firestore.SlateGame)
	for i, game := range slateGames {
		if game.Superdog {
			sdGames[gameRefs[i].ID] = game
		}
	}

	picks, pickRefs, err := firestore.GetPicks(ctx, weekRef, pickerRef)
	if err != nil {
		return fmt.Errorf("Pickem: failed to get picks for picker '%s': %w", ctx.Picker, err)
	}

	pickLookup := newPicksByGameID(picks, pickRefs)

	teams, teamRefs, err := firestore.GetTeams(ctx, seasonRef)
	if err != nil {
		return fmt.Errorf("Pickem: failed to get teams: %w", err)
	}

	teamLookup, err2 := firestore.NewTeamRefsByOtherName(teams, teamRefs)
	if err2 != nil {
		panic(err2)
	}

	picksToUpdate := make(map[string]firestore.Pick)
	newPicks := make([]firestore.Pick, 0)

	for _, pickedTeam := range ctx.Picks {
		_, gameRef, ok := gameLookup.Lookup(pickedTeam)
		if !ok {
			return fmt.Errorf("Pickem: Team '%s' not found in slate games", pickedTeam)
		}
		pickedTeamRef, ok := teamLookup[pickedTeam]
		if !ok {
			return fmt.Errorf("Pickem: Team '%s' not found in teams", pickedTeam)
		}
		pick, pickRef, ok := pickLookup.Lookup(gameRef.ID)
		pick.PickedTeam = pickedTeamRef
		if !ok {
			pick.Picker = pickerRef
			pick.SlateGame = gameRef
			newPicks = append(newPicks, pick)
		} else {
			pick.ModelPrediction = nil
			pick.PredictedProbability = 0.
			pick.PredictedSpread = 0.
			picksToUpdate[pickRef.ID] = pick
		}
		log.Printf("Picked %s for picker %s in game %s", pickedTeam, ctx.Picker, gameRef.ID)
	}

	if ctx.SuperDog != "" {
		game, gameRef, ok := gameLookup.Lookup(ctx.SuperDog)
		if !ok {
			return fmt.Errorf("Pickem: Team '%s' not found in slate games", ctx.SuperDog)
		}
		if !game.Superdog {
			return fmt.Errorf("Pickem: Team '%s' not found in superdog games", ctx.SuperDog)
		}
		pickedTeamRef, ok := teamLookup[ctx.SuperDog]
		if !ok {
			return fmt.Errorf("Pickem: Team '%s' not found in teams", ctx.SuperDog)
		}

		// unpick other SD games
		for id := range sdGames {
			pick, pickRef, ok := pickLookup.Lookup(id)
			if !ok {
				// don't care about unpicked SD games
				continue
			}
			pick.PickedTeam = nil
			pick.ModelPrediction = nil
			pick.PredictedProbability = 0.
			pick.PredictedSpread = 0.
			picksToUpdate[pickRef.ID] = pick
		}

		pick, pickRef, ok := pickLookup.Lookup(gameRef.ID)
		pick.PickedTeam = pickedTeamRef
		if !ok {
			pick.Picker = pickerRef
			pick.SlateGame = gameRef
			newPicks = append(newPicks, pick)
		} else {
			pick.ModelPrediction = nil
			pick.PredictedProbability = 0.
			pick.PredictedSpread = 0.
			picksToUpdate[pickRef.ID] = pick
		}

		log.Printf("Picked superdog %s for picker %s in game %s", ctx.SuperDog, ctx.Picker, gameRef.ID)
	}

	if ctx.DryRun {
		log.Printf("DRY RUN: would create %d new picks", len(newPicks))
		for _, pick := range newPicks {
			log.Print(pick)
		}
		log.Printf("DRY RUN: would update %d previously-made picks", len(picksToUpdate))
		for id, pick := range picksToUpdate {
			log.Printf("%s -> %s", id, pick)
		}
		return nil
	}

	if len(picksToUpdate) > 0 && !ctx.Force {
		return fmt.Errorf("Pickem: refusing to proceed when updating %d picks: add --force flag to force update", len(picksToUpdate))
	}

	picksCollection := weekRef.Collection(firestore.PICKS_COLLECTION)
	err = ctx.FirestoreClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
		for id, pick := range picksToUpdate {
			ref := picksCollection.Doc(id)
			if err := t.Set(ref, &pick); err != nil {
				return err
			}
		}
		for _, pick := range newPicks {
			ref := picksCollection.NewDoc()
			if err := t.Create(ref, &pick); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("Pickem: failed to complete transaction to update picks: %w", err)
	}

	return nil
}

type slateGamesByTeam struct {
	games       []firestore.SlateGame
	gameRefs    []*fs.DocumentRef
	indexLookup map[string]int
}

func newSlateGamesByTeam(ctx context.Context, games []firestore.SlateGame, refs []*fs.DocumentRef) (*slateGamesByTeam, error) {
	lookup := make(map[string]int)
	for i, sg := range games {
		gr := sg.Game
		snap, err := gr.Get(ctx)
		if err != nil {
			return nil, err
		}

		var game firestore.Game
		err = snap.DataTo(&game)
		if err != nil {
			return nil, err
		}

		snap, err = game.HomeTeam.Get(ctx)
		if err != nil {
			return nil, err
		}

		var team firestore.Team
		err = snap.DataTo(&team)
		if err != nil {
			return nil, err
		}

		for _, name := range team.OtherNames {
			if _, ok := lookup[name]; ok {
				return nil, fmt.Errorf("newSlateGamesByTeam: ambiguous team OtherName '%s'", name)
			}
			lookup[name] = i
		}

		snap, err = game.AwayTeam.Get(ctx)
		if err != nil {
			return nil, err
		}

		err = snap.DataTo(&team)
		if err != nil {
			return nil, err
		}

		for _, name := range team.OtherNames {
			if _, ok := lookup[name]; ok {
				return nil, fmt.Errorf("newSlateGamesByTeam: ambiguous team OtherName '%s'", name)
			}
			lookup[name] = i
		}
	}

	return &slateGamesByTeam{
		games:       games,
		gameRefs:    refs,
		indexLookup: lookup,
	}, nil
}

func (s *slateGamesByTeam) Lookup(team string) (sg firestore.SlateGame, ref *fs.DocumentRef, ok bool) {
	var idx int
	idx, ok = s.indexLookup[team]
	if !ok {
		return
	}
	sg = s.games[idx]
	ref = s.gameRefs[idx]
	return
}

type picksByGameID struct {
	picks       []firestore.Pick
	pickRefs    []*fs.DocumentRef
	indexLookup map[string]int
}

func newPicksByGameID(picks []firestore.Pick, refs []*fs.DocumentRef) *picksByGameID {
	lookup := make(map[string]int)
	for i, p := range picks {
		lookup[p.SlateGame.ID] = i
	}
	return &picksByGameID{
		picks:       picks,
		pickRefs:    refs,
		indexLookup: lookup,
	}
}

func (p *picksByGameID) Lookup(id string) (pick firestore.Pick, ref *fs.DocumentRef, ok bool) {
	var idx int
	idx, ok = p.indexLookup[id]
	if !ok {
		return
	}
	pick = p.picks[idx]
	ref = p.pickRefs[idx]
	return
}
