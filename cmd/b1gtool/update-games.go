package main

import (
	"github.com/reallyasi9/b1gpickem/internal/tools/updategames"
)

func init() {
	updategames.InitializeSubcommand()
	Commands[updategames.COMMAND] = updategames.UpdateGames
	Usage[updategames.COMMAND] = updategames.Usage
}
