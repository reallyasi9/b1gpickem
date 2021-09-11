package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/reallyasi9/b1gpickem/firestore"
)

// APIKey is a key from collegefootballdata.com
var APIKey string

// ProjectID is the Google Cloud Project ID where the season data will be loaded.
var ProjectID string

// UpdateWeek, if set, will just update one week's worth of games rather than replacing the entire dataset in Firestore.
var UpdateWeek int

// Season is the year of the start of the season.
var Season int

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(), `Usage: setup-season [flags] <Season>

Set up a new season in Firestore. Downloades data from api.collegefootballdata.com and creates a season with teams, venues, weeks, and games collections.

Arguments:
  Season int
    	Year to set up (e.g., %d).
Flags:
`, time.Now().Year())

	flag.PrintDefaults()
}

func init() {
	flag.Usage = usage

	flag.StringVar(&APIKey, "key", "", "API key for collegefootballdata.com.")
	flag.StringVar(&ProjectID, "project", "", "Google Cloud Project ID.  If equal to the empty string, the environment variable GCP_PROJECT will be used.")
	flag.IntVar(&UpdateWeek, "week", -1, "Only update a given week.  If less than zero, all weeks will be updated.")
}

func main() {
	parseCommandLine()

	client := http.DefaultClient

	venues, err := getVenues(client, APIKey)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Loaded %d venues\n", len(venues))
	// convert to map keyed by ID for easy lookup
	venueLookup := make(map[uint64]firestore.Venue)
	for _, venue := range venues {
		id, v := venue.ToFirestore()
		venueLookup[id] = v
	}

	teams, err := getTeams(client, APIKey)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Loaded %d teams\n", len(teams))
	// convert to map keyed by ID for easy lookup
	teamLookup := make(map[uint64]firestore.Team)
	for _, team := range teams {
		id, t := team.ToFirestore()
		teamLookup[id] = t
	}

	games, err := getGames(client, APIKey, Season, UpdateWeek)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Loaded %d games\n", len(games))

}

func parseCommandLine() {
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}
	if APIKey == "" {
		fmt.Println("APIKey not given: this will probably fail.")
	}
	if ProjectID == "" {
		ProjectID = os.Getenv("GCP_PROJECT")
	}
	if ProjectID == "" {
		fmt.Println("-project not given and environment variable GCP_PROJECT not found: this will probably fail.")
	}

	var err error // avoid shadowing
	Season, err = strconv.Atoi(flag.Arg(0))
	if err != nil {
		panic(err)
	}
}

func doRequest(client *http.Client, key string, url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %v", err)
	}
	req.Header.Add("accept", "application/json")
	req.Header.Add("Authorization", "Bearer "+key)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to do request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	return body, nil
}

func getTeams(client *http.Client, key string) ([]Team, error) {
	body, err := doRequest(client, key, "https://api.collegefootballdata.com/teams")
	if err != nil {
		return nil, fmt.Errorf("failed to do teams request: %v", err)
	}

	var teams []Team
	err = json.Unmarshal(body, &teams)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal teams response body: %v", err)
	}

	return teams, nil
}

func getVenues(client *http.Client, key string) ([]Venue, error) {
	body, err := doRequest(client, key, "https://api.collegefootballdata.com/venues")
	if err != nil {
		return nil, fmt.Errorf("failed to do venues request: %v", err)
	}

	var venues []Venue
	err = json.Unmarshal(body, &venues)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal venues response body: %v", err)
	}

	return venues, nil
}

func getGames(client *http.Client, key string, year, week int) ([]Game, error) {
	query := fmt.Sprintf("?year=%d", year)
	if week > 0 {
		query += fmt.Sprintf("&week=%d", week)
	}
	body, err := doRequest(client, key, "https://api.collegefootballdata.com/games"+query)
	if err != nil {
		return nil, fmt.Errorf("failed to do game request: %v", err)
	}

	var games []Game
	err = json.Unmarshal(body, &games)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal games response body: %v", err)
	}

	return games, nil
}
