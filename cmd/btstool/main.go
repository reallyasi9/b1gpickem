package main

import "github.com/alecthomas/kong"

type globalCmd struct {
	ProjectID string `help:"GCP project ID." env:"GCP_PROJECT" required:""`
	DryRun    bool   `help:"Print database writes to log and exit without writing." xor:"Force,DryRun"`
	Force     bool   `help:"Force overwriting or deleting data in database." xor:"Force,DryRun"`
}

var CLI struct {
	globalCmd

	Teams struct {
		Add addTeamsCmd `cmd:"" help:"Add teams to competition."`
		Rm  rmTeamsCmd  `cmd:"" help:"Remove teams from competition."`
		Ls  lsTeamsCmd  `cmd:"" help:"List all teams in competition."`
	} `cmd:""`

	Streakers struct {
		Activate   activateStreakersCmd   `cmd:"" help:"Activate streakers."`
		Deactivate deactivateStreakersCmd `cmd:"" help:"Deactivate streakers."`
		Ls         lsStreakersCmd         `cmd:"" help:"List all streakers."`
	} `cmd:""`

	WeekTypes setupWeeksCmd `cmd:"" help:"Setup week types available in competition."`

	Pick makePickCmd `cmd:"" help:"Make streak picks."`

	Status statusCmd `cmd:"" help:"Show status of all streaks."`

	Simulate struct {
		Anneal     annealCmd     `cmd:"" help:"Perform simulated annealing to approximate the best choice among all possible streaks."`
		BruteForce bruteForceCmd `cmd:"" help:"Perform exhaustive search for the best choice over all possible streaks."`
		Enumerate  enumerateCmd  `cmd:"" help:"Enumerate all possible streaks."`
	}
}

func main() {
	ctx := kong.Parse(&CLI)
	err := ctx.Run(&CLI.globalCmd)
	ctx.FatalIfErrorf(err)
}
