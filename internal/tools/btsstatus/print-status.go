package btsstatus

import (
	"fmt"
	"sort"
	"strings"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

type WeekNumberPicks struct {
	WeekNumber int
	Picks      []string
}

type StreakerNamePicksStreakOver struct {
	StreakerName string
	Picks        []WeekNumberPicks
	StreakOver   bool
}

type byStreakerName []StreakerNamePicksStreakOver

func (a byStreakerName) Len() int { return len(a) }
func (a byStreakerName) Less(i, j int) bool {
	if a[i].StreakOver == a[j].StreakOver {
		if len(a[i].Picks) == len(a[j].Picks) {
			return a[i].StreakerName < a[j].StreakerName
		}
		return len(a[i].Picks) > len(a[j].Picks)
	}
	return a[j].StreakOver
}
func (a byStreakerName) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

type byWeekNumber []WeekNumberPicks

func (a byWeekNumber) Len() int           { return len(a) }
func (a byWeekNumber) Less(i, j int) bool { return a[i].WeekNumber < a[j].WeekNumber }
func (a byWeekNumber) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

func (a StreakerNamePicksStreakOver) String() string {
	var sb strings.Builder
	sb.WriteString(a.StreakerName)
	sb.WriteString(": ")
	sort.Sort(byWeekNumber(a.Picks))
	p := make([]string, len(a.Picks))
	for i, pick := range a.Picks {
		p[i] = fmt.Sprintf("%d:%v", pick.WeekNumber, pick.Picks)
	}
	sb.WriteString(strings.Join(p, ","))
	if a.StreakOver {
		sb.WriteString(" (OVER)")
	}
	return sb.String()
}

func PrintStatus(ctx *Context) error {
	season, seasonRef, err := firestore.GetSeason(ctx, ctx.FirestoreClient, ctx.Season)
	if err != nil {
		return fmt.Errorf("PrintStatus: failed to get season %d: %w", ctx.Season, err)
	}
	weekSnaps, err := seasonRef.Collection(firestore.WEEKS_COLLECTION).OrderBy("first_game_start", fs.Asc).Documents(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("PrintStatus: failed to get weeks: %w", err)
	}
	weeks := make([]firestore.Week, len(weekSnaps))
	for i, snap := range weekSnaps {
		var week firestore.Week
		if err = snap.DataTo(&week); err != nil {
			return fmt.Errorf("PrintStatus: failed to convert week '%s': %w", snap.Ref.ID, err)
		}
		weeks[i] = week
	}
	pickers, pickerRefs, err := firestore.GetPickers(ctx, ctx.FirestoreClient)
	if err != nil {
		return fmt.Errorf("PrintStatus: failed to get pickers: %w", err)
	}
	teamNames := make(map[string]string)
	teams, teamRefs, err := firestore.GetTeams(ctx, seasonRef)
	if err != nil {
		return fmt.Errorf("PrintStatus: failed to get teams: %w", err)
	}
	for i, ref := range teamRefs {
		teamNames[ref.ID] = teams[i].School
	}

	pickerStreaks := make([]StreakerNamePicksStreakOver, len(pickers))
	longestStreak := -1
	anyOver := false
	for i, pickerRef := range pickerRefs {
		pickerName := pickers[i].Name
		snpso := StreakerNamePicksStreakOver{
			StreakerName: pickerName,
		}
		// weeks are in forward order, so picked teams can be calculated week by week
		// based on changes from streak teams active in the season
		active := make(map[string]struct{})
		for _, ref := range season.StreakTeams {
			active[ref.ID] = struct{}{}
		}
		lastWeek := -1
		for i, week := range weeks {
			if lastWeek >= 0 {
				remaining := make(map[string]struct{})
				weekSnap := weekSnaps[i]
				strs, _, err := firestore.GetStreakTeamsRemaining(ctx, seasonRef, weekSnap.Ref, pickerRef)
				if err != nil {
					if _, converted := err.(firestore.NoStreakTeamsRemaining); converted {
						continue
					}
					return fmt.Errorf("PrintStatus: failed to get streak teams remaining for season '%s', week '%s', picker '%s': %w", seasonRef.ID, weekSnap.Ref.ID, pickerRef.ID, err)
				}
				if week.Number > longestStreak {
					longestStreak = week.Number
				}
				for _, pick := range strs.TeamsRemaining {
					remaining[pick.ID] = struct{}{}
					delete(active, pick.ID)
				}
				picks := make([]string, 0, len(active))
				for id := range active {
					picks = append(picks, teamNames[id])
				}
				active = remaining
				snpso.Picks = append(snpso.Picks, WeekNumberPicks{WeekNumber: lastWeek, Picks: picks})
			}
			lastWeek = week.Number
		}
		pickerStreaks[i] = snpso
	}

	// set streaks to over if < longest
	snpsos := make([]StreakerNamePicksStreakOver, 0, len(pickerStreaks))
	for _, s := range pickerStreaks {
		if len(s.Picks) < longestStreak && anyOver {
			s.StreakOver = true
		}
		snpsos = append(snpsos, s)
	}

	sort.Sort(byStreakerName(snpsos))

	for _, streak := range snpsos {
		fmt.Println(streak)
	}

	return nil
}
