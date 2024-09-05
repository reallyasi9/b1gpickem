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
func MakeSchedule(ctx context.Context, season *firestore.DocumentRef, week int, teams []*firestore.DocumentRef) (schedule Schedule, err error) {
	weeks, err := season.Collection(bpefs.WEEKS_COLLECTION).Where("number", ">=", week).OrderBy("number", firestore.Asc).Documents(ctx).GetAll()
	if err != nil {
		return
	}

	schedule = make(Schedule)
	teamLookup := make(map[string]Team)
	for _, team := range teams {
		t := Team(team.ID)
		schedule[t] = make([]*Game, len(weeks))
		// default to all BYE weeks
		byeGame := NewGame(t, BYE, Neutral)
		for iwk := 0; iwk < len(weeks); iwk++ {
			schedule[t][iwk] = byeGame
		}
		teamLookup[team.ID] = t
	}

	for iwk, weekSnap := range weeks {
		// Search through games in each week for a matching team.
		games, _, e := bpefs.GetGames(ctx, weekSnap.Ref)
		if e != nil {
			err = e
			return
		}

		for _, game := range games {
			var g *Game
			if t, ok := teamLookup[game.HomeTeam.ID]; ok {
				var loc RelativeLocation
				if !game.NeutralSite {
					loc = Home
				}
				g = NewGame(t, Team(game.AwayTeam.ID), loc)
				schedule[t][iwk] = g
			}
			if t, ok := teamLookup[game.AwayTeam.ID]; ok {
				if g == nil {
					var loc RelativeLocation
					if !game.NeutralSite {
						loc = Away
					}
					g = NewGame(t, Team(game.HomeTeam.ID), loc)
				}
				schedule[t][iwk] = g
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
	for _, weeks := range s {
		for week, game := range weeks {
			// Bye games do not count
			if game.team1 == BYE || game.team2 == BYE {
				continue
			}
			me := weekTeam{week: week, team: game.team1}
			opponent := weekTeam{week: week, team: game.team2}
			_, okMe := gamesSeen[me]
			_, okOp := gamesSeen[opponent]
			if !okMe && !okOp {
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
			thisTeam := 0
			if g.team2 == team {
				thisTeam = 1
			}
			extra := ' '
			switch g.LocationRelativeToTeam(thisTeam) {
			case Away:
				extra = '@'
			case Far:
				extra = '>'
			case Near:
				extra = '<'
			case Neutral:
				extra = '!'
			}
			if g.Team(1-thisTeam) != BYE {
				b.WriteRune(extra)
			} else {
				b.WriteRune(' ')
			}
			b.WriteString(fmt.Sprintf("%-4s ", g.Team(1-thisTeam)))
		}
		b.WriteString("\n")
	}

	return b.String()
}
