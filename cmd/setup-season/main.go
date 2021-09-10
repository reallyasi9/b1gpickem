package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// APIKey is a key from collegefootballdata.com
var APIKey string

// ProjectID is the Google Cloud Project ID where the season data will be loaded.
var ProjectID string

// UpdateWeek, if set, will just update one week's worth of games rather than replacing the entire dataset in Firestore.
var UpdateWeek int

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

	client := http.DefaultClient

	teams, err := GetTeams(client, APIKey)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Loaded %d teams\n", len(teams))
	fmt.Printf("First team:\n%+v\nLast team:\n%+v", teams[0], teams[len(teams)-1])
}

func GetTeams(client *http.Client, key string) ([]Team, error) {
	req, err := http.NewRequest("GET", "https://api.collegefootballdata.com/teams", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build teams request: %v", err)
	}
	req.Header.Add("accept", "application/json")
	req.Header.Add("Authorization", "Bearer "+key)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to do teams request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read teams response body: %v", err)
	}

	var teams []Team
	err = json.Unmarshal(body, &teams)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal teams response body: %v", err)
	}

	return teams, nil
}
