package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"time"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
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

	Commands["update-models"] = updateModels
	Usage["update-models"] = umUsage
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
	year := umFlagSet.Arg(0) // technically, strings are okay
	week := umFlagSet.Arg(1)

	ctx := context.Background()
	fsClient, err := fs.NewClient(ctx, ProjectID)
	if err != nil {
		log.Fatalf("Error creating firestore client: %v", err)
	}

	models, refs, err := firestore.GetModels(ctx, fsClient)
	if err != nil {
		log.Fatalf("Error getting models: %v", err)
	}

	lookup := firestore.NewModelRefsByShortName(models, refs)
	slookup := firestore.NewModelRefsBySystem(models, refs)
	rlookup := lookup.ReverseMap()

	pt, err := newPerformanceTable(perfURL, slookup)
	if err != nil {
		log.Fatalf("Error parsing performance table: %v", err)
	}

	weekRef := fsClient.Collection("seasons").Doc(year).Collection("weeks").Doc(week)
	weekSS, err := weekRef.Get(ctx)
	if err != nil {
		log.Fatalf("Error getting week snapshot: %v", err)
	}
	if !weekSS.Exists() {
		log.Fatalf("Week '%s' of season '%s' does not exist: run setup-season", week, year)
	}

	now := time.Now()
	perfRef := weekRef.Collection("model-performances").Doc(now.Format(time.RFC3339))

	if DryRun {
		log.Printf("DRY RUN: would write the following to %s:", perfRef.Path)
		for _, p := range *pt {
			log.Println(p)
		}
		return
	}

	err = fsClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
		perfCollDoc := struct {
			Timestamp time.Time `firestore:"timestamp"`
		}{
			Timestamp: now,
		}
		var err error
		if Force {
			err = t.Set(perfRef, &perfCollDoc)
		} else {
			err = t.Create(perfRef, &perfCollDoc)
		}
		if err != nil {
			return err
		}
		for _, p := range *pt {
			name, ok := rlookup[p.Model]
			if !ok {
				return fmt.Errorf("model short name for '%s' not in lookup table", p.Model.ID)
			}
			ref := perfRef.Collection("performances").Doc(name)
			if Force {
				err = t.Set(ref, &p)
			} else {
				err = t.Create(ref, &p)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		log.Fatalf("Unable to write model performances to firestore: %v", err)
	}
}

type performanceTable []firestore.ModelPerformance

var resultsTableRegex = regexp.MustCompile(`(?ism)<table\s+[^>]+CLASS=['"]results_table['"].*>(.*?)</table>`)
var headerRegex = regexp.MustCompile(`(?ism)<th.*?>\s*(?:<a.*?>)?\s*(.*?)\s*(?:</a>)?\s*</th>`)
var rowRegex = regexp.MustCompile(`(?ism)<tr><td.*?>.*?</td></tr>`)
var valueRegex = regexp.MustCompile(`(?ism)<td.*?>(?:<font.*?>)?(.*?)(?:</font>)?</td>`)

func newPerformanceTable(f string, lookup firestore.ModelRefsByName) (*performanceTable, error) {
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
				perf.Model = model
			default:
				return nil, fmt.Errorf("column '%s' in row %d not understood", col, j)
			}
			if err != nil {
				return nil, fmt.Errorf("error parsing column '%s' in row '%d': %w", col, j, err)
			}
		}
		perf.StdDev = math.Sqrt(perf.MSE - math.Pow(perf.Bias, 2.))
		pt = append(pt, perf)
	}

	return &pt, nil
}
