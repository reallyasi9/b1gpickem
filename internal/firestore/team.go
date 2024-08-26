package firestore

import (
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/firestore"
	"github.com/jedib0t/go-pretty/v6/list"
	"google.golang.org/api/iterator"
)

const TEAMS_COLLECTION = "teams"

// Team represents an NCAA football team.
type Team struct {
	// Abbreviation is a short, capitalized abbreviation of the team's name.
	// By convention, it is at most 4 characters long. There is no authoritative list of Name4 names,
	// but traditionally they have been chosen to match the abbreviated names that are used by ESPN.
	// Examples include:
	// - MICH (University of Michigan Wolverines)
	// - OSU (The Ohio State University Buckeyes)
	// - M-OH (Miami University of Ohio RedHawks)
	Abbreviation string `firestore:"abbreviation"`

	// ShortNames are capitalized abbreviations that Luke Heinkel has given to the team.
	// There is no authoritative list of these names, and they are not necessarily consistent over time (hence the array slice).
	// Examples include:
	// - MICH (University of Michigan Wolverines)
	// - OSU (The Ohio State University Buckeyes)
	// - CINCY (University of Cincinnati Bearcats)
	ShortNames []string `firestore:"short_names"`

	// OtherNames are the names that various other documents give to the team.
	// These are collected over time as various sports outlets call the team different official or unofficial names.
	// Examples include:
	// - [Michigan] (University of Michigan Wolverines)
	// - [Ohio St., Ohio State] (The Ohio State University Buckeyes)
	// - [Pitt, Pittsburgh] (University of Pittsburgh Panthers)
	OtherNames []string `firestore:"other_names,omitempty"`

	// School is the unofficial, unabbreviated name of the school used for display purposes.
	// Examples include:
	// - Michigan (University of Michigan Wolverines)
	// - Ohio State (The Ohio State University Buckeyes)
	// - Southern California (University of Southern California Trojans)
	School string `firestore:"school"`

	// Mascot is the official nickname of the team.
	// Examples include:
	// - Wolverines (University of Michigan Wolverines)
	// - Buckeyes (The Ohio State University Buckeyes)
	// - Chanticleers (Coastal Carolina Chanticleers)
	Mascot string `firestore:"mascot"`

	// Colors are the team colors in HTML RGB format ("#RRGGBB").
	Colors []string `firestore:"colors"`

	// Logos are links to logos, typically in size order (smallest first).
	Logos []string `firestore:"logos"`

	// Venue is a reference to a Venue document.
	Venue *firestore.DocumentRef `firestore:"venue"`
}

// NameType is an enumeration of types of team names (short, other, etc.)
type NameType int64

const (
	ShortName NameType = 0
	OtherName NameType = 1
)

func (n NameType) String() string {
	switch n {
	case ShortName:
		return "short"
	case OtherName:
		return "other"
	}
	return "unknown"
}

type DuplicateTeamNameError struct {
	// Name is the duplicate name detected
	Name string

	// NameType is the name type where the duplicate was found (e.g., "short", "other")
	NameType NameType

	// Teams are the Team structs where duplicates were detected (in the same order as Refs)
	Teams []Team

	// Refs are the references to the Team documents where duplicates were detected (in the same order as Teams)
	Refs []*firestore.DocumentRef
}

// Error fulfils error interface
func (err DuplicateTeamNameError) Error() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s (%d teams):\n", err.Name, len(err.Teams)))
	for _, t := range err.Teams {
		sb.WriteString(fmt.Sprintf("%s\n", t))
	}
	return fmt.Sprintf("duplicate team names of type %s detected: %v", err.NameType.String(), sb.String())
}

type NameNotFoundError struct {
	Name     string
	NameType NameType
}

// Error fulfills the error interface
func (s NameNotFoundError) Error() string {
	return fmt.Sprintf("team %s name '%s' not found", s.NameType, s.Name)
}

func (t Team) String() string {
	return fmt.Sprintf("%s %s", t.School, t.Mascot)
}

func (t Team) Pretty() string {
	l := list.NewWriter()
	l.SetStyle(list.StyleConnectedLight)
	l.AppendItem(fmt.Sprintf("%s (%s)", t.String(), t.Abbreviation))
	l.AppendItem("Other names")
	l.Indent()
	l.AppendItems([]interface{}{t.OtherNames})
	l.UnIndent()
	l.AppendItem("Short names")
	l.Indent()
	l.AppendItems([]interface{}{t.ShortNames})
	l.UnIndent()
	return l.Render()
}

// GetTeams returns a collection of teams for a given season.
func GetTeams(ctx context.Context, season *firestore.DocumentRef) ([]Team, []*firestore.DocumentRef, error) {
	iter := season.Collection(TEAMS_COLLECTION).Documents(ctx)
	teams := make([]Team, 0)
	refs := make([]*firestore.DocumentRef, 0)
	for {
		ss, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("error getting team snapshot: %w", err)
		}
		var t Team
		err = ss.DataTo(&t)
		if err != nil {
			return nil, nil, fmt.Errorf("error getting team snapshot data: %w", err)
		}
		teams = append(teams, t)
		refs = append(refs, ss.Ref)
	}
	return teams, refs, nil
}

// TeamRefsByName is a type for quick lookups of teams by other name.
type TeamRefsByName map[string]*firestore.DocumentRef

func NewTeamRefsByOtherName(teams []Team, refs []*firestore.DocumentRef) (TeamRefsByName, *DuplicateTeamNameError) {
	byName := make(TeamRefsByName)
	// Only return the first duplicate detected
	nameCatcher := make(map[string]int)
	var duplicates *DuplicateTeamNameError
	for i, t := range teams {
		for _, n := range t.OtherNames {
			if j, found := nameCatcher[n]; found {
				if duplicates == nil {
					duplicates = &DuplicateTeamNameError{
						Name:     n,
						NameType: OtherName,
						Teams:    []Team{t, teams[j]},
						Refs:     []*firestore.DocumentRef{refs[i], refs[j]},
					}
				} else {
					duplicates.Teams = append(duplicates.Teams, t)
					duplicates.Refs = append(duplicates.Refs, refs[i])
				}
			}
			nameCatcher[n] = i
			byName[n] = refs[i]
		}
		if duplicates != nil {
			return nil, duplicates
		}
	}
	return byName, nil
}

func NewTeamRefsByShortName(teams []Team, refs []*firestore.DocumentRef) (TeamRefsByName, *DuplicateTeamNameError) {
	byName := make(TeamRefsByName)
	// Only return the first duplicate detected
	nameCatcher := make(map[string]int)
	var duplicates *DuplicateTeamNameError
	for i, t := range teams {
		for _, n := range t.ShortNames {
			if j, found := nameCatcher[n]; found {
				if duplicates == nil {
					duplicates = &DuplicateTeamNameError{
						Name:     n,
						NameType: ShortName,
						Teams:    []Team{t, teams[j]},
						Refs:     []*firestore.DocumentRef{refs[i], refs[j]},
					}
				} else {
					duplicates.Teams = append(duplicates.Teams, t)
					duplicates.Refs = append(duplicates.Refs, refs[i])
				}
			}
			nameCatcher[n] = i
			byName[n] = refs[i]
		}
		if duplicates != nil {
			return nil, duplicates
		}
	}
	return byName, nil
}
