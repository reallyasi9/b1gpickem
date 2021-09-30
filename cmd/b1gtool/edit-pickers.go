package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/reallyasi9/b1gpickem/firestore"
)

// pickerFlagSet is a flag.FlagSet for parsing the edit-pickers subcommand.
var pickerFlagSet *flag.FlagSet

// season is the season to edit. If not supplied, the most recent defined season is used.
var season int

const shortDateFormat = "2006/01/02"

// addPickers implements the flag.Value interface
type addPickers struct {
	pickers *[]firestore.Picker
}

func (a addPickers) String() string {
	if a.pickers == nil {
		return "nil"
	}
	if len(*a.pickers) == 0 {
		return "none"
	}
	s := []string{}
	for _, v := range *a.pickers {
		s = append(s, fmt.Sprintf("%s:%s:%s", v.LukeName, v.Name, v.Joined.Format(shortDateFormat)))
	}
	return strings.Join(s, ",")
}

func (a addPickers) Set(val string) error {
	if val == "" {
		return errors.New("must specify picker to add in short_name[:full_name[:join_time]] format")
	}
	splits := strings.Split(val, ":")
	if len(splits) > 3 {
		return fmt.Errorf("too many fields in picker to add: expected <= 3, got %d", len(splits))
	}
	shortName := splits[0]
	fullName := shortName
	joinDate := time.Now()
	if len(splits) > 1 {
		fullName = splits[1]
	}
	if len(splits) > 2 {
		var err error
		joinDate, err = time.Parse("2006/01/02", splits[2])
		if err != nil {
			return err
		}
	}
	if a.pickers == nil {
		*a.pickers = make([]firestore.Picker, 0, 1)
	}
	p := firestore.Picker{
		LukeName: shortName,
		Name:     fullName,
		Joined:   joinDate,
	}
	*a.pickers = append(*a.pickers, p)
	return nil
}

var pickersToAdd []firestore.Picker

// rmPickers implements the flag.Value interface
type rmPickers []string

var pickersToRm rmPickers

// pickerUsage is the usage documentation for the edit-pickers subcommand.
func pickerUsage() {
	fmt.Fprintf(flag.CommandLine.Output(), `Usage: b1gtool [global-flags] edit-pickers [flags]
	
Edit pickers.
	
Flags:
`)

	pickerFlagSet.PrintDefaults()

	fmt.Fprint(flag.CommandLine.Output(), "\nGlobal Flags:\n")

	flag.PrintDefaults()

}

func init() {
	pickerFlagSet = flag.NewFlagSet("edit-pickers", flag.ExitOnError)
	pickerFlagSet.SetOutput(flag.CommandLine.Output())
	pickerFlagSet.Usage = pickerUsage

	pickerFlagSet.Var(&addPickers{&pickersToAdd}, "add", "Add `picker` by specifying short_name[:full_name[:join_date]]. The default full_name is short_name, and the default join_date is the date the program is run. Flag can be specified multiple times.")

	Commands["edit-pickers"] = editPickers
	Usage["edit-pickers"] = pickerUsage
}

func editPickers() {
	err := pickerFlagSet.Parse(flag.Args()[1:])
	if err != nil {
		log.Fatalf("Failed to parse edit-pickers arguments: %v", err)
	}
	// if modelFlagSet.NArg() < 2 {
	// 	modelFlagSet.Usage()
	// 	log.Fatal("Model short name and system name not supplied")
	// }
	// if modelFlagSet.NArg() > 2 {
	// 	modelFlagSet.Usage()
	// 	log.Fatal("Too many arguments supplied")
	// }

	// ctx := context.Background()
	// fsClient, err := fs.NewClient(ctx, ProjectID)
	// if err != nil {
	// 	log.Fatalf("Error creating firestore client: %v", err)
	// }
}
