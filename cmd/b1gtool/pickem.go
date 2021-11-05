package main

import (
	"github.com/reallyasi9/b1gpickem/internal/tools/pickem"
)

func init() {
	Commands[pickem.COMMAND] = pickem.Pickem
	Usage[pickem.COMMAND] = pickem.Usage
}
