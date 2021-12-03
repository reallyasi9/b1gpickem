package main

import (
	"github.com/alecthomas/kong"
)

type globalCmd struct {
	ProjectID string `help:"GCP project ID." env:"GCP_PROJECT" required:""`
}

var CLI struct {
	globalCmd

	Pickers struct {
		Add        addPickersCmd        `cmd:"" help:"Add pickers."`
		Rm         rmPickersCmd         `cmd:"" help:"Remove pickers."`
		Ls         lsPickersCmd         `cmd:"" help:"List all pickers."`
		Edit       editPickerCmd        `cmd:"" help:"Edit picker."`
		Activate   activatePickersCmd   `cmd:"" help:"Activate pickers for a season."`
		Deactivate deactivatePickersCmd `cmd:"" help:"Deactivate pickers for a season."`
	} `cmd:""`

	Teams struct {
		Edit editTeamCmd `cmd:"" help:"Edit team."`
	} `cmd:""`

	Season struct {
		Setup     setupSeasonCmd `cmd:"" help:"Setup season."`
		SplitWeek splitWeekCmd   `cmd:"" help:"Split week based on time of kickoff."`
	} `cmd:""`

	Models struct {
		Update         updateModelsCmd   `cmd:"" help:"Update models."`
		GetPredictions getPredictionsCmd `cmd:"" help:"Get model predictions."`
		UpdateSagarin  updateSagarinCmd  `cmd:"" help:"Update Sagarin points."`
		Add            addModelsCmd      `cmd:"" help:"Add new model information."`
		Rm             rmModelsCmd       `cmd:"" help:"Remove model."`
		Ls             lsModelsCmd       `cmd:"" help:"List all models."`
	} `cmd:""`

	Slate struct {
		Parse parseSlateCmd `cmd:"" help:"Parse official slate."`
	} `cmd:""`

	Picks struct {
		Pickem pickemCmd      `cmd:"" help:"Make picks."`
		Export exportPicksCmd `cmd:"" help:"Export picks."`
	} `cmd:""`
}

func main() {
	ctx := kong.Parse(&CLI)
	err := ctx.Run(&CLI.globalCmd)
	ctx.FatalIfErrorf(err)
}
