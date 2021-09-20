package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// The subcommand update-predictions scrapes both the performance to date of the various prediction models from https://www.thepredictiontracker.com/ncaaresults.php
// as well as the individual model predictions for each game of the week from https://www.thepredictiontracker.com/ncaapredictions.csv.
// Only a subset of games are predicted each week (games that have an opening Vegas line).

// upFlagSet is a flag.FlagSet for parsing the update-predictions subcommand.
var upFlagSet *flag.FlagSet

// predCSV is the URL for game predictions.
var predCSV string

// upUsage is the usage documentation for the update-predictions subcommand.
func upUsage() {
	fmt.Fprint(flag.CommandLine.Output(), `Usage: b1gtool [global-flags] update-predictions [flags]
	
Update predictions for games in Firestore. Downloads data from thepredictiontracker.com.
	
Arguments:

Flags:
`)

	ugFlagSet.PrintDefaults()

	fmt.Fprint(flag.CommandLine.Output(), "Global Flags:\n")

	flag.PrintDefaults()

}

func init() {
	upFlagSet = flag.NewFlagSet("update-predictions", flag.ExitOnError)
	upFlagSet.SetOutput(flag.CommandLine.Output())
	upFlagSet.Usage = upUsage

	upFlagSet.StringVar(&predCSV, "csv", "https://www.thepredictiontracker.com/ncaapredictions.csv", "URL or file name of CSV file containing model predictions.")
}

func updatePredictions() {
	err := upFlagSet.Parse(flag.Args()[1:])
	if err != nil {
		log.Fatalf("Failed to parse update-predictions arguments: %v", err)
	}

	pt, err := newPredictionTable(predCSV)
	if err != nil {
		log.Fatalf("Failed to read prediction table from CSV '%s': %v", predCSV, err)
	}

	fmt.Print(pt)
}

// predictionTable collects the predictions for a set of models in a nice format.
type predictionTable struct {
	homeTeams   []string
	awayTeams   []string
	predictions map[string][]float64
	missing     map[string][]bool
}

func newPredictionTable(f string) (*predictionTable, error) {
	var rc io.ReadCloser
	if _, err := url.Parse(f); err == nil {
		httpClient := http.DefaultClient
		var err error
		rc, err = request(httpClient, f)
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		rc, err = os.Open(f)
		if err != nil {
			return nil, err
		}
	}
	defer rc.Close()
	csvr := csv.NewReader(rc)

	record, err := csvr.Read()
	if err != nil {
		return nil, fmt.Errorf("error reading header from '%s': %v", f, err)
	}
	header, err := headerMap(record)
	if err != nil {
		return nil, err
	}
	homeTeams := make([]string, 0)
	awayTeams := make([]string, 0)
	predictions := make(map[string][]float64)
	missing := make(map[string][]bool)
	for {
		record, err := csvr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		for colname, which := range header {
			val := record[which]
			switch colname {
			case "home":
				homeTeams = append(homeTeams, val)
			case "road":
				awayTeams = append(awayTeams, val)
			default:
				if !strings.HasPrefix(colname, "line") {
					continue
				}
				f := 0.
				m := true
				if val != "" {
					m = false
					f, err = strconv.ParseFloat(val, 64)
				}
				if err != nil {
					return nil, err
				}
				predictions[colname] = append(predictions[colname], f)
				missing[colname] = append(missing[colname], m)
			}
		}
	}
	return &predictionTable{
		homeTeams:   homeTeams,
		awayTeams:   awayTeams,
		predictions: predictions,
		missing:     missing,
	}, nil
}

func request(client *http.Client, url string) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to do request: %v", err)
	}
	return resp.Body, nil
}

func headerMap(record []string) (map[string]int, error) {
	out := make(map[string]int)
	for i, s := range record {
		if j, ok := out[s]; ok {
			return nil, fmt.Errorf("header '%s' repeated in columns %d and %d", s, i, j)
		}
		out[s] = i
	}
	return out, nil
}
