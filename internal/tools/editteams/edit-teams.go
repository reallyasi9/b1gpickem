package editteams

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

const COMMAND = "edit-teams"

var force bool
var dryrun bool
var project string

// FlagSet is a flag.FlagSet for parsing the edit-teams subcommand.
var FlagSet *flag.FlagSet

// idShortName is an ID:short_name combination
type idShortName struct {
	ID        string
	ShortName string
}

// addShortName implements the flag.Value interface
type addShortName struct {
	teams *[]idShortName
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
		s = append(s, fmt.Sprintf("%s:%v", v.ID, v.ShortName))
	}
	return strings.Join(s, " ")
}

func (a addShortName) Set(val string) error {
	if val == "" {
		return errors.New("must specify team in ID:short_name format")
	}
	splits := strings.Split(val, ":")
	if len(splits) != 2 {
		return fmt.Errorf("unexpected number of fields for team: expected 2, got %d", len(splits))
	}
	id := splits[0]
	shortName := splits[1]
	if a.teams == nil {
		*a.teams = make([]idShortName, 0, 1)
	}
	t := idShortName{
		ID:        id,
		ShortName: shortName,
	}
	*a.teams = append(*a.teams, t)
	return nil
}

var shortNamesToAdd []idShortName

// Usage is the usage documentation for the edit-teams subcommand.
func Usage() {
	fmt.Fprintf(flag.CommandLine.Output(), `Usage: %s [global-flags] %s <season> [flags]
	
Edit teams.
	
Flags:
`, flag.CommandLine.Name(), COMMAND)

	FlagSet.PrintDefaults()

	fmt.Fprint(flag.CommandLine.Output(), "\nGlobal Flags:\n")

	flag.PrintDefaults()

}

func InitializeSubcommand() {
	FlagSet = flag.NewFlagSet(COMMAND, flag.ExitOnError)
	FlagSet.SetOutput(flag.CommandLine.Output())
	FlagSet.Usage = Usage

	FlagSet.BoolVar(&force, "force", false, "Force overwrite of data.")
	FlagSet.BoolVar(&dryrun, "dryrun", false, "Perform dry run: print intended actions to the log, but do not modify any data.")
	FlagSet.StringVar(&project, "project", os.Getenv("GCP_PROJECT"), "GCP Project ID. Defaults to the environment variable GCP_PROJECT, if set.")

	FlagSet.Var(&addShortName{&shortNamesToAdd}, "add-short-name", "Add a short name to a team. Multiple values are allowed. Values to be specified as a string in ID:short_name format.")
}

func EditTeams() {
	err := FlagSet.Parse(flag.Args()[1:])
	if err != nil {
		log.Fatalf("Failed to parse edit-teams arguments: %v", err)
	}

	season, err := strconv.Atoi(FlagSet.Arg(0))
	if err != nil {
		log.Fatalf("Failed to parse season argument: %v", err)
	}

	ctx := context.Background()
	fsClient, err := fs.NewClient(ctx, project)
	if err != nil {
		log.Fatalf("Unable to create firestore client: %v", err)
	}

	if err := shortNameAdd(ctx, fsClient, season, shortNamesToAdd); err != nil {
		log.Fatalf("Unable to add team short names: %v", err)
	}
}

func appendDistinct(v []string, s ...string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(v)+len(s))
	for _, a := range v {
		if _, ok := seen[a]; ok {
			continue
		}
		seen[a] = struct{}{}
		out = append(out, a)
	}
	for _, a := range s {
		if _, ok := seen[a]; ok {
			continue
		}
		seen[a] = struct{}{}
		out = append(out, a)
	}
	return out
}

func shortNameAdd(ctx context.Context, fsClient *fs.Client, season int, snTeams []idShortName) error {
	_, seasonRef, err := firestore.GetSeason(ctx, fsClient, season)
	if err != nil {
		return err
	}

	writeMe := make(map[string]firestore.Team, 0)
	for _, snTeam := range snTeams {
		var team firestore.Team
		var ok bool
		if team, ok = writeMe[snTeam.ID]; !ok {
			teamRef := seasonRef.Collection(firestore.TEAMS_COLLECTION).Doc(snTeam.ID)
			snap, err := teamRef.Get(ctx)
			if err != nil {
				return fmt.Errorf("shortNameAdd: unable to get team from Firestore: %w", err)
			}
			err = snap.DataTo(&team)
			if err != nil {
				return fmt.Errorf("shortNameAdd: unable to create Team object from Firestore data: %w", err)
			}
		}
		team.ShortNames = appendDistinct(team.ShortNames, snTeam.ShortName)
		writeMe[snTeam.ID] = team
	}

	if dryrun {
		log.Print("DRY RUN: would write the following:")
		for id, tr := range writeMe {
			ref := seasonRef.Collection(firestore.TEAMS_COLLECTION).Doc(id)
			log.Printf("%s -> %s", ref.Path, tr)
		}
		return nil
	}

	if !force {
		log.Printf("Refusing to write: because shortname edit involves overwriting data in Firestore, -force flag is required to make modifications.")
		return nil
	}

	err = fsClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
		for id, tr := range writeMe {
			ref := seasonRef.Collection(firestore.TEAMS_COLLECTION).Doc(id)
			err = t.Set(ref, &tr)
			if err != nil {
				return err
			}
		}
		return nil
	})

	return err
}
