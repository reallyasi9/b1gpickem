package updatemodels

import (
	"bytes"
	"context"
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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const PERF_URL = "https://www.thepredictiontracker.com/ncaaresults.php"

func UpdateModels(ctx *Context) error {
	models, refs, err := firestore.GetModels(ctx, ctx.FirestoreClient)
	if err != nil {
		return fmt.Errorf("UpdateModels: Error getting models: %w", err)
	}

	lookup := firestore.NewModelRefsByShortName(models, refs)
	slookup := firestore.NewModelRefsBySystem(models, refs)
	rlookup := lookup.ReverseMap()

	pt, err := newPerformanceTable(PERF_URL, slookup)
	if err != nil {
		return fmt.Errorf("UpdateModels: Error parsing performance table: %w", err)
	}

	year := strconv.Itoa(ctx.Season)
	week := strconv.Itoa(ctx.Week)
	weekRef := ctx.FirestoreClient.Collection(firestore.SEASONS_COLLECTION).Doc(year).Collection("weeks").Doc(week)
	_, err = weekRef.Get(ctx)
	if status.Code(err) == codes.NotFound {
		return fmt.Errorf("UpdateModels: Week '%s' of season '%s' does not exist: run setup-season", week, year)
	}
	if err != nil {
		return fmt.Errorf("UpdateModels: Error getting week snapshot: %w", err)
	}

	now := time.Now()
	perfRef := weekRef.Collection(firestore.MODEL_PERFORMANCES_COLLECTION).NewDoc()

	if ctx.DryRun {
		log.Printf("DRY RUN: would write the following to %s:", perfRef.Path)
		for _, p := range *pt {
			log.Println(p)
		}
		return nil
	}

	err = ctx.FirestoreClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
		perfCollDoc := struct {
			Timestamp time.Time `firestore:"timestamp"`
		}{
			Timestamp: now,
		}
		var err error
		if ctx.Force {
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
			ref := perfRef.Collection(firestore.PERFORMANCES_COLLECTION).Doc(name)
			if ctx.Force {
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
		return fmt.Errorf("UpdateModels: Unable to write model performances to firestore: %w", err)
	}
	return nil
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
