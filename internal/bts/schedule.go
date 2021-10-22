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
	sweeks, err := season.Collection("weeks").Documents(ctx).GetAll()
	if err != nil {
		return
	}

	weeks := make(map[int]*firestore.DocumentRef)
	lastWeek := -1
	for _, sweek := range sweeks {
		n, e := sweek.DataAt("number")
		if e != nil {
			err = e
			return
		}
		nint := int(n.(int64))
		if nint < week {
			continue
		}
		if nint > lastWeek {
			lastWeek = nint
		}
		weeks[nint] = sweek.Ref
	}

	lastWeek++
	schedule = make(Schedule)
	teamMap := make(map[string]struct{})
	for _, team := range teams {
		allGames := make([]*Game, lastWeek-week)
		// fill them with byes first, because some weeks are absent
		for i := range allGames {
			allGames[i] = NewGame(Team(team.ID), BYE, Neutral)
		}
		schedule[Team(team.ID)] = allGames
		teamMap[team.ID] = struct{}{}
	}

	for iWeek, weekRef := range weeks {
		i := iWeek - week
		games, _, e := bpefs.GetGames(ctx, client, weekRef)
		if e != nil {
			err = e
			return
		}
		for _, game := range games {
			if _, ok := teamMap[game.HomeTeam.ID]; ok {
				var loc RelativeLocation
				if !game.NeutralSite {
					loc = Home
				}
				g := NewGame(Team(game.HomeTeam.ID), Team(game.AwayTeam.ID), loc)
				schedule[Team(game.HomeTeam.ID)][i] = g
			}
			if _, ok := teamMap[game.AwayTeam.ID]; ok {
				var loc RelativeLocation
				if !game.NeutralSite {
					loc = Away
				}
				g := NewGame(Team(game.AwayTeam.ID), Team(game.HomeTeam.ID), loc)
				schedule[Team(game.AwayTeam.ID)][i] = g
			}
		}
	}

	return
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

func splitLocTeam(locTeam string) (RelativeLocation, Team) {
	if locTeam == "BYE" || locTeam == "" {
		return Neutral, BYE
	}
	// Note: this is relative to the schedule team, not the team given here.
	switch locTeam[0] {
	case '@':
		return Away, Team(locTeam[1:])
	case '>':
		return Far, Team(locTeam[1:])
	case '<':
		return Near, Team(locTeam[1:])
	case '!':
		return Neutral, Team(locTeam[1:])
	default:
		return Home, Team(locTeam)
	}
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
