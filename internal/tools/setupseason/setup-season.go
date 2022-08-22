package setupseason

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/cfbdata"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func SetupSeason(ctx *Context) error {

	httpClient := http.DefaultClient

	weeks, err := cfbdata.GetWeeks(httpClient, ctx.ApiKey, ctx.Season, ctx.Weeks)
	if err != nil {
		return fmt.Errorf("SetupSeason: failed to get weeks: %w", err)
	}
	log.Printf("Loaded %d weeks\n", weeks.Len())

	venues, err := cfbdata.GetVenues(httpClient, ctx.ApiKey)
	if err != nil {
		return fmt.Errorf("SetupSeason: failed to get game venues: %w", err)
	}
	log.Printf("Loaded %d venues\n", venues.Len())

	teams, err := cfbdata.GetTeams(httpClient, ctx.ApiKey)
	if err != nil {
		return fmt.Errorf("SetupSeason: failed to get teams: %w", err)
	}
	log.Printf("Loaded %d teams\n", teams.Len())

	games, err := cfbdata.GetAllGames(httpClient, ctx.ApiKey, ctx.Season)
	if err != nil {
		return fmt.Errorf("SetupSeason: failed to get all games in season: %w", err)
	}
	log.Printf("Loaded %d games\n", games.Len())

	// eliminate teams that are not in games
	teams = teams.EliminateNonContenders(games)

	// set everything up to write to firestore
	seasonID := strconv.Itoa(ctx.Season)
	seasonRef := ctx.FirestoreClient.Collection(firestore.SEASONS_COLLECTION).Doc(seasonID)
	season := firestore.Season{
		Year:            ctx.Season,
		StartTime:       weeks.FirstStartTime(),
		Pickers:         make(map[string]*fs.DocumentRef),
		StreakTeams:     make([]*fs.DocumentRef, 0),
		StreakPickTypes: make([]int, 0),
	}
	if err := weeks.LinkRefs(seasonRef.Collection(firestore.WEEKS_COLLECTION)); err != nil {
		return fmt.Errorf("SetupSeason: failed to link week references: %w", err)
	}
	if err := venues.LinkRefs(seasonRef.Collection(firestore.VENUES_COLLECTION)); err != nil {
		return fmt.Errorf("SetupSeason: failed to link venue references: %w", err)
	}
	if err := teams.LinkRefs(venues, seasonRef.Collection(firestore.TEAMS_COLLECTION)); err != nil {
		return fmt.Errorf("SetupSeason: failed to link team references: %w", err)
	}

	newWeeks := make(map[int64]struct{})
	for i := 0; i < weeks.Len(); i++ {
		wr := weeks.Ref(i)
		_, err := wr.Get(ctx)
		if status.Code(err) == codes.NotFound {
			newWeeks[weeks.ID(i)] = struct{}{}
		}
	}

	gamesByWeek := make(map[int64]cfbdata.GameCollection)
	for i := 0; i < weeks.Len(); i++ {
		id := weeks.ID(i)
		wr := weeks.Ref(i)
		gs := games.GetWeek(int(id))
		if err := gs.LinkRefs(teams, venues, wr.Collection(firestore.GAMES_COLLECTION)); err != nil {
			return fmt.Errorf("SetupSeason: failed to link game references: %w", err)
		}
		gamesByWeek[id] = gs
	}

	if ctx.DryRun {
		log.Println("DRY RUN: would write the following to firestore:")
		log.Printf("Season:\n%s: %+v\n---\n", seasonRef.Path, season)
		log.Println("Venues:")
		cfbdata.DryRun(log.Writer(), venues)
		log.Println("---")
		log.Println("Teams:")
		cfbdata.DryRun(log.Writer(), teams)
		log.Println("---")
		log.Println("Weeks:")
		cfbdata.DryRun(log.Writer(), weeks)
		log.Println("---")
		log.Println("Games:")
		for wk, gc := range gamesByWeek {
			log.Printf("Week %d\n", wk)
			cfbdata.DryRun(log.Writer(), gc)
		}
		log.Println("---")
		return nil
	}

	// Either set or create, depending on force parameter
	if ctx.Force {
		log.Println("Forcing overwrite with UPDATE command")
		_, err := seasonRef.Update(ctx,
			[]fs.Update{
				{Path: "year", Value: &season.Year},
				{Path: "start_time", Value: &season.StartTime},
			},
		)
		if err != nil {
			return fmt.Errorf("SetupSeason: failed to update season data: %w", err)
		}
	} else {
		log.Println("Writing with CREATE command")
		_, err := seasonRef.Create(ctx, &season)
		if err != nil {
			return fmt.Errorf("SetupSeason: failed to create season: %w", err)
		}
	}

	// Venues second
	vfcn := cfbdata.TransactionIterator{
		UpdateFcn: func(t *fs.Transaction, dr *fs.DocumentRef, i interface{}) error {
			if !ctx.Force {
				return t.Create(dr, i)
			}
			v, ok := i.(firestore.Venue)
			if !ok {
				return fmt.Errorf("writeFunc: failed to convert value to Venue")
			}
			return t.Update(dr, []fs.Update{
				{Path: "name", Value: v.Name},
				{Path: "capacity", Value: v.Capacity},
				{Path: "grass", Value: v.Grass},
				{Path: "city", Value: v.City},
				{Path: "state", Value: v.State},
				{Path: "zip", Value: v.Zip},
				{Path: "country_code", Value: v.CountryCode},
				{Path: "latlon", Value: v.LatLon},
				{Path: "year", Value: v.Year},
				{Path: "dome", Value: v.Dome},
				{Path: "timezone", Value: v.Timezone},
			})
		},
	}
	errs := vfcn.IterateTransaction(ctx, ctx.FirestoreClient, venues, 500)
	for err := range errs {
		if err != nil {
			return fmt.Errorf("SetupSeason: failed running venues transaction: %w", err)
		}
	}

	// Teams third: do not update if --force is specified!
	var oneTeamErr sync.Once
	tfcn := cfbdata.TransactionIterator{
		UpdateFcn: func(t *fs.Transaction, dr *fs.DocumentRef, i interface{}) error {
			oneTeamErr.Do(func() {
				log.Print("Refusing to update teams: use teams command instead")
			})
			return nil
		},
	}

	errs = tfcn.IterateTransaction(ctx, ctx.FirestoreClient, teams, 500)
	for err := range errs {
		if err != nil {
			return fmt.Errorf("SetupSeason: failed running teams transaction: %w", err)
		}
	}

	// Weeks fourth
	wfcn := cfbdata.TransactionIterator{
		UpdateFcn: func(t *fs.Transaction, dr *fs.DocumentRef, i interface{}) error {
			if !ctx.Force {
				return t.Create(dr, i)
			}
			v, ok := i.(firestore.Week)
			if !ok {
				return fmt.Errorf("writeFunc: failed to convert value to Week")
			}
			return t.Update(dr, []fs.Update{
				{Path: "number", Value: v.Number},
				{Path: "first_game_start", Value: v.FirstGameStart},
			})
		},
	}
	errs = wfcn.IterateTransaction(ctx, ctx.FirestoreClient, weeks, 500)
	for err := range errs {
		if err != nil {
			return fmt.Errorf("SetupSeason: failed running weeks transaction: %w", err)
		}
	}

	// Games fifth
	gfcn := cfbdata.TransactionIterator{
		UpdateFcn: func(t *fs.Transaction, dr *fs.DocumentRef, i interface{}) error {
			if !ctx.Force {
				return t.Create(dr, i)
			}
			v, ok := i.(firestore.Game)
			if !ok {
				return fmt.Errorf("writeFunc: failed to convert value to Game")
			}
			return t.Update(dr, []fs.Update{
				{Path: "home_team", Value: v.HomeTeam},
				{Path: "away_team", Value: v.AwayTeam},
				{Path: "start_time", Value: v.StartTime},
				{Path: "start_time_tbd", Value: v.StartTimeTBD},
				{Path: "neutral_site", Value: v.NeutralSite},
				{Path: "venue", Value: v.Venue},
				{Path: "home_points", Value: v.HomePoints},
				{Path: "away_points", Value: v.AwayPoints},
			})
		},
	}
	for _, weekOfGames := range gamesByWeek {
		errs = gfcn.IterateTransaction(ctx, ctx.FirestoreClient, weekOfGames, 500)
		for err := range errs {
			if err != nil {
				return fmt.Errorf("SetupSeason: failed running games transaction: %w", err)
			}
		}
	}

	return nil
}
