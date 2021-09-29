package cfbdata

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
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

var commonAbbreviations = map[*regexp.Regexp][]string{
	regexp.MustCompile(`\bSt(\.|ate)?\b`):          {"State", "St", "St."},                                             // Appalachian State
	regexp.MustCompile(`\bMiss(\.|issippi)\b`):     {"Mississippi", "Miss", "Miss."},                                   // Southern Mississippi
	regexp.MustCompile(`(?i)\s*\(Oh(\.|io)?\)`):    {" (Ohio)", " (OH)", " (OH.)", " (NTM)", "-Ohio"},                  // Miami (NTM)
	regexp.MustCompile(`(?i)\s*\(Fl(\.|orida)?\)`): {" (Florida)", " (FL)", " (FL.)", " (Fla.)", " (YTM)", "-Florida"}, // Miami (YTM)
	regexp.MustCompile(`\bMichigan\b`):             {"Mich."},                                                          // Central Michigan
	regexp.MustCompile(`\bInternational\b`):        {"Intl."},                                                          // Florida International
	regexp.MustCompile(`\bTennessee\b`):            {"Tenn."},                                                          // Middle Tennessee State
	regexp.MustCompile(`\bUMass\b`):                {"Massachusetts"},                                                  // All the Massachusettses
	regexp.MustCompile(`^UL (.)(.*)$`):             {"Louisiana-${1}${2}", "Louisiana${1}${2}(UL${1})"},                // Louisianas Lafayette and Monroe
	regexp.MustCompile(`^Troy$`):                   {"Troy St."},                                                       // Troy. Kill me.
	regexp.MustCompile(`^Kent State$`):             {"Kent"},                                                           // Kent. Kill me.
	regexp.MustCompile(`Hawai'i`):                  {"Hawaii"},                                                         // Would it kill you to add the appostrophe?
	regexp.MustCompile(`\bVirginia\b`):             {"Va."},                                                            // West Virginia
	regexp.MustCompile(`\bIllinois\b`):             {"Ill."},                                                           // More like "Illannoying," am I right?

	// special Sagarin abbreviations
	regexp.MustCompile(`^Ole Miss$`):         {"Mississippi"},                   // That's your name. Use it.
	regexp.MustCompile(`^UCF$`):              {"Central Florida(UCF)"},          // UCF
	regexp.MustCompile(`^USC$`):              {"Southern California"},           // USC
	regexp.MustCompile(`^Army$`):             {"Army West Point"},               // As if there were another.
	regexp.MustCompile(`^Nicholls$`):         {"Nicholls State"},                // Nicholls, not Nichols.
	regexp.MustCompile(`^UT Martin$`):        {"Tennessee-Martin"},              // Tennessee, not Texas.
	regexp.MustCompile(`^Florida `):          {"Fla. "},                         // Florida International
	regexp.MustCompile(`^Monmouth$`):         {"Monmouth-NJ", "Monmouth (YTM)"}, // _That_ Monmouth.
	regexp.MustCompile(`^McNeese$`):          {"McNeese State"},                 // McNeese
	regexp.MustCompile(`^Albany$`):           {"Albany-NY", "Albany (YTA)"},     // _That_ Albany.
	regexp.MustCompile(`^Prairie View$`):     {"Prairie View A&M"},              // I guess it's an A&M.
	regexp.MustCompile(`^South Carolina `):   {"SC "},                           // South Carolina State
	regexp.MustCompile(`^St Francis \(PA\)`): {"Saint Francis-Pa."},             // This is a mess in more ways than one.
	regexp.MustCompile(`^Cal Poly$`):         {"Cal Poly-SLO"},                  // Location
	regexp.MustCompile(`^Southern$`):         {"Southern U."},                   // Being more specific.
	regexp.MustCompile(`\bArkansas-`):        {"Ark.-"},                         // Pine Bluff
	regexp.MustCompile(`(?i)\s*\(MN\)`):      {"-Mn."},                          // St. Thomas
}

func replaceCommonAbbreviations(ss []string) []string {
	// defensive copy
	out := make([]string, len(ss))
	copy(out, ss)
	for _, s := range ss {
		for rx, rpls := range commonAbbreviations {
			if !rx.MatchString(s) {
				continue
			}
			for _, rpl := range rpls {
				out = append(out, rx.ReplaceAllString(s, rpl))
			}
		}
	}
	return out
}

func remove(ss []string, x string) []string {
	// defensive copy
	result := make([]string, len(ss))
	copy(result, ss)
	n := 0
	for _, s := range result {
		if s != x {
			result[n] = s
			n++
		}
	}
	result = result[:n]
	return result
}

func distinctStrings(ss []string) []string {
	// defensive copy
	result := make([]string, len(ss))
	copy(result, ss)
	distinct := make(map[string]struct{})
	n := 0
	for _, s := range result {
		if _, ok := distinct[s]; !ok {
			distinct[s] = struct{}{}
			result[n] = s
			n++
		}
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
	ab := sb.String()
	return ab[:4]
}

// toFirestore does not link the Venue--that has to be done with an external lookup.
func (t Team) toFirestore() firestore.Team {
	otherNames := make([]string, 0)
	otherNames = appendNonNilStrings(otherNames, t.AltName1, t.AltName2, t.AltName3, &t.School)
	otherNames = replaceCommonAbbreviations(otherNames)
	otherNames = distinctStrings(otherNames)
	colors := make([]string, 0)
	colors = appendNonNilStrings(colors, t.Color, t.AltColor)

	abbr := coalesceString(t.Abbreviation, strings.ToUpper(t.School))
	ft := firestore.Team{
		Abbreviation: abbr,
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

func (tc TeamCollection) EliminateNonContenders(gc GameCollection) TeamCollection {
	teams := make(map[int64]struct{})
	for _, g := range gc.games {
		teams[g.HomeID] = struct{}{}
		teams[g.AwayID] = struct{}{}
	}

	tOut := make([]Team, len(teams))
	fOut := make([]firestore.Team, len(teams))
	rOut := make([]*fs.DocumentRef, len(teams))
	iOut := make(map[int64]int)

	n := 0
	for id := range teams {
		io := tc.ids[id]
		tOut[n] = tc.teams[io]
		fOut[n] = tc.fsTeams[io]
		rOut[n] = tc.refs[io]
		iOut[id] = n
		n++
	}

	return TeamCollection{
		teams:   tOut,
		fsTeams: fOut,
		refs:    rOut,
		ids:     iOut,
	}
}

func GetTeams(client *http.Client, key string) (TeamCollection, error) {
	body, err := DoRequest(client, key, "https://api.collegefootballdata.com/teams")
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
