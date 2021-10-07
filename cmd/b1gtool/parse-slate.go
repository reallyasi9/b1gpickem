package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	fs "cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
	"github.com/reallyasi9/b1gpickem/firestore"
	"github.com/tealeg/xlsx"
)

// slateFlagSet is a flag.FlagSet for parsing the parse-slate subcommand.
var slateFlagSet *flag.FlagSet

// slateSeason is the season the slate occurs in.
var slateSeason int

// slateWeek is the week the slate occurs in.
var slateWeek int

// slateUsage is the usage documentation for the parse-slate subcommand.
func slateUsage() {
	fmt.Fprintf(flag.CommandLine.Output(), `Usage: b1gtool [global-flags] parse-slate [flags] <slate>
	
Parse the weekly Pick'Em slate.

Arguments:
  slate string
      File name of slate to parse. To specify a Google Cloud Storage location, specify a URL with a "gs://" scheme.
	
Flags:
`)

	slateFlagSet.PrintDefaults()

	fmt.Fprint(flag.CommandLine.Output(), "\nGlobal Flags:\n")

	flag.PrintDefaults()

}

func init() {
	cmd := "parse-slate"

	slateFlagSet = flag.NewFlagSet(cmd, flag.ExitOnError)
	slateFlagSet.SetOutput(flag.CommandLine.Output())
	slateFlagSet.Usage = slateUsage

	slateFlagSet.IntVar(&slateSeason, "season", -1, "The `season` to which the slate belongs. If negative, the most recent season will be used.")
	slateFlagSet.IntVar(&slateWeek, "week", -1, "The `week` to which the slate belongs. If negative, the week will be calculated based on the season start_time and today's date.")

	Commands[cmd] = parseSlate
	Usage[cmd] = slateUsage
}

func parseSlate() {
	err := slateFlagSet.Parse(flag.Args()[1:])
	if err != nil {
		log.Fatalf("Failed to parse parse-slate arguments: %v", err)
	}
	if slateFlagSet.NArg() == 0 {
		slateFlagSet.Usage()
		log.Fatal("No slate given")
	}
	if slateFlagSet.NArg() > 1 {
		slateFlagSet.Usage()
		log.Fatal("Too many arguments given")
	}
	slateLocation := slateFlagSet.Arg(0)

	ctx := context.Background()
	reader, err := getFileOrGSReader(ctx, slateLocation)
	if err != nil {
		log.Fatalf("Unable to open '%s': %v", slateLocation, err)
	}
	defer reader.Close()

	fsClient, err := fs.NewClient(ctx, ProjectID)
	if err != nil {
		log.Fatalf("Unable to create firestore client: %v", err)
	}

	_, seasonRef, err := firestore.GetSeason(ctx, fsClient, slateSeason)
	if err != nil {
		log.Fatalf("Unable to get season: %v", err)
	}

	_, weekRef, err := firestore.GetWeek(ctx, fsClient, seasonRef, slateWeek)
	if err != nil {
		log.Fatalf("Unable to get week: %v", err)
	}

	games, gameRefs, err := firestore.GetGames(ctx, fsClient, weekRef)
	if err != nil {
		log.Fatalf("Unable to get games: %v", err)
	}

	gl := firestore.NewGameRefsByMatchup(games, gameRefs)

	teams, teamRefs, err := firestore.GetTeams(ctx, fsClient, seasonRef)
	if err != nil {
		log.Fatalf("Unable to get teams: %v", err)
	}

	tlOther := firestore.NewTeamRefsByOtherName(teams, teamRefs)
	tlShort := firestore.NewTeamRefsByShortName(teams, teamRefs)

	slurp, err := io.ReadAll(reader)
	if err != nil {
		log.Fatalf("Unable to read slate file: %v", err)
	}

	sgames, err := parseSheet(slurp, tlOther, tlShort, gl)
	if err != nil {
		log.Fatalf("Unable to parse games from slate file: %v", err)
	}

	if DryRun {
		log.Print("DRY RUN: would write the following to firestore:")
		for _, g := range sgames {
			log.Printf("%s", g)
		}
		return
	}

	ct, err := getCreationTime(ctx, slateLocation)
	if err != nil {
		log.Fatalf("Unable to stat time from file: %v", err)
	}
	slate := firestore.Slate{
		Created:  ct,
		FileName: slateLocation,
	}
	slateID := time.Now().Format(time.UnixDate)
	slateRef := weekRef.Collection("slates").Doc(slateID)
	err = fsClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
		var err error
		if Force {
			err = t.Set(slateRef, &slate)
		} else {
			err = t.Create(slateRef, &slate)
		}
		if err != nil {
			return err
		}

		for _, game := range sgames {
			gameID := game.Game.ID // convenient
			gameRef := slateRef.Collection("games").Doc(gameID)
			if Force {
				err = t.Set(gameRef, &game)
			} else {
				err = t.Create(gameRef, &game)
			}
			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		log.Fatalf("Unable to store slate and games in firestore: %v", err)
	}
}

func getFileOrGSReader(ctx context.Context, f string) (io.ReadCloser, error) {
	u, err := url.Parse(f)
	if err != nil {
		return nil, err
	}
	var r io.ReadCloser
	switch u.Scheme {
	case "gs":
		gsClient, err := storage.NewClient(ctx)
		if err != nil {
			return nil, err
		}
		bucket := gsClient.Bucket(u.Host)
		obj := bucket.Object(u.Path)
		r, err = obj.NewReader(ctx)
		if err != nil {
			return nil, err
		}

	case "file":
		fallthrough
	case "":
		r, err = os.Open(u.Path)
		if err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unable to determine how to open '%s'", f)
	}

	return r, nil
}

// getCreationTime gets the creation time of a file on disk or in Google Storage.
func getCreationTime(ctx context.Context, f string) (time.Time, error) {
	var t time.Time
	u, err := url.Parse(f)
	if err != nil {
		return t, err
	}
	switch u.Scheme {
	case "gs":
		gsClient, err := storage.NewClient(ctx)
		if err != nil {
			return t, err
		}
		bucket := gsClient.Bucket(u.Host)
		obj := bucket.Object(u.Path)
		attrs, err := obj.Attrs(ctx)
		if err != nil {
			return t, err
		}
		t = attrs.Created

	case "file":
		fallthrough
	case "":
		s, err := os.Stat(f)
		if err != nil {
			return t, err
		}
		t = s.ModTime()

	default:
		return t, fmt.Errorf("unable to determine how to stat '%s'", f)
	}

	return t, nil
}

func parseSheet(slurp []byte, tlOther, tlShort firestore.TeamRefsByName, gl firestore.GameRefsByMatchup) ([]firestore.SlateGame, error) {
	xl, err := xlsx.OpenBinary(slurp)
	if err != nil {
		return nil, err
	}

	sheet := xl.Sheets[0]
	log.Printf("Reading sheet name: %s", sheet.Name)

	games := make([]firestore.SlateGame, 0)

	for irow, row := range sheet.Rows {
		for icol, cell := range row.Cells {

			matchup, homeRank, awayRank, gotw, found, err := parseGame(cell.Value, tlShort)
			if err != nil {
				return nil, err
			}
			if found {
				game, swap, wn, ok := gl.LookupCorrectMatchup(matchup)
				if !ok {
					return nil, fmt.Errorf("pick matchup %+v not found", matchup)
				}
				if swap {
					homeRank, awayRank = awayRank, homeRank
					matchup.Home, matchup.Away = matchup.Away, matchup.Home
				}
				sgame := firestore.SlateGame{
					Row:                 irow,
					HomeRank:            homeRank,
					AwayRank:            awayRank,
					GOTW:                gotw,
					Game:                game,
					HomeDisagreement:    swap,
					NeutralDisagreement: wn,
				}
				// check the immediate next column for noise
				if len(row.Cells) != icol+1 {
					favorite, spread, found, err := parseNoisySpread(row.Cells[icol+1].Value, tlShort)
					if err != nil {
						return nil, err
					}
					if found {
						sgame.HomeFavored = favorite == matchup.Home
						if !sgame.HomeFavored {
							spread = -spread
						}
						sgame.NoisySpread = spread
					}
				}

				games = append(games, sgame)

				// There is nothing else to parse in this row
				break
			}

			matchup, favorite, value, found, err := parseDog(cell.Value, tlOther)
			if err != nil {
				return nil, err
			}
			if !found {
				continue
			}
			game, swap, _, ok := gl.LookupCorrectMatchup(matchup)
			if !ok {
				return nil, fmt.Errorf("superdog matchup %+v not found", matchup)
			}
			if swap {
				matchup.Home, matchup.Away = matchup.Away, matchup.Home
			}
			sgame := firestore.SlateGame{
				Row:         irow,
				Game:        game,
				HomeFavored: matchup.Home == favorite,
				Value:       value,
				Superdog:    true,
			}
			games = append(games, sgame)
		}
	}

	return games, nil
}

//^(\*\*)?         # Marker for GOTW
//(?:\#(\d+)\s+)?  # Optional rank for Team 1
//(.*?)\s+         # Team 1 (home) name in LUKE format
//(@|vs)\s+        # Whether or not Team 2 is away (@) or if the game is at a neutral site (vs)
//(?:\#(\d+)\s+)?  # Optional rank for Team 2
//(.*?)            # Team 2 (away) name in LUKE format
//\1?\s*$          # Marker for GOTW
var gameRe = regexp.MustCompile(`^\s*(\*\*)?\s*(?:#\s*(\d+)\s+)?(.*?)\s+((?i:@|at|vs))\s+(?:#\s*(\d+)\s+)?(.*?)(?:\s*\*\*)?\s*$`)

var noiseRe = regexp.MustCompile(`\s*(?i:Enter\s+(.*?)\s+iff\s+you\s+predict\s+.*?\s+wins\s+by\s+at\s+least\s+(\d+)\s+points)`)

var sdRe = regexp.MustCompile(`(?i:\s*(.*?)\s+over\s+(.*?)\s+\(\s*(\d+)\s+points,?\s+if\s+correct\s*\))`)

// parseGame parses game information in Luke's default format
func parseGame(cell string, tl firestore.TeamRefsByName) (matchup firestore.Matchup, homeRank int, awayRank int, gotw bool, found bool, err error) {

	submatches := gameRe.FindStringSubmatch(cell)
	if len(submatches) == 0 {
		return
	}

	found = true

	gotw = submatches[1] == "**"

	if submatches[2] != "" {
		awayRank, err = strconv.Atoi(submatches[2])
		if err != nil {
			err = fmt.Errorf("parseGame: error parsing rank of first team: %w", err)
			return
		}
	}

	var ok bool
	var teamRef *fs.DocumentRef
	name := submatches[3]
	if teamRef, ok = tl[name]; !ok {
		err = fmt.Errorf("parseGame: unable to find team with name '%s' in cell '%s'", name, cell)
		return
	}
	matchup.Away = teamRef.ID

	matchup.Neutral = strings.ToLower(submatches[4]) == "vs"

	if submatches[5] != "" {
		homeRank, err = strconv.Atoi(submatches[5])
		if err != nil {
			err = fmt.Errorf("parseGame: error parsing rank of second team: %w", err)
			return
		}
	}

	name = submatches[6]
	if teamRef, ok = tl[name]; !ok {
		err = fmt.Errorf("parseGame: unable to find team with name '%s' in cell '%s'", name, cell)
		return
	}
	matchup.Home = teamRef.ID

	return
}

// parseNoisySpread parses noisy spread from a cell.
func parseNoisySpread(cell string, tl firestore.TeamRefsByName) (favorite string, spread int, found bool, err error) {
	submatches := noiseRe.FindStringSubmatch(cell)
	if len(submatches) == 0 {
		return
	}

	found = true

	name := submatches[1]
	var teamRef *fs.DocumentRef
	var ok bool
	if teamRef, ok = tl[name]; !ok {
		err = fmt.Errorf("parseNoisySpread: unable to find team with name '%s' in cell '%s'", name, cell)
		return
	}
	favorite = teamRef.ID

	spread, err = strconv.Atoi(submatches[2])
	if err != nil {
		err = fmt.Errorf("parseNoisySpread: error parsing noisy spread value: %w", err)
		return
	}
	return
}

// parseDog parses a superdog game from a cell.
func parseDog(cell string, tl firestore.TeamRefsByName) (matchup firestore.Matchup, favorite string, value int, found bool, err error) {
	submatches := sdRe.FindStringSubmatch(cell)
	if len(submatches) == 0 {
		return
	}

	found = true

	name := submatches[1]
	var teamRef *fs.DocumentRef
	var ok bool
	if teamRef, ok = tl[name]; !ok {
		err = fmt.Errorf("parseDog: unable to find team with name '%s' in cell '%s'", name, cell)
		return
	}
	matchup.Home = teamRef.ID
	favorite = teamRef.ID

	name = submatches[2]
	if teamRef, ok = tl[name]; !ok {
		err = fmt.Errorf("parseDog: unable to find team with name '%s' in cell '%s'", name, cell)
		return
	}
	matchup.Away = teamRef.ID

	value, err = strconv.Atoi(submatches[3])

	if err != nil {
		err = fmt.Errorf("parseDog: error parsing game value: %w", err)
		return
	}

	return
}
