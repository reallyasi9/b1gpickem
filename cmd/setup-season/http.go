package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/reallyasi9/b1gpickem/firestore"
)

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

func GetWeeks(client *http.Client, key string, season int) (WeekCollection, error) {
	body, err := doRequest(client, key, fmt.Sprintf("https://api.collegefootballdata.com/calendar?year=%d", season))
	if err != nil {
		return WeekCollection{}, fmt.Errorf("failed to do calendar request: %v", err)
	}

	var weeks []Week
	err = json.Unmarshal(body, &weeks)
	if err != nil {
		return WeekCollection{}, fmt.Errorf("failed to unmarshal calendar response body: %v", err)
	}

	wmap := make(map[int]Week)
	for _, week := range weeks {
		wmap[week.Number] = week
	}
	return WeekCollection{weeks: wmap, fsWeeks: make(map[int]firestore.Week)}, nil
}

func getTeams(client *http.Client, key string) (map[uint64]Team, error) {
	body, err := doRequest(client, key, "https://api.collegefootballdata.com/teams")
	if err != nil {
		return nil, fmt.Errorf("failed to do teams request: %v", err)
	}

	var teams []Team
	err = json.Unmarshal(body, &teams)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal teams response body: %v", err)
	}

	tmap := make(map[uint64]Team)
	for _, t := range teams {
		tmap[t.ID] = t
	}

	return tmap, nil
}

func GetVenues(client *http.Client, key string) (VenueCollection, error) {
	body, err := doRequest(client, key, "https://api.collegefootballdata.com/venues")
	if err != nil {
		return VenueCollection{}, fmt.Errorf("failed to do venues request: %v", err)
	}

	var venues []Venue
	err = json.Unmarshal(body, &venues)
	if err != nil {
		return VenueCollection{}, fmt.Errorf("failed to unmarshal venues response body: %v", err)
	}

	vmap := make(map[uint64]Venue)
	for _, v := range venues {
		vmap[v.ID] = v
	}

	return VenueCollection{venues: vmap, fsVenues: make(map[uint64]firestore.Venue)}, nil
}

func GetGames(client *http.Client, key string, year, week int) (GameCollection, error) {
	query := fmt.Sprintf("?year=%d", year)
	if week > 0 {
		query += fmt.Sprintf("&week=%d", week)
	}
	body, err := doRequest(client, key, "https://api.collegefootballdata.com/games"+query)
	if err != nil {
		return GameCollection{}, fmt.Errorf("failed to do game request: %v", err)
	}

	var games []Game
	err = json.Unmarshal(body, &games)
	if err != nil {
		return GameCollection{}, fmt.Errorf("failed to unmarshal games response body: %v", err)
	}

	gmap := make(map[uint64]Game)
	for _, g := range games {
		gmap[g.ID] = g
	}

	return GameCollection{games: gmap, fsGames: make(map[uint64]firestore.Game)}, nil
}
