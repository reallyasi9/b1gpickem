package bts

import "fmt"

// RelativeLocation describes where a game is being played relative to one team's home field.
type RelativeLocation int

const (
	// Home is a team's home field.
	Home RelativeLocation = 2

	// Near is a field closer to a given team's home field than the team's opponent's home field.
	Near RelativeLocation = 1

	// Neutral is a truely neutral location.
	Neutral RelativeLocation = 0

	// Far is a field closer to a given team's opponent's home field than the team's home field.
	Far RelativeLocation = -1

	// Away is a team's opponent's home field.
	Away RelativeLocation = -2
)

// Game represents a matchup between two teams.
type Game struct {
	team1    Team
	team2    Team
	location RelativeLocation
}

// NULLGAME represents a game that doesn't exsit.  Go figure.
var NULLGAME = Game{NONE, NONE, Neutral}

// NewGame makes a game between two teams.
func NewGame(team1, team2 Team, locRelTeam1 RelativeLocation) *Game {
	return &Game{team1: team1, team2: team2, location: locRelTeam1}
}

// Team returns a given team.
func (g *Game) Team(t int) Team {
	switch t {
	case 0:
		return g.team1
	case 1:
		return g.team2
	default:
		panic(fmt.Errorf("team %d is not a valid team", t))
	}
}

// LocationRelativeToTeam returns the location of the game relative to the given team.
func (g *Game) LocationRelativeToTeam(t int) RelativeLocation {
	switch t {
	case 0:
		return g.location
	case 1:
		return -g.location
	default:
		panic(fmt.Errorf("team %d is not a valid team", t))
	}
}

// SwapTeams swaps which team is team 1 and which team is team 2 in the game, swapping the location accordingly.
func (g *Game) SwapTeams() {
	g.team1, g.team2 = g.team2, g.team1
	g.location = -g.location
}

// EqualTo determines if to Game objects refer to the same actual matchup.
func (lhs *Game) EqualTo(rhs *Game) bool {
	if rhs == nil {
		return false
	}
	if lhs.team1 == rhs.team1 && lhs.team2 == rhs.team2 && lhs.location == rhs.location {
		return true
	}
	return lhs.team2 == rhs.team1 && lhs.team1 == rhs.team2 && lhs.location == -rhs.location
}
