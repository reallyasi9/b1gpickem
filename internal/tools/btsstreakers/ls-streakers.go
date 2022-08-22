package btsstreakers

import (
	"fmt"
	"sort"

	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

type StreakerNameStreakLengthStreakOver struct {
	StreakerName string
	WeekNumber   int
	StreakOver   bool
}

type byStreakerName []StreakerNameStreakLengthStreakOver

func (a byStreakerName) Len() int { return len(a) }
func (a byStreakerName) Less(i, j int) bool {
	if a[i].StreakOver == a[j].StreakOver {
		if a[i].WeekNumber == a[j].WeekNumber {
			return a[i].StreakerName < a[j].StreakerName
		}
		return a[i].WeekNumber > a[j].WeekNumber
	}
	return a[j].StreakOver
}
func (a byStreakerName) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

func LsStreakers(ctx *Context) error {
	_, seasonRef, err := firestore.GetSeason(ctx, ctx.FirestoreClient, ctx.Season)
	if err != nil {
		return fmt.Errorf("LsStreakers: failed to get season %d: %w", ctx.Season, err)
	}
	weekSnaps, err := seasonRef.Collection(firestore.WEEKS_COLLECTION).Documents(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("LsStreakers: failed to get weeks: %w", err)
	}
	pickerNames := make(map[string]string)
	pickers, pickerRefs, err := firestore.GetPickers(ctx, ctx.FirestoreClient)
	if err != nil {
		return fmt.Errorf("LsStreakers: failed to get pickers: %w", err)
	}
	for i, ref := range pickerRefs {
		pickerNames[ref.ID] = pickers[i].Name
	}
	pickerStreaks := make(map[string]StreakerNameStreakLengthStreakOver)
	lastWeekStreaking := -1
	for _, snap := range weekSnaps {
		var week firestore.Week
		err = snap.DataTo(&week)
		if err != nil {
			return fmt.Errorf("LsStreakers: failed to convert week: %w", err)
		}

		strs, err := snap.Ref.Collection(firestore.STREAK_TEAMS_REMAINING_COLLECTION).Documents(ctx).GetAll()
		if err != nil {
			return fmt.Errorf("LsStreakers: failed to get streak teams remaining for week '%s': %w", snap.Ref.ID, err)
		}

		for _, ssnap := range strs {
			var str firestore.StreakTeamsRemaining
			err = ssnap.DataTo(&str)
			if err != nil {
				return fmt.Errorf("LsStreakers: failed to convert streak teams remaining: %w", err)
			}

			if s, ok := pickerStreaks[str.Picker.ID]; !ok || s.WeekNumber <= week.Number {
				pickerStreaks[str.Picker.ID] = StreakerNameStreakLengthStreakOver{StreakerName: pickerNames[str.Picker.ID], WeekNumber: week.Number}
				if lastWeekStreaking < week.Number {
					lastWeekStreaking = week.Number
				}
			}
		}
	}

	ls := make(byStreakerName, 0, len(pickerStreaks))
	for _, streak := range pickerStreaks {
		if streak.WeekNumber < lastWeekStreaking {
			streak.StreakOver = true
		}
		ls = append(ls, streak)
	}

	sort.Sort(ls)

	for _, streak := range ls {
		fmt.Printf("%s ", streak.StreakerName)
		if streak.StreakOver {
			fmt.Print("OVER ")
		} else {
			fmt.Print("ACTIVE ")
		}
		fmt.Printf("WEEK %d\n", streak.WeekNumber)
	}

	return nil
}
