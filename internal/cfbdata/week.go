package cfbdata

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
)

type Week struct {
	Season         string    `json:"season"`
	Number         int64     `json:"week"`
	FirstGameStart time.Time `json:"firstGameStart"`
	LastGameStart  time.Time `json:"lastGameStart"`
}

type WeekCollection struct {
	weeks   []Week
	fsWeeks []firestore.Week
	refs    []*fs.DocumentRef
	ids     map[int64]int
}

func GetWeeks(client *http.Client, key string, season int, weekNumbers []int) (WeekCollection, error) {

	keep := make(map[int]struct{})
	for _, n := range weekNumbers {
		keep[n] = struct{}{}
	}

	body, err := DoRequest(client, key, fmt.Sprintf("https://api.collegefootballdata.com/calendar?year=%d", season))
	if err != nil {
		return WeekCollection{}, fmt.Errorf("failed to do calendar request: %v", err)
	}

	var weeks []Week
	err = json.Unmarshal(body, &weeks)
	if err != nil {
		return WeekCollection{}, fmt.Errorf("failed to unmarshal calendar response body: %v", err)
	}

	kept := make([]Week, 0, len(weeks))
	f := make([]firestore.Week, 0, len(weeks))
	refs := make([]*fs.DocumentRef, 0, len(weeks))
	ids := make(map[int64]int)
	i := 0
	for _, w := range weeks {
		_, ok := keep[int(w.Number)]
		if len(weekNumbers) > 0 && !ok {
			continue
		}
		kept = append(kept, w)
		f = append(f, w.toFirestore())
		refs = append(refs, nil)
		ids[w.Number] = i
		i++
	}
	return WeekCollection{weeks: kept, fsWeeks: f, refs: refs, ids: ids}, nil
}

// Len returns the number of weeks in the collection
func (wc WeekCollection) Len() int {
	return len(wc.weeks)
}

func (wc WeekCollection) Ref(i int) *fs.DocumentRef {
	return wc.refs[i]
}

func (wc WeekCollection) ID(i int) int64 {
	return wc.weeks[i].Number
}

func (wc WeekCollection) Datum(i int) interface{} {
	return wc.fsWeeks[i]
}

func (wc WeekCollection) RefByID(id int64) (*fs.DocumentRef, bool) {
	if i, ok := wc.ids[id]; ok {
		return wc.refs[i], true
	}
	return nil, false
}

func (w Week) toFirestore() firestore.Week {
	return firestore.Week{
		Number:         int(w.Number),
		FirstGameStart: w.FirstGameStart,
	}
}

func (wc WeekCollection) LinkRefs(col *fs.CollectionRef) error {
	for i, week := range wc.weeks {
		fsWeek := wc.fsWeeks[i]
		wc.fsWeeks[i] = fsWeek
		wc.refs[i] = col.Doc(fmt.Sprintf("%d", week.Number))
	}
	return nil
}

func (wc WeekCollection) Select(n int) (WeekCollection, bool) {
	if i, ok := wc.ids[int64(n)]; ok {
		return WeekCollection{weeks: wc.weeks[i : i+1], fsWeeks: wc.fsWeeks[i : i+1], refs: wc.refs[i : i+1], ids: map[int64]int{int64(n): i}}, true
	}
	return WeekCollection{}, false
}

// Split splits a WeekCollection into two based on indices to include or exclude from the results.
func (wc WeekCollection) Split(include []int) (in WeekCollection, out WeekCollection) {
	includeMap := make(map[int]struct{})
	for _, i := range include {
		includeMap[i] = struct{}{}
	}
	in.ids = make(map[int64]int)
	out.ids = make(map[int64]int)
	for i := 0; i < wc.Len(); i++ {
		if _, ok := includeMap[i]; ok {
			in.fsWeeks = append(in.fsWeeks, wc.fsWeeks[i])
			in.weeks = append(in.weeks, wc.weeks[i])
			in.refs = append(in.refs, wc.refs[i])
			in.ids[wc.weeks[i].Number] = i
		} else {
			out.fsWeeks = append(out.fsWeeks, wc.fsWeeks[i])
			out.weeks = append(out.weeks, wc.weeks[i])
			out.refs = append(out.refs, wc.refs[i])
			out.ids[wc.weeks[i].Number] = i
		}
	}
	return
}

func (wc WeekCollection) FprintDatum(w io.Writer, i int) (int, error) {
	return fmt.Fprint(w, wc.fsWeeks[i].String())
}

func (wc WeekCollection) FirstStartTime() time.Time {
	first := wc.weeks[0].FirstGameStart
	for i := 1; i < len(wc.fsWeeks); i++ {
		if wc.weeks[i].FirstGameStart.Before(first) {
			first = wc.weeks[i].FirstGameStart
		}
	}
	return first
}
