package btspick

import (
	"fmt"
	"log"
	"strings"
	"sync"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

func MakePicks(ctx *Context) error {
	season, seasonRef, err := firestore.GetSeason(ctx, ctx.FirestoreClient, ctx.Season)
	if err != nil {
		return fmt.Errorf("MakePicks: failed to get season %d: %w", ctx.Season, err)
	}

	week, weekRef, err := firestore.GetWeek(ctx, seasonRef, ctx.Week)
	if err != nil {
		return fmt.Errorf("MakePicks: failed to get week %d of season %d: %w", ctx.Week, season.Year, err)
	}

	// If making picks, eliminate the picks from next week's data
	_, nextWeekRef, err := firestore.GetWeek(ctx, seasonRef, week.Number+1)
	if err != nil {
		return fmt.Errorf("MakePicks: failed to get following week: %w", err)
	}

	for picker, picks := range ctx.Picks {
		// Make sure the picker is picking this season
		var pickerRef *fs.DocumentRef
		var ok bool
		if pickerRef, ok = season.Pickers[picker]; !ok {
			return fmt.Errorf("MakePicks: picker '%s' is not playing in season %d", picker, season.Year)
		}

		picksArr := strings.Split(picks, ",")
		err = makeStreakPick(ctx, seasonRef, weekRef, nextWeekRef, pickerRef, picksArr)
		if err != nil {
			return fmt.Errorf("MakePicks: unable to make streak pick of teams %v for picker '%s': %w", picks, picker, err)
		}
	}

	return nil
}

var teamRefsByOtherName firestore.TeamRefsByName
var teamsOnce sync.Once

var gameRefsByMatchup firestore.GameRefsByMatchup
var gamesOnce sync.Once

// Delete team with names in `teamNames` from the list of remaining teams in week `weekTo` for picker with short name `pickerName`.
// Return an error if `picker` cannot pick all of the teams in `teamNames` for whatever reason.
func makeStreakPick(ctx *Context, season, weekFrom, weekTo, picker *fs.DocumentRef, teamNames []string) error {

	str, _, err := firestore.GetStreakTeamsRemaining(ctx, season, weekFrom, picker)
	if err != nil {
		return fmt.Errorf("makeStreakPick: unable to get streak teams remaining for picker '%s', week '%s': %w", picker.ID, weekFrom.ID, err)
	}

	// remove empty team names (bye week pick)
	n := 0
	for _, tn := range teamNames {
		if tn == "" {
			continue
		}
		teamNames[n] = tn
		n++
	}
	teamNames = teamNames[:n]

	nPicks := len(teamNames)
	if len(str.PickTypesRemaining) < nPicks+1 || str.PickTypesRemaining[nPicks] <= 0 {
		return fmt.Errorf("makeStreakPick: not enough picks of type %d remaining for picker '%s'", nPicks, picker.ID)
	}
	str.PickTypesRemaining[nPicks]--

	teamsOnce.Do(func() {
		teams, teamRefs, err := firestore.GetTeams(ctx, season)
		if err != nil {
			panic(err)
		}
		teamRefsByOtherName, err = firestore.NewTeamRefsByOtherName(teams, teamRefs)
		if err != nil {
			panic(err)
		}
	})
	gamesOnce.Do(func() {
		games, gameRefs, err := firestore.GetGames(ctx, weekFrom)
		if err != nil {
			panic(err)
		}
		gameRefsByMatchup = firestore.NewGameRefsByMatchup(games, gameRefs)
	})
	for _, teamName := range teamNames {
		var teamRef *fs.DocumentRef
		var ok bool
		if teamRef, ok = teamRefsByOtherName[teamName]; !ok {
			return fmt.Errorf("makeStreakPick: team with other name '%s' not found in season '%s'", teamName, season.ID)
		}
		if _, ok = gameRefsByMatchup.LookupTeam(teamRef.ID); !ok {
			return fmt.Errorf("makeStreakPick: team with other name '%s' not playing in week '%s'", teamName, weekFrom.ID)
		}
		var found bool
		for i, ref := range str.TeamsRemaining {
			if ref.ID == teamRef.ID {
				found = true
				// Delete and free pointers while preserving order
				copy(str.TeamsRemaining[i:], str.TeamsRemaining[i+1:])
				str.TeamsRemaining[len(str.TeamsRemaining)-1] = nil
				str.TeamsRemaining = str.TeamsRemaining[:len(str.TeamsRemaining)-1]
				break
			}
		}
		if !found {
			return fmt.Errorf("makeStreakPick: unable to find team '%s' in remaining teams for picker '%s'", teamRef.ID, picker.ID)
		}
	}

	// Update remaining picks in next week's collection
	col := weekTo.Collection("streak-teams-remaining")
	newRef := col.Doc(picker.ID)
	if ctx.DryRun {
		log.Print("DRY RUN: would write the following to datastore:")
		log.Printf("%s -> %v\n", newRef.Path, str)
		return nil
	}
	if ctx.Force {
		_, err = newRef.Set(ctx, &str)
	} else {
		_, err = newRef.Create(ctx, &str)
	}
	return err
}
