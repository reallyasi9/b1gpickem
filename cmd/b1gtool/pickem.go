package main

import (
	"github.com/reallyasi9/b1gpickem/internal/tools/pickem"
)

func init() {
	pickem.InitializeSubcommand()
	Commands[pickem.COMMAND] = pickem.Pickem
	Usage[pickem.COMMAND] = pickem.Usage
}
