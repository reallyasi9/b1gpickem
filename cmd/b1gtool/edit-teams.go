package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"strings"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

// editTeamsFlagSet is a flag.FlagSet for parsing the edit-teams subcommand.
var editTeamsFlagSet *flag.FlagSet

// addShortName implements the flag.Value interface
type addShortName struct {
	teams *[]firestore.Team
}

func (a addShortName) String() string {
	if a.teams == nil {
		return "nil"
	}
	if len(*a.teams) == 0 {
		return "none"
	}
	s := []string{}
	for _, v := range *a.teams {
		s = append(s, fmt.Sprintf("%s %v", v.School, v.ShortNames))
	}
	return strings.Join(s, ",")
}

func (a addShortName) Set(val string) error {
	if val == "" {
		return errors.New("must specify team in ABBR:short_name format")
	}
	splits := strings.Split(val, ":")
	if len(splits) != 2 {
		return fmt.Errorf("unexpected number of fields for team: expected 2, got %d", len(splits))
	}
	abbr := splits[0]
	shortName := splits[1]
	if a.teams == nil {
		*a.teams = make([]firestore.Team, 0, 1)
	}
	t := firestore.Team{
		Abbreviation: abbr,
		ShortNames:   []string{shortName},
	}
	*a.teams = append(*a.teams, t)
	return nil
}

var shortNamesToAdd []firestore.Team

// editTeamsSeason is the season in which the teams should be edited.
var editTeamsSeason int

// editTeamsUsage is the usage documentation for the edit-teams subcommand.
func editTeamsUsage() {
	fmt.Fprintf(flag.CommandLine.Output(), `Usage: b1gtool [global-flags] edit-teams [flags]
	
Edit teams.
	
Flags:
`)

	editTeamsFlagSet.PrintDefaults()

	fmt.Fprint(flag.CommandLine.Output(), "\nGlobal Flags:\n")

	flag.PrintDefaults()

}

func init() {
	editTeamsFlagSet = flag.NewFlagSet("edit-teams", flag.ExitOnError)
	editTeamsFlagSet.SetOutput(flag.CommandLine.Output())
	editTeamsFlagSet.Usage = editTeamsUsage

	editTeamsFlagSet.Var(&addShortName{&shortNamesToAdd}, "shortname", "Edit team's `shortname` by specifying ABBR:short_name. Flag can be specified multiple times.")
	editTeamsFlagSet.IntVar(&editTeamsSeason, "season", -1, "Choose a `season` in which to edit teams. If negative, the most recent season will be used.")

	Commands["edit-teams"] = editTeams
	Usage["edit-teams"] = editTeamsUsage
}

func editTeams() {
	err := editTeamsFlagSet.Parse(flag.Args()[1:])
	if err != nil {
		log.Fatalf("Failed to parse edit-teams arguments: %v", err)
	}

	ctx := context.Background()
	fsClient, err := fs.NewClient(ctx, ProjectID)
	if err != nil {
		log.Fatalf("Unable to create firestore client: %v", err)
	}

	if err := shortNameAdd(ctx, fsClient, shortNamesToAdd); err != nil {
		log.Fatalf("Unable to add team short names: %v", err)
	}
}

func shortNameAdd(ctx context.Context, fsClient *fs.Client, snTeams []firestore.Team) error {
	_, seasonRef, err := firestore.GetSeason(ctx, fsClient, editTeamsSeason)
	if err != nil {
		return err
	}

	teams, teamRefs, err := firestore.GetTeams(ctx, seasonRef)
	if err != nil {
		return err
	}

	type TeamRef struct {
		Team firestore.Team
		Ref  *fs.DocumentRef
	}

	teamsByAbbr := make(map[string]TeamRef)
	for i, team := range teams {
		teamsByAbbr[team.Abbreviation] = TeamRef{Team: team, Ref: teamRefs[i]}
	}

	writeMe := make([]TeamRef, 0)
	for _, snTeam := range snTeams {
		if team, ok := teamsByAbbr[snTeam.Abbreviation]; ok {
			team.Team.ShortNames = append(team.Team.ShortNames, snTeam.ShortNames...)
			writeMe = append(writeMe, team)
			continue
		}
		return fmt.Errorf("team abbreviation '%s' not recognized", snTeam.Abbreviation)
	}

	if DryRun {
		log.Print("DRY RUN: would write the following:")
		for _, tr := range writeMe {
			log.Printf("%s -> %s", tr.Ref.Path, tr.Team)
		}
		return nil
	}

	if !Force {
		log.Printf("Refusing to write: because shortname edit involves overwriting data in Firestore, -force flag is required to make modifications.")
		return nil
	}

	err = fsClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
		for _, tr := range writeMe {
			err = t.Set(tr.Ref, &tr.Team)
			if err != nil {
				return err
			}
		}
		return nil
	})

	return err
}
