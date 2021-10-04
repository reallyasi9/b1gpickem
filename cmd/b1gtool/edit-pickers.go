package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/firestore"
)

// pickerFlagSet is a flag.FlagSet for parsing the edit-pickers subcommand.
var pickerFlagSet *flag.FlagSet

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
		return errors.New("must specify picker in short_name[:full_name[:join_time]] format")
	}
	splits := strings.Split(val, ":")
	if len(splits) > 3 {
		return fmt.Errorf("too many fields for picker: expected <= 3, got %d", len(splits))
	}
	shortName := splits[0]
	fullName := shortName
	joinDate := time.Now()
	if len(splits) > 1 && splits[1] != "" {
		fullName = splits[1]
	}
	if len(splits) > 2 && splits[2] != "" {
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
var pickersToEdit []firestore.Picker

// shortNames implements the flag.Value interface
type shortNames struct {
	pickers *[]string
}

func (s shortNames) String() string {
	if s.pickers == nil {
		return "nil"
	}
	if len(*s.pickers) == 0 {
		return "none"
	}
	return strings.Join(*s.pickers, ",")
}

func (s shortNames) Set(val string) error {
	if val == "" {
		return errors.New("must specify picker short_name")
	}
	if s.pickers == nil {
		*s.pickers = make([]string, 0, 1)
	}
	*s.pickers = append(*s.pickers, val)
	return nil
}

// pickersToDeactivate are the pickers that should be removed from a given season.
var pickersToDeactivate []string

// pickersToActivate are the pickers that should be added to a given season.
var pickersToActivate []string

// season is the season to which the pickers should be added.
var season int

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
	pickerFlagSet.Var(&addPickers{&pickersToEdit}, "edit", "Edit `picker` by specifying short_name[:full_name[:join_date]]. Fields not specified (or empty) will retain their values in Firestore. Flag can be specified multiple times.")
	pickerFlagSet.Var(&shortNames{&pickersToActivate}, "activate", "Activate `picker` for a given season. Specify picker by short_name. Flag can be specified multiple times.")
	pickerFlagSet.Var(&shortNames{&pickersToDeactivate}, "deactivate", "Deactivate `picker` from a given season. Specify picker by short_name. Flag can be specified multiple times.")
	pickerFlagSet.IntVar(&season, "season", 0, "Choose a `season` to activate or deactivate pickers. If equal to zero, the most recent season will be used.")

	Commands["edit-pickers"] = editPickers
	Usage["edit-pickers"] = pickerUsage
}

func editPickers() {
	err := pickerFlagSet.Parse(flag.Args()[1:])
	if err != nil {
		log.Fatalf("Failed to parse edit-pickers arguments: %v", err)
	}

	ctx := context.Background()
	fsClient, err := fs.NewClient(ctx, ProjectID)
	if err != nil {
		log.Fatalf("Unable to create firestore client: %v", err)
	}

	if err := pickerAdd(ctx, fsClient, pickersToAdd); err != nil {
		log.Fatalf("Unable to add pickers: %v", err)
	}

	if err := pickerEdit(ctx, fsClient, pickersToEdit); err != nil {
		log.Fatalf("Unable to edit pickers: %v", err)
	}

	if err := pickerActivate(ctx, fsClient, pickersToActivate, season); err != nil {
		log.Fatalf("Unable to activate pickers: %v", err)
	}

	if err := pickerDeactivate(ctx, fsClient, pickersToDeactivate, season); err != nil {
		log.Fatalf("Unable to deactivate pickers: %v", err)
	}

}

func pickerAdd(ctx context.Context, fsClient *fs.Client, pickers []firestore.Picker) error {
	if DryRun {
		log.Print("DRY RUN: would write the following:")
		for _, picker := range pickers {
			log.Printf("%s", picker)
		}
		return nil
	}

	pickerCol := fsClient.Collection("pickers")
	err := fsClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
		for _, picker := range pickers {
			ref := pickerCol.Doc(picker.LukeName)
			var err error
			if Force {
				err = t.Set(ref, &picker)
			} else {
				err = t.Create(ref, &picker)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})

	return err
}

func pickerEdit(ctx context.Context, fsClient *fs.Client, pickers []firestore.Picker) error {
	if DryRun {
		log.Print("DRY RUN: would edit the following:")
		for _, picker := range pickers {
			log.Printf("%s", picker)
		}
		return nil
	}

	pickerCol := fsClient.Collection("pickers")
	extantPickerRefs, err := pickerCol.DocumentRefs(ctx).GetAll()
	if err != nil {
		return err
	}
	err = fsClient.RunTransaction(ctx, func(c context.Context, t *fs.Transaction) error {
		pickerMap := make(map[string]firestore.Picker)
		refMap := make(map[string]*fs.DocumentRef)
		for _, ref := range extantPickerRefs {
			ss, err := t.Get(ref)
			if err != nil {
				return err
			}
			var p firestore.Picker
			if err = ss.DataTo(&p); err != nil {
				return err
			}
			if _, ok := pickerMap[p.LukeName]; ok {
				return fmt.Errorf("multiple pickers with the same LukeName \"%s\" defined", p.LukeName)
			}
			pickerMap[p.LukeName] = p
			refMap[p.LukeName] = ref
		}

		for _, picker := range pickers {
			var err error
			var ok bool
			var ref *fs.DocumentRef
			if ref, ok = refMap[picker.LukeName]; !ok {
				return fmt.Errorf("picker with LukeName \"%s\" not defined", picker.LukeName)
			}
			p := pickerMap[picker.LukeName]
			if picker.Name != "" {
				p.Name = picker.Name
			}
			var nulTime time.Time
			if p.Joined != nulTime {
				p.Joined = picker.Joined
			}
			err = t.Set(ref, &p)
			if err != nil {
				return err
			}
		}
		return nil
	})

	return err
}

func pickerActivate(ctx context.Context, fsClient *fs.Client, pickers []string, season int) error {
	s, sref, err := firestore.GetSeason(ctx, fsClient, season)
	if err != nil {
		return err
	}

	if DryRun {
		log.Printf("DRY RUN: would activate the following pickers for season %d:", s.Year)
		for _, picker := range pickers {
			log.Printf("%s", picker)
		}
		return nil
	}

	for _, picker := range pickers {
		var err error
		var ref *fs.DocumentRef
		if _, ref, err = firestore.GetPickerByLukeName(ctx, fsClient, picker); err != nil {
			return err
		}
		s.Pickers[picker] = ref
	}

	_, err = sref.Set(ctx, s)

	return err
}

func pickerDeactivate(ctx context.Context, fsClient *fs.Client, pickers []string, season int) error {
	s, sref, err := firestore.GetSeason(ctx, fsClient, season)
	if err != nil {
		return err
	}

	if DryRun {
		log.Printf("DRY RUN: would deactivate the following pickers for season %d:", s.Year)
		for _, picker := range pickers {
			log.Printf("%s", picker)
		}
		return nil
	}

	for _, picker := range pickers {
		delete(s.Pickers, picker)
	}

	_, err = sref.Set(ctx, s)

	return err
}
