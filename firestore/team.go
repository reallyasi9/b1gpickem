package firestore

// Team represents an NCAA football team.
type Team struct {
	// Name4 is a short, capitalized abbreviation of the team's name.
	// By convention, it is at most 4 characters long. There is no authoritative list of Name4 names,
	// but traditionally they have been chosen to match the abbreviated names that are used by ESPN.
	// Examples include:
	// - MICH (University of Michigan Wolverines)
	// - OSU (The Ohio State University Buckeyes)
	// - M-OH (Miami University of Ohio RedHawks)
	Name4 string `firestore:"name_4"`

	// LukeName are capitalized abbreviations that Luke Henkel has given to the team.
	// There is no authoritative list of these names, and they are not necessarily consistent over time (hence the array slice).
	// Examples include:
	// - MICH (University of Michigan Wolverines)
	// - OSU (The Ohio State University Buckeyes)
	// - CINCY (University of Cincinnati Bearcats)
	LukeNames []string `firestore:"name_luke,omitempty"`

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

	// Name is the official nickname of the team.
	// Examples include:
	// - Wolverines (University of Michigan Wolverines)
	// - Buckeyes (The Ohio State University Buckeyes)
	// - Chanticleers (Coastal Carolina Chanticleers)
	Name string `firestore:"team"`
}
