package pickem

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

var FlagSet *flag.FlagSet

const COMMAND = "pickem"

var force bool
var dryrun bool
var project string

func InitializeSubcommand() {
	FlagSet = flag.NewFlagSet(COMMAND, flag.ExitOnError)
	FlagSet.SetOutput(flag.CommandLine.Output())
	FlagSet.Usage = Usage

	FlagSet.BoolVar(&force, "force", false, "Force overwrite of data.")
	FlagSet.BoolVar(&dryrun, "dryrun", false, "Perform dry run: print intended actions to the log, but do not modify any data.")
	FlagSet.StringVar(&project, "project", os.Getenv("GCP_PROJECT"), "GCP Project ID. Defaults to the environment variable GCP_PROJECT, if set.")
}

func Usage() {
	fmt.Fprintf(flag.CommandLine.Output(), `Usage: %s [global-flags] %s [flags] <season> <week> <picker> [pick [pick...]]
	
Record picks.

Arguments:
  season int
      Season (year) of slate. If negative, the current year will be used.
  week int
      Week number of slate. If negative, the week number will be calculated based on today's date.
  picker string
      Short name of picker.
  pick string
      Short name of picked team. Must correspond to a team on the slate in the given season and week.
	
Flags:
`, flag.CommandLine.Name(), COMMAND)

	FlagSet.PrintDefaults()

	fmt.Fprint(flag.CommandLine.Output(), "\nGlobal Flags:\n")

	flag.PrintDefaults()
}

func Pickem() {
	err := FlagSet.Parse(flag.Args()[1:])
	if err != nil {
		log.Fatalf("Failed to parse arguments: %v", err)
	}
	if FlagSet.NArg() < 3 {
		FlagSet.Usage()
		log.Fatal("Season, week, and picker parameters required.")
	}
	if project == "" {
		log.Print("No project ID. This will probably fail. Supply a project ID with the -project flag or the GCP_PROJECT environment variable.")
	}

	ctx := context.Background()
	fsClient, err := fs.NewClient(ctx, project)
	if err != nil {
		log.Fatalf("Unable to create firestore client: %v", err)
	}

	seasonInt, err := strconv.Atoi(FlagSet.Arg(0))
	if err != nil {
		log.Fatalf("Unable to parse season '%s': %v", FlagSet.Arg(0), err)
	}
	weekInt, err := strconv.Atoi(FlagSet.Arg(1))
	if err != nil {
		log.Fatalf("Unable to parse week '%s': %v", FlagSet.Arg(1), err)
	}

	_, seasonRef, err := firestore.GetSeason(ctx, fsClient, seasonInt)
	if err != nil {
		log.Fatalf("Unable to get season: %v", err)
	}
	_, weekRef, err := firestore.GetWeek(ctx, seasonRef, weekInt)
	if err != nil {
		log.Fatalf("Unable to get week: %v", err)
	}

	pickerName := FlagSet.Arg(2)
	_, pickerRef, err := firestore.GetPickerByLukeName(ctx, fsClient, pickerName)
	if err != nil {
		log.Fatalf("Unable to get picker '%s': %v", pickerName, err)
	}

	slateGames, gameRefs, err := firestore.GetSlateGames(ctx, weekRef)
	if err != nil {
		log.Fatalf("Unable to get slate games: %v", err)
	}

	gameLookup, err := newSlateGamesByTeam(ctx, slateGames, gameRefs)
	if err != nil {
		log.Fatalf("Unable to build slate game lookup: %v", err)
	}

	picks, pickRefs, err := firestore.GetPicks(ctx, weekRef, pickerRef)
	if err != nil {
		log.Fatalf("Unable to get picks for picker '%s': %v", pickerName, err)
	}

	pickLookup := newPicksByGameID(picks, pickRefs)

	teams, teamRefs, err := firestore.GetTeams(ctx, seasonRef)
	if err != nil {
		log.Fatalf("Unable to get teams: %v", err)
	}

	teamLookup := firestore.NewTeamRefsByOtherName(teams, teamRefs)

	picksToUpdate := make(map[string]firestore.Pick)
	newPicks := make([]firestore.Pick, 0)

	for _, pickedTeam := range FlagSet.Args()[3:] {
		_, gameRef, ok := gameLookup.Lookup(pickedTeam)
		if !ok {
			log.Fatalf("Team '%s' not found in slate games", pickedTeam)
		}
		pickedTeamRef, ok := teamLookup[pickedTeam]
		if !ok {
			log.Panicf("Team '%s' not found in teams", pickedTeam)
		}
		pick, pickRef, ok := pickLookup.Lookup(gameRef.ID)
		pick.PickedTeam = pickedTeamRef
		if !ok {
			pick.Picker = pickerRef
			pick.SlateGame = gameRef
			newPicks = append(newPicks, pick)
		} else {
			pick.ModelPrediction = nil
			pick.PredictedProbability = 0.
			pick.PredictedSpread = 0.
			picksToUpdate[pickRef.ID] = pick
		}
		log.Printf("Picked %s for picker %s in game %s", pickedTeam, pickerName, gameRef.ID)
	}

	if dryrun {
		log.Printf("DRY RUN: would create %d new picks", len(newPicks))
		for _, pick := range newPicks {
			log.Print(pick)
		}
		log.Printf("DRY RUN: would update %d previously-made picks", len(picksToUpdate))
		for id, pick := range picksToUpdate {
			log.Printf("%s -> %s", id, pick)
		}
		return
	}

	if len(picksToUpdate) > 0 && !force {
		log.Fatalf("Refusing to proceed. Need to update %d picks. Add -force flag to force update.", len(picksToUpdate))
	}

	picksCollection := weekRef.Collection(firestore.PICKS_COLLECTION)
	err = fsClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
		for id, pick := range picksToUpdate {
			ref := picksCollection.Doc(id)
			if err := t.Set(ref, &pick); err != nil {
				return err
			}
		}
		for _, pick := range newPicks {
			ref := picksCollection.NewDoc()
			if err := t.Create(ref, &pick); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		log.Panicf("Unable to complete transaction to update picks: %v", err)
	}
}

type slateGamesByTeam struct {
	games       []firestore.SlateGame
	gameRefs    []*fs.DocumentRef
	indexLookup map[string]int
}

func newSlateGamesByTeam(ctx context.Context, games []firestore.SlateGame, refs []*fs.DocumentRef) (*slateGamesByTeam, error) {
	lookup := make(map[string]int)
	for i, sg := range games {
		gr := sg.Game
		snap, err := gr.Get(ctx)
		if err != nil {
			return nil, err
		}

		var game firestore.Game
		err = snap.DataTo(&game)
		if err != nil {
			return nil, err
		}

		snap, err = game.HomeTeam.Get(ctx)
		if err != nil {
			return nil, err
		}

		var team firestore.Team
		err = snap.DataTo(&team)
		if err != nil {
			return nil, err
		}

		for _, name := range team.OtherNames {
			if _, ok := lookup[name]; ok {
				return nil, fmt.Errorf("newSlateGamesByTeam: ambiguous team OtherName '%s'", name)
			}
			lookup[name] = i
		}

		snap, err = game.AwayTeam.Get(ctx)
		if err != nil {
			return nil, err
		}

		err = snap.DataTo(&team)
		if err != nil {
			return nil, err
		}

		for _, name := range team.OtherNames {
			if _, ok := lookup[name]; ok {
				return nil, fmt.Errorf("newSlateGamesByTeam: ambiguous team OtherName '%s'", name)
			}
			lookup[name] = i
		}
	}

	return &slateGamesByTeam{
		games:       games,
		gameRefs:    refs,
		indexLookup: lookup,
	}, nil
}

func (s *slateGamesByTeam) Lookup(team string) (sg firestore.SlateGame, ref *fs.DocumentRef, ok bool) {
	var idx int
	idx, ok = s.indexLookup[team]
	if !ok {
		return
	}
	sg = s.games[idx]
	ref = s.gameRefs[idx]
	return
}

type picksByGameID struct {
	picks       []firestore.Pick
	pickRefs    []*fs.DocumentRef
	indexLookup map[string]int
}

func newPicksByGameID(picks []firestore.Pick, refs []*fs.DocumentRef) *picksByGameID {
	lookup := make(map[string]int)
	for i, p := range picks {
		lookup[p.SlateGame.ID] = i
	}
	return &picksByGameID{
		picks:       picks,
		pickRefs:    refs,
		indexLookup: lookup,
	}
}

func (p *picksByGameID) Lookup(id string) (pick firestore.Pick, ref *fs.DocumentRef, ok bool) {
	var idx int
	idx, ok = p.indexLookup[id]
	if !ok {
		return
	}
	pick = p.picks[idx]
	ref = p.pickRefs[idx]
	return
}
