package bts

import (
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/firestore"

	bpefs "github.com/reallyasi9/b1gpickem/internal/firestore"
)

// Schedule is a team's schedule for the year.
type Schedule map[Team][]*Game

// MakeSchedule builds a schedule from the games in Firestore.
// The schedule will only include games from the given `week` onward (inclusive), and only for the given `teams`.
// If a `team` does not have a game in a given week, a BYE will be inserted.
func MakeSchedule(ctx context.Context, client *firestore.Client, season *firestore.DocumentRef, week int, teams []*firestore.DocumentRef) (schedule Schedule, err error) {
	weeks, err := season.Collection(bpefs.WEEKS_COLLECTION).Where("number", ">=", week).OrderBy("number", firestore.Asc).Documents(ctx).GetAll()
	if err != nil {
		return
	}

	schedule = make(Schedule)
	teamMap := make(map[string]struct{})
	for _, team := range teams {
		schedule[Team(team.ID)] = make([]*Game, 0)
		teamMap[team.ID] = struct{}{}
	}

	for _, weekSnap := range weeks {
		// Search through games in each week for a matching team.
		games, _, e := bpefs.GetGames(ctx, weekSnap.Ref)
		if e != nil {
			err = e
			return
		}
		anyGames := false
		// build a set of bye games for this week as default
		allGames := make(map[string]*Game)
		for t := range teamMap {
			allGames[t] = NewGame(Team(t), BYE, Neutral)
		}
		for _, game := range games {
			if _, ok := teamMap[game.HomeTeam.ID]; ok {
				var loc RelativeLocation
				if !game.NeutralSite {
					loc = Home
				}
				g := NewGame(Team(game.HomeTeam.ID), Team(game.AwayTeam.ID), loc)
				allGames[game.HomeTeam.ID] = g
				anyGames = true
			}
			if _, ok := teamMap[game.AwayTeam.ID]; ok {
				var loc RelativeLocation
				if !game.NeutralSite {
					loc = Away
				}
				g := NewGame(Team(game.AwayTeam.ID), Team(game.HomeTeam.ID), loc)
				allGames[game.AwayTeam.ID] = g
				anyGames = true
			}
		}

		if anyGames {
			// At least one game: count it!
			for t, g := range allGames {
				schedule[Team(t)] = append(schedule[Team(t)], g)
			}
		}
	}

	return
}

type weekTeam struct {
	week int
	team Team
}

// UniqueGames filters a schedule to the unique games.
// If two teams (the highest level of sorting of the schedule) play each other, only one of those games is kept.
func (s Schedule) UniqueGames() []*Game {
	gamesSeen := make(map[weekTeam]*Game)
	for team, weeks := range s {
		for week, game := range weeks {
			opponent := weekTeam{week: week, team: game.team2}
			if _, ok := gamesSeen[opponent]; !ok {
				me := weekTeam{week: week, team: team}
				gamesSeen[me] = game
			}
		}
	}

	games := make([]*Game, 0, len(gamesSeen))
	for _, game := range gamesSeen {
		games = append(games, game)
	}

	return games
}

// Get a game for a team and week number.
func (s Schedule) Get(t Team, w int) *Game {
	if t == NONE {
		// Picking no team is strange
		return &NULLGAME
	}
	return s[t][w]
}

// NumWeeks returns the number of weeks contained in the schedule.
func (s Schedule) NumWeeks() int {
	for _, v := range s {
		return len(v)
	}
	return 0
}

// FilterWeeks filters the Predictions by removing weeks prior to the given one.
func (s *Schedule) FilterWeeks(w int) {
	if w <= 0 {
		return
	}
	for team := range *s {
		(*s)[team] = (*s)[team][w:]
	}
}

// TeamList generates a list of first-level teams from the schedule.
func (s Schedule) TeamList() TeamList {
	tl := make(TeamList, len(s))
	i := 0
	for t := range s {
		tl[i] = t
		i++
	}
	return tl
}

func (s Schedule) String() string {
	tl := s.TeamList()
	nW := s.NumWeeks()
	var b strings.Builder

	b.WriteString("      ")
	for week := 0; week < nW; week++ {
		b.WriteString(fmt.Sprintf("%-5d ", week))
	}
	b.WriteString("\n")

	for _, team := range tl {
		b.WriteString(fmt.Sprintf("%4s: ", team))
		for week := 0; week < nW; week++ {
			g := s.Get(team, week)
			extra := ' '
			switch g.LocationRelativeToTeam(0) {
			case Away:
				extra = '@'
			case Far:
				extra = '>'
			case Near:
				extra = '<'
			case Neutral:
				extra = '!'
			}
			if g.Team(1) != BYE {
				b.WriteRune(extra)
			} else {
				b.WriteRune(' ')
			}
			b.WriteString(fmt.Sprintf("%-4s ", g.Team(1)))
		}
		b.WriteString("\n")
	}

	return b.String()
}
