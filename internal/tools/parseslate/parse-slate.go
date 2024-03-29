package parseslate

import (
	"context"
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
	"github.com/reallyasi9/b1gpickem/internal/firestore"
	"github.com/tealeg/xlsx"
)

func ParseSlate(ctx *Context) error {

	reader, err := getFileOrGSReader(ctx, ctx.Slate)
	if err != nil {
		return fmt.Errorf("ParseSlate: failed to open '%s': %w", ctx.Slate, err)
	}
	defer reader.Close()

	_, seasonRef, err := firestore.GetSeason(ctx, ctx.FirestoreClient, ctx.Season)
	if err != nil {
		return fmt.Errorf("ParseSlate: failed to get season: %w", err)
	}

	_, weekRef, err := firestore.GetWeek(ctx, seasonRef, ctx.Week)
	if err != nil {
		return fmt.Errorf("ParseSlate: failed to get week: %w", err)
	}

	games, gameRefs, err := firestore.GetGames(ctx, weekRef)
	if err != nil {
		return fmt.Errorf("ParseSlate: failed to get games: %w", err)
	}

	gl := firestore.NewGameRefsByMatchup(games, gameRefs)

	teams, teamRefs, err := firestore.GetTeams(ctx, seasonRef)
	if err != nil {
		return fmt.Errorf("ParseSlate: failed to get teams: %w", err)
	}

	tlOther := firestore.NewTeamRefsByOtherName(teams, teamRefs)
	tlShort := firestore.NewTeamRefsByShortName(teams, teamRefs)

	slurp, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("ParseSlate: failed to read slate file: %w", err)
	}

	sgames, err := parseSheet(slurp, tlOther, tlShort, gl)
	if err != nil {
		return fmt.Errorf("ParseSlate: failed to parse games from slate file: %w", err)
	}

	if ctx.DryRun {
		log.Print("DRY RUN: would write the following to firestore:")
		for _, g := range sgames {
			log.Printf("%s", g)
		}
		return nil
	}

	ct, err := getCreationTime(ctx, ctx.Slate)
	if err != nil {
		return fmt.Errorf("ParseSlate: failed to stat time from file: %w", err)
	}
	slate := firestore.Slate{
		Created:  ct,
		FileName: ctx.Slate,
	}
	slateRef := weekRef.Collection(firestore.SLATES_COLLECTION).NewDoc()
	err = ctx.FirestoreClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
		var err error
		if ctx.Force {
			err = t.Set(slateRef, &slate)
		} else {
			err = t.Create(slateRef, &slate)
		}
		if err != nil {
			return err
		}

		for _, game := range sgames {
			gameID := game.Game.ID // convenient
			gameRef := slateRef.Collection(firestore.SLATE_GAMES_COLLECTION).Doc(gameID)
			if ctx.Force {
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
		return fmt.Errorf("ParseSlate: failed to store slate and games in firestore: %w", err)
	}

	return nil
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

func openFileOrGSWriter(ctx context.Context, f string) (io.WriteCloser, error) {
	u, err := url.Parse(f)
	if err != nil {
		return nil, err
	}
	var w io.WriteCloser
	switch u.Scheme {
	case "gs":
		gsClient, err := storage.NewClient(ctx)
		if err != nil {
			return nil, err
		}
		bucket := gsClient.Bucket(u.Host)
		obj := bucket.Object(u.Path)
		w = obj.NewWriter(ctx)

	case "file":
		fallthrough
	case "":
		w, err = os.Create(u.Path)
		if err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unable to determine how to open '%s'", f)
	}

	return w, nil
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

	// catch all the errors from all the cells and report them all rather than stopping after the first
	errors := make([]error, 0)

	for irow, row := range sheet.Rows {
		for icol, cell := range row.Cells {

			matchup, homeRank, awayRank, gotw, found, err := parseGame(cell.Value, tlShort)
			if err != nil {
				errors = append(errors, err)
				continue
			}
			if found {
				value := 1
				if gotw {
					value = 2
				}
				game, swap, wn, ok := gl.LookupCorrectMatchup(matchup)
				if !ok {
					errors = append(errors, fmt.Errorf("pick matchup %+v not found", matchup))
					continue
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
					Value:               value,
				}
				// check the immediate next column for noise
				if len(row.Cells) != icol+1 {
					favorite, spread, found, err := parseNoisySpread(row.Cells[icol+1].Value, tlShort)
					if err != nil {
						errors = append(errors, err)
						continue
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
				errors = append(errors, err)
			}
			if !found {
				continue
			}
			game, swap, _, ok := gl.LookupCorrectMatchup(matchup)
			if !ok {
				errors = append(errors, fmt.Errorf("superdog matchup %+v not found", matchup))
				continue
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

	if len(errors) != 0 {
		log.Print("Errors occured while parsing the slate")
		for i, e := range errors {
			log.Printf("Error %d: %s", i, e)
		}
		log.Print("Returning the first error")
		err = errors[0]
	}

	return games, err
}

// ^(\*\*)?         # Marker for GOTW
// (?:\#(\d+)\s+)?  # Optional rank for Team 1
// (.*?)\s+         # Team 1 (home) name in LUKE format
// (@|vs)\s+        # Whether or not Team 2 is away (@) or if the game is at a neutral site (vs)
// (?:\#(\d+)\s+)?  # Optional rank for Team 2
// (.*?)            # Team 2 (away) name in LUKE format
// \1?\s*$          # Marker for GOTW
var gameRe = regexp.MustCompile(`^\s*(\*\*)?\s*(?:#\s*(\d+)\s+)?(.*?)\s+((?i:@|at|vs))\s+(?:#\s*(\d+)\s+)?(.*?)(?:\s*\*\*)?\s*$`)

var noiseRe = regexp.MustCompile(`\s*(?i:Enter\s+(.*?)\s+iff\s+you\s+predict\s+.*?\s+wins\s+by\s+at\s+least\s+(\d+)\s+points)`)

var sdRe = regexp.MustCompile(`(?i:\s*(?:#\s*\d+\s+)?(.*?)\s+over\s+(?:#\s*\d+\s+)?(.*?)\s+\(\s*(\d+)\s+points,?\s+if\s+correct\s*\))`)

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

	name = submatches[2]
	if teamRef, ok = tl[name]; !ok {
		err = fmt.Errorf("parseDog: unable to find team with name '%s' in cell '%s'", name, cell)
		return
	}
	matchup.Away = teamRef.ID
	favorite = teamRef.ID // second team listed is always the favorite

	value, err = strconv.Atoi(submatches[3])

	if err != nil {
		err = fmt.Errorf("parseDog: error parsing game value: %w", err)
		return
	}

	return
}
