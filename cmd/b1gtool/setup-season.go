package main

import "github.com/reallyasi9/b1gpickem/internal/tools/setupseason"

func init() {
	setupseason.InitializeSubcommand()
	Commands[setupseason.COMMAND] = setupseason.SetupSeason
	Usage[setupseason.COMMAND] = setupseason.Usage
}
