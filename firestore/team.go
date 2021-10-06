package firestore

import (
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

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

func (t Team) String() string {
	var sb strings.Builder
	sb.WriteString("Team\n")
	ss := make([]string, 0)
	ss = append(ss, treeString("Abbreviation", 0, false, t.Abbreviation))
	ss = append(ss, treeStringSlice("ShortNames", 0, false, t.ShortNames))
	ss = append(ss, treeStringSlice("OtherNames", 0, false, t.OtherNames))
	ss = append(ss, treeString("School", 0, false, t.School))
	ss = append(ss, treeString("Mascot", 0, false, t.Mascot))
	ss = append(ss, treeStringSlice("Colors", 0, false, t.Colors))
	ss = append(ss, treeStringSlice("Logos", 0, false, t.Logos))
	ss = append(ss, treeRef("Venue", 0, true, t.Venue))
	sb.WriteString(strings.Join(ss, "\n"))
	return sb.String()
}

// GetTeams returns a collection of teams for a given season.
func GetTeams(ctx context.Context, client *firestore.Client, season *firestore.DocumentRef) ([]Team, []*firestore.DocumentRef, error) {
	iter := season.Collection("teams").Documents(ctx)
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

func NewTeamRefsByOtherName(teams []Team, refs []*firestore.DocumentRef) TeamRefsByName {
	byName := make(TeamRefsByName)
	catcher := make(map[string]Team)
	duplicates := make(map[string][]Team)
	for i, t := range teams {
		for _, n := range t.OtherNames {
			if dd, ok := catcher[n]; ok {
				if _, found := duplicates[n]; !found {
					duplicates[n] = []Team{dd}
				}
				duplicates[n] = append(duplicates[n], t)
			}
			catcher[n] = t
			byName[n] = refs[i]
		}
	}
	if len(duplicates) != 0 {
		var sb strings.Builder
		for name, ts := range duplicates {
			sb.WriteString(fmt.Sprintf("%s (%d teams):\n", name, len(ts)))
			for _, t := range ts {
				sb.WriteString(fmt.Sprintf("%s\n", t))
			}
		}
		panic(fmt.Errorf("duplicate other names detected: %v", sb.String()))
	}
	return byName
}

func NewTeamRefsByShortName(teams []Team, refs []*firestore.DocumentRef) TeamRefsByName {
	byName := make(TeamRefsByName)
	catcher := make(map[string]Team)
	duplicates := make(map[string][]Team)
	for i, t := range teams {
		for _, n := range t.ShortNames {
			if dd, ok := catcher[n]; ok {
				if _, found := duplicates[n]; !found {
					duplicates[n] = []Team{dd}
				}
				duplicates[n] = append(duplicates[n], t)
			}
			catcher[n] = t
			byName[n] = refs[i]
		}
	}
	if len(duplicates) != 0 {
		var sb strings.Builder
		for name, ts := range duplicates {
			sb.WriteString(fmt.Sprintf("%s (%d teams):\n", name, len(ts)))
			for _, t := range ts {
				sb.WriteString(fmt.Sprintf("%s\n", t))
			}
		}
		panic(fmt.Errorf("duplicate short names detected: %v", sb.String()))
	}
	return byName
}
