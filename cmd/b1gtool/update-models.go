package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/firestore"
)

// The subcommand update-modes the performance to date of the various prediction models from https://www.thepredictiontracker.com/ncaaresults.php.

// umFlagSet is a flag.FlagSet for parsing the update-models subcommand.
var umFlagSet *flag.FlagSet

// perfURL is the URL for model performance to date.
var perfURL string

// umUsage is the usage documentation for the update-models subcommand.
func umUsage() {
	fmt.Fprint(flag.CommandLine.Output(), `Usage: b1gtool [global-flags] update-models [flags] <season> <week>
	
Update model performance to date. Downloads data from thepredictiontracker.com.
	
Arguments:
  season int
      Year of games being updated.
  week int
      Week of games being updated.

Flags:
`)

	umFlagSet.PrintDefaults()

	fmt.Fprint(flag.CommandLine.Output(), "Global Flags:\n")

	flag.PrintDefaults()

}

func init() {
	umFlagSet = flag.NewFlagSet("update-models", flag.ExitOnError)
	umFlagSet.SetOutput(flag.CommandLine.Output())
	umFlagSet.Usage = umUsage

	umFlagSet.StringVar(&perfURL, "perf", "https://www.thepredictiontracker.com/ncaaresults.php", "URL or file name of web site containing model performance to date.")
}

func updateModels() {
	err := umFlagSet.Parse(flag.Args()[1:])
	if err != nil {
		log.Fatalf("Failed to parse update-models arguments: %v", err)
	}

	if umFlagSet.NArg() != 2 {
		umFlagSet.Usage()
		log.Fatal("Season and week arguments not supplied")
	}
	_ = umFlagSet.Arg(0) // technically, strings are okay
	_ = umFlagSet.Arg(1)

	ctx := context.Background()
	fsClient, err := fs.NewClient(ctx, ProjectID)
	if err != nil {
		log.Fatalf("Error creating firestore client: %v", err)
	}

	models, refs, err := firestore.GetModels(ctx, fsClient)
	if err != nil {
		log.Fatalf("Error getting models: %v", err)
	}

	lookup := make(modelRefsByName)
	for i := range models {
		model := models[i]
		ref := refs[i]
		lookup[model.System] = ref
	}

	pt, err := newPerformanceTable(perfURL, lookup)
	if err != nil {
		log.Fatalf("Error parsing performance table: %v", err)
	}

	// TODO: the performances have to be written somewhere...  like season/week/n, in a document that has a timestamp, maybe?
	fmt.Print(pt)
}

type performanceTable []firestore.ModelPerformance

var resultsTableRegex = regexp.MustCompile(`(?ism)<table\s+[^>]+CLASS=['"]results_table['"].*>(.*?)</table>`)
var headerRegex = regexp.MustCompile(`(?ism)<th.*?>\s*(?:<a.*?>)?\s*(.*?)\s*(?:</a>)?\s*</th>`)
var rowRegex = regexp.MustCompile(`(?ism)<tr><td.*?>.*?</td></tr>`)
var valueRegex = regexp.MustCompile(`(?ism)<td.*?>(?:<font.*?>)?(.*?)(?:</font>)?</td>`)

func newPerformanceTable(f string, lookup modelRefsByName) (*performanceTable, error) {
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

	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(rc)
	if err != nil {
		return nil, err
	}
	body := buf.String()

	matches := resultsTableRegex.FindStringSubmatch(body)
	if matches == nil {
		return nil, fmt.Errorf("unable to match table in body")
	}
	table := matches[0]

	headers := headerRegex.FindAllStringSubmatch(table, -1)
	if headers == nil {
		return nil, fmt.Errorf("unable to match headers in table")
	}

	rows := rowRegex.FindAllString(table, -1)
	if rows == nil {
		return nil, fmt.Errorf("unable to match data rows in table")
	}

	pt := performanceTable{}
	for j, row := range rows {
		values := valueRegex.FindAllStringSubmatch(row, -1)
		if values == nil {
			return nil, fmt.Errorf("unable to match values in table row %d", j)
		}

		perf := firestore.ModelPerformance{}
		for i, val := range values {
			s := val[1]
			col := headers[i][1]
			switch col {
			case "Pct. Correct": // float64
				perf.PercentCorrect, err = strconv.ParseFloat(s, 64)
			case "Against Spread": // float64
				f := 1.0
				if s != "" {
					f, err = strconv.ParseFloat(s, 64)
				}
				perf.PercentATS = f
			case "Bias": // float64
				perf.Bias, err = strconv.ParseFloat(s, 64)
			case "Mean Square Error": // float64
				perf.MSE, err = strconv.ParseFloat(s, 64)
			case "Absolute Error": // float64
				perf.MAE, err = strconv.ParseFloat(s, 64)
			case "games": // int
				perf.GamesPredicted, err = strconv.Atoi(s)
			case "suw": // int
				perf.Wins, err = strconv.Atoi(s)
			case "sul": // int
				perf.Losses, err = strconv.Atoi(s)
			case "atsw": // int
				w := 0
				if s != "" {
					w, err = strconv.Atoi(s)
				}
				perf.WinsATS = w
			case "atsl": // int
				l := 0
				if s != "" {
					l, err = strconv.Atoi(s)
					perf.LossesATS = l
				}
			case "Rank": // int
				perf.Rank, err = strconv.Atoi(s)
			case "System": // string
				model, ok := lookup[s]
				if !ok {
					return nil, fmt.Errorf("unable to find model '%s'", s)
				}
				perf.Model = model // TODO: LOOK ME UP!
			default:
				return nil, fmt.Errorf("column '%s' in row %d not understood", col, j)
			}
			if err != nil {
				return nil, fmt.Errorf("error parsing column '%s' in row '%d': %w", col, j, err)
			}
		}
		pt = append(pt, perf)
	}

	return &pt, nil
}

type modelRefsByName map[string]*fs.DocumentRef
