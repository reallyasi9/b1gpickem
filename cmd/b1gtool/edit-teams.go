package main

import "github.com/reallyasi9/b1gpickem/internal/tools/editteams"

func init() {
	editteams.InitializeSubcommand()
	Commands[editteams.COMMAND] = editteams.EditTeams
	Usage[editteams.COMMAND] = editteams.Usage
}
