package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"

	"cloud.google.com/go/firestore"
	bpefs "github.com/reallyasi9/b1gpickem/internal/firestore"
)

var teamsSeason int

var teamsFlagSet *flag.FlagSet

func init() {
	teamsFlagSet = flag.NewFlagSet("pick", flag.ExitOnError)
	teamsFlagSet.SetOutput(flag.CommandLine.Output())
	teamsFlagSet.Usage = pickUsage

	teamsFlagSet.IntVar(&teamsSeason, "season", -1, "Season year. Negative values will calculate season based on today's date.")

	Commands["setup-teams"] = setupTeams
	Usage["setup-teams"] = teamsUsage
}

func teamsUsage() {
	w := flag.CommandLine.Output()
	fmt.Fprint(w, `btstool [global-flags] setup-teams [flags] types team [team...]

Instantiate teams and pick types for a season's BTS competition.

Arguments:
  types
      A colon-separated list of numbers of pick types allowed for the season. The first element is the number of bye weeks, the second is the number of single picks, and so on.
  team
      A team (specified by other name) to add to the BTS competition. Multiple iterations are allowed.

Flags:
`)

	teamsFlagSet.PrintDefaults()

	fmt.Fprint(flag.CommandLine.Output(), "\nGlobal Flags:\n")

	flag.PrintDefaults()
}

func setupTeams() {
	ctx := context.Background()

	err := teamsFlagSet.Parse(flag.Args()[1:])
	if err != nil {
		log.Fatalf("Failed to parse setup-teams arguments: %v", err)
	}

	if teamsFlagSet.NArg() < 2 {
		teamsFlagSet.Usage()
		log.Fatal("Pick types and at least one team required")
	}

	pickTypes, err := splitPickTypes(teamsFlagSet.Arg(0))
	if err != nil {
		log.Print(err)
		log.Fatalf("Failed to parse pick types: %v", err)
	}

	fsclient, err := firestore.NewClient(ctx, ProjectID)
	if err != nil {
		log.Print(err)
		log.Fatalf("Check that the project ID \"%s\" is correctly specified (either the -project flag or the GCP_PROJECT environment variable)", ProjectID)
	}

	season, seasonRef, err := bpefs.GetSeason(ctx, fsclient, streakerSeason)
	if err != nil {
		log.Fatal(err)
	}

	teams, teamRefs, err := bpefs.GetTeams(ctx, fsclient, seasonRef)
	if err != nil {
		panic(err)
	}
	teamRefsByOtherName := bpefs.NewTeamRefsByOtherName(teams, teamRefs)

	streakTeams := make([]*firestore.DocumentRef, len(teamsFlagSet.Args()[1:]))
	for i, name := range teamsFlagSet.Args()[1:] {
		var teamRef *firestore.DocumentRef
		var ok bool
		if teamRef, ok = teamRefsByOtherName[name]; !ok {
			log.Fatalf("Team '%s' not found", name)
		}
		streakTeams[i] = teamRef
	}

	if DryRun {
		log.Print("DRY RUN: would write the following to Firestore:")
		log.Printf("StreakTeams: %v", streakTeams)
		log.Printf("StreakPickTypes: %v", pickTypes)
		return
	}
	if len(season.StreakTeams) > 0 && !Force {
		log.Fatal("Refusing to overwrite streak teams remaining: pass -force flag to override this behavior")
	}
	season.StreakTeams = streakTeams
	season.StreakPickTypes = pickTypes
	_, err = seasonRef.Set(ctx, &season)
	if err != nil {
		log.Fatal(err)
	}
}

func splitPickTypes(arg string) ([]int, error) {
	values := strings.Split(arg, ":")
	out := make([]int, len(values))
	for i, val := range values {
		var parsed int
		var err error
		if parsed, err = strconv.Atoi(val); err != nil {
			return nil, err
		}
		out[i] = parsed
	}
	return out, nil
}
