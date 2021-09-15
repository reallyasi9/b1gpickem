package cfbdata

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/firestore"
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

	f := make([]firestore.Week, len(weeks))
	refs := make([]*fs.DocumentRef, len(weeks))
	ids := make(map[int64]int)
	for i, w := range weeks {
		f[i] = w.toFirestore()
		ids[w.Number] = i
	}
	return WeekCollection{weeks: weeks, fsWeeks: f, refs: refs, ids: ids}, nil
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
	return firestore.Week{}
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
