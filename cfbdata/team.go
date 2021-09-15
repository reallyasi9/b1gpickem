package cfbdata

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/firestore"
)

type Team struct {
	ID           int64    `json:"id"`
	School       string   `json:"school"`
	Mascot       *string  `json:"mascot"`
	Abbreviation *string  `json:"abbreviation"`
	AltName1     *string  `json:"alt_name1"`
	AltName2     *string  `json:"alt_name2"`
	AltName3     *string  `json:"alt_name3"`
	Color        *string  `json:"color"`
	AltColor     *string  `json:"alt_color"`
	Logos        []string `json:"logos"`
	Location     struct {
		VenueID *int64 `json:"venue_id"`
	}
}

func appendNonNilStrings(s []string, vals ...*string) []string {
	for _, v := range vals {
		if v == nil {
			continue
		}
		s = append(s, *v)
	}
	return s
}

func coalesceString(s *string, replacement string) string {
	if s == nil || *s == "" {
		return replacement
	}
	return *s
}

func distinctStrings(ss []string) []string {
	// defensive copy
	result := make([]string, len(ss))
	copy(result, ss)
	distinct := make(map[string]struct{})
	n := 0
	for _, s := range result {
		if _, ok := distinct[s]; ok {
			result[n] = s
			n++
			continue
		}
		distinct[s] = struct{}{}
	}
	result = result[:n]
	return result
}

func abbreviate(s string) string {
	if len(s) < 5 {
		return strings.ToUpper(s)
	}
	splits := strings.Split(s, " ")
	if len(splits) == 1 {
		return strings.ToUpper(s[:4])
	}
	var sb strings.Builder
	for _, split := range splits {
		sb.WriteString(strings.ToUpper(split[:1]))
	}
	return sb.String()
}

// ToFirestore does not link the Venue--that has to be done with an external lookup.
func (t Team) toFirestore() firestore.Team {
	otherNames := make([]string, 0)
	otherNames = appendNonNilStrings(otherNames, t.AltName1, t.AltName2, t.AltName3)
	otherNames = distinctStrings(otherNames)
	colors := make([]string, 0)
	colors = appendNonNilStrings(colors, t.Color, t.AltColor)

	abbr := coalesceString(t.Abbreviation, strings.ToUpper(t.School))
	ft := firestore.Team{
		Abbreviation: coalesceString(t.Abbreviation, strings.ToUpper(t.School)),
		ShortNames:   []string{abbr},
		OtherNames:   otherNames,
		School:       t.School,
		Mascot:       coalesceString(t.Mascot, "Football Team"),
		Colors:       colors,
	}
	return ft
}

type TeamCollection struct {
	teams   []Team
	fsTeams []firestore.Team
	refs    []*fs.DocumentRef
	ids     map[int64]int
}

// Len returns the number of weeks in the collection
func (tc TeamCollection) Len() int {
	return len(tc.teams)
}

func (tc TeamCollection) Ref(i int) *fs.DocumentRef {
	return tc.refs[i]
}

func (tc TeamCollection) ID(i int) int64 {
	return tc.teams[i].ID
}

func (tc TeamCollection) Datum(i int) interface{} {
	return tc.fsTeams[i]
}

func (tc TeamCollection) RefByID(id int64) (*fs.DocumentRef, bool) {
	if i, ok := tc.ids[id]; ok {
		return tc.refs[i], true
	}
	return nil, false
}

func GetTeams(client *http.Client, key string) (TeamCollection, error) {
	body, err := doRequest(client, key, "https://api.collegefootballdata.com/teams")
	if err != nil {
		return TeamCollection{}, fmt.Errorf("failed to do teams request: %v", err)
	}

	var teams []Team
	err = json.Unmarshal(body, &teams)
	if err != nil {
		return TeamCollection{}, fmt.Errorf("failed to unmarshal teams response body: %v", err)
	}

	// // Filter out worthless teams
	// n := 0
	// for _, t := range teams {
	// 	if t.ID < 100000 { // above 100000 are historical teams
	// 		teams[n] = t
	// 		n++
	// 	}
	// }
	// teams = teams[:n]

	f := make([]firestore.Team, len(teams))
	refs := make([]*fs.DocumentRef, len(teams))
	ids := make(map[int64]int)
	for i, t := range teams {
		f[i] = t.toFirestore()
		ids[t.ID] = i
	}
	return TeamCollection{teams: teams, fsTeams: f, refs: refs, ids: ids}, nil

}

func (tc TeamCollection) LinkRefs(vc VenueCollection, col *fs.CollectionRef) error {
	for i, t := range tc.teams {
		id := t.ID
		fst := tc.fsTeams[i]
		venueID := t.Location.VenueID

		if venueID != nil {
			var ok bool
			if fst.Venue, ok = vc.RefByID(*venueID); !ok {
				return fmt.Errorf("venue %d for game %d not found in reference map", venueID, id)
			}
		}

		tc.fsTeams[i] = fst
		tc.refs[i] = col.Doc(fmt.Sprintf("%d", id))
	}
	return nil
}

func (tc TeamCollection) FprintDatum(w io.Writer, i int) (int, error) {
	return fmt.Fprint(w, tc.fsTeams[i].String())
}
