package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"strings"
	"sync"

	"cloud.google.com/go/firestore"
	bpefs "github.com/reallyasi9/b1gpickem/internal/firestore"
)

var streakerWeek int
var streakerSeason int

// PastPicks is a mapping of pickers to the pick(s) they made in a given week.
type PastPicks struct {
	picks *map[string][]string
}

// String implements the flag.Value interface.
func (p PastPicks) String() string {
	if p.picks == nil {
		return "nil"
	}
	if len(*p.picks) == 0 {
		return "none"
	}
	s := []string{}
	for picker, picks := range *p.picks {
		for _, pick := range picks {
			s = append(s, fmt.Sprintf("%s:%s", picker, pick))
		}
	}
	return strings.Join(s, ",")
}

// Set implements the flag.Value interface.
func (p PastPicks) Set(val string) error {
	if val == "" {
		return errors.New("must specify streak pick in short_name:team format")
	}
	splits := strings.Split(val, ":")
	if len(splits) != 2 {
		return fmt.Errorf("streak pick has the wrong number of colon-separated fields: expected 2, got %d", len(splits))
	}
	pickerName := splits[0]
	teamName := splits[1]
	if p.picks == nil {
		*p.picks = make(map[string][]string)
	}
	var picks []string
	var ok bool
	if picks, ok = (*p.picks)[pickerName]; !ok {
		picks = make([]string, 0, 1)
	}
	// Empty picks are okay: those are bye weeks.
	if teamName != "" {
		picks = append(picks, teamName)
	}
	(*p.picks)[pickerName] = picks
	return nil
}

var pickFlagSet *flag.FlagSet

func init() {
	pickFlagSet = flag.NewFlagSet("pick", flag.ExitOnError)
	pickFlagSet.SetOutput(flag.CommandLine.Output())
	pickFlagSet.Usage = pickUsage

	pickFlagSet.IntVar(&streakerSeason, "season", -1, "Season year. Negative values will calculate season based on today's date.")
	pickFlagSet.IntVar(&streakerWeek, "week", -1, "Week number. Negative values will calculate week number based on today's date.")
	// streakerFlagSet.Var(&PastPicks{&streakPicks}, "pick", "Make pick of a given team for a given picker in `picker:team` format. Flag can be specified multiple times.")

	Commands["pick"] = pick
	Usage["pick"] = pickUsage
}

func pickUsage() {
	w := flag.CommandLine.Output()
	fmt.Fprint(w, `btstool [global-flags] pick [flags] streaker:team [streaker:team...]

Make streak picks for streakers.

Arguments:
  streaker:team
      A team (specified by other name) that the streaker (specified by short name) picked for the week given by the -week flag. Streaker and team fields are separated by a colon. Multiple iterations are allowed. If the team name is empty, a streak bye week will be used.

Flags:
`)

	pickFlagSet.PrintDefaults()

	fmt.Fprint(flag.CommandLine.Output(), "\nGlobal Flags:\n")

	flag.PrintDefaults()
}

func pick() {
	ctx := context.Background()

	err := pickFlagSet.Parse(flag.Args()[1:])
	if err != nil {
		log.Fatalf("Failed to parse pick arguments: %v", err)
	}

	if pickFlagSet.NArg() < 1 {
		pickFlagSet.Usage()
		log.Fatal("No picks supplied")
	}

	streakPicks := make(map[string][]string)
	for _, arg := range pickFlagSet.Args() {
		err := PastPicks{&streakPicks}.Set(arg)
		if err != nil {
			log.Fatalf("Unable to parse pick string '%s': %v", arg, err)
		}
	}

	fsclient, err := firestore.NewClient(ctx, ProjectID)
	if err != nil {
		log.Print(err)
		log.Fatalf("Check that the project ID \"%s\" is correctly specified (either the -project flag or the GCP_PROJECT environment variable)", ProjectID)
	}

	_, seasonRef, err := bpefs.GetSeason(ctx, fsclient, streakerSeason)
	if err != nil {
		log.Fatal(err)
	}

	week, weekRef, err := bpefs.GetWeek(ctx, fsclient, seasonRef, streakerWeek)
	if err != nil {
		log.Fatal(err)
	}

	// If making picks, eliminate the picks from next week's data
	var nextWeekRef *firestore.DocumentRef
	if len(streakPicks) > 0 {
		_, nextWeekRef, err = bpefs.GetWeek(ctx, fsclient, seasonRef, week.Number+1)
		if err != nil {
			log.Fatal(err)
		}
	}

	for picker, picks := range streakPicks {
		err = makeStreakPick(ctx, fsclient, seasonRef, weekRef, nextWeekRef, picker, picks)
		if err != nil {
			log.Fatalf("Unable to make streak pick of teams %v for picker '%s': %v", picks, picker, err)
		}
	}

}

var teamRefsByOtherName bpefs.TeamRefsByName
var teamsOnce sync.Once

// Delete team with names in `teamNames` from the list of remaining teams for picker with short name `pickerName`.
func makeStreakPick(ctx context.Context, client *firestore.Client, season, weekFrom, weekTo *firestore.DocumentRef, pickerName string, teamNames []string) error {
	_, pickerRef, err := bpefs.GetPickerByLukeName(ctx, client, pickerName)
	if err != nil {
		return fmt.Errorf("unable to get picker with short name '%s': %w", pickerName, err)
	}

	str, _, err := bpefs.GetStreakTeamsRemaining(ctx, client, weekFrom, pickerRef)
	if err != nil {
		return fmt.Errorf("unable to get streak teams remaining for picker with short name '%s', week '%s': %w", pickerName, weekFrom.ID, err)
	}

	nPicks := len(teamNames)
	if len(str.PickTypesRemaining) < nPicks+1 || str.PickTypesRemaining[nPicks] <= 0 {
		return fmt.Errorf("not enough picks of type %d remaining for picker with short name '%s'", nPicks, pickerName)
	}
	str.PickTypesRemaining[nPicks]--

	teamsOnce.Do(func() {
		teams, teamRefs, err := bpefs.GetTeams(ctx, client, season)
		if err != nil {
			panic(err)
		}
		teamRefsByOtherName = bpefs.NewTeamRefsByOtherName(teams, teamRefs)
	})
	for _, teamName := range teamNames {
		var teamRef *firestore.DocumentRef
		var ok bool
		if teamRef, ok = teamRefsByOtherName[teamName]; !ok {
			return fmt.Errorf("team with other name '%s' not found in season '%s'", teamName, season.ID)
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
			return fmt.Errorf("unable to find team '%s' in remaining teams for picker '%s'", teamRef.ID, pickerName)
		}
	}

	// Update remaining picks in next week's collection
	col := weekTo.Collection("streak_teams_remaining")
	newRef := col.Doc(pickerRef.ID)
	if DryRun {
		log.Print("DRY RUN: would write the following to Firestore:")
		log.Printf("%s -> %v\n", newRef.Path, str)
		return nil
	}
	if Force {
		_, err = newRef.Set(ctx, &str)
	} else {
		_, err = newRef.Create(ctx, &str)
	}
	return err
}
