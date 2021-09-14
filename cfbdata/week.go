package cfbdata

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/firestore"
)

type Week struct {
	Season         string    `json:"season"`
	Number         uint64    `json:"week"`
	FirstGameStart time.Time `json:"firstGameStart"`
	LastGameStart  time.Time `json:"lastGameStart"`
}

type WeekCollection struct {
	weeks   []Week
	fsWeeks []firestore.Week
	refs    []*fs.DocumentRef
	ids     map[uint64]int
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
	ids := make(map[uint64]int)
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

func (wc WeekCollection) Datum(i int) interface{} {
	return wc.weeks[i]
}

func (wc WeekCollection) RefByID(id uint64) (*fs.DocumentRef, bool) {
	if i, ok := wc.ids[id]; ok {
		return wc.refs[i], true
	}
	return nil, false
}

func (w Week) toFirestore() firestore.Week {
	return firestore.Week{
		Number: int(w.Number),
	}
}

func (wc WeekCollection) LinkRefs(sr *fs.DocumentRef, col *fs.CollectionRef) error {
	for i, week := range wc.weeks {
		fsWeek := wc.fsWeeks[i]
		fsWeek.Season = sr
		wc.fsWeeks[i] = fsWeek
		wc.refs[i] = col.Doc(fmt.Sprintf("%d", week.Number))
	}
	return nil
}

func (wc WeekCollection) Select(n int) (WeekCollection, bool) {
	if i, ok := wc.ids[uint64(n)]; ok {
		return WeekCollection{weeks: wc.weeks[i : i+1], fsWeeks: wc.fsWeeks[i : i+1], refs: wc.refs[i : i+1], ids: map[uint64]int{uint64(n): i}}, true
	}
	return WeekCollection{}, false
}
