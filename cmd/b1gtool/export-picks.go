package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"

	fs "cloud.google.com/go/firestore"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
	excelize "github.com/xuri/excelize/v2"
)

var exportFlags *flag.FlagSet

// exportLocation is the location where to export the picks.
var exportLocation string

func init() {
	cmd := "export-picks"

	exportFlags = flag.NewFlagSet(cmd, flag.ExitOnError)
	exportFlags.SetOutput(flag.CommandLine.Output())
	exportFlags.Usage = exportUsage

	exportFlags.StringVar(&exportLocation, "o", "", "The `location` where the slate will be exported in Excel format. If not given, prints picks to console. A Google Cloud Storage location can be specified with a gs:// prefix.")

	Commands[cmd] = exportPicks
	Usage[cmd] = exportUsage
}

func exportUsage() {
	fmt.Fprintf(flag.CommandLine.Output(), `Usage: b1gtool [global-flags] export-picks [flags] <season> <week> <picker>

Export a picker's weekly picks to file or Google Cloud Storage.

Arguments:
  season int
      Season of picks to export. If less than zero, season will be guessed based on today's date.
  week int
      Week number of picks to export. If less than zero, week will be guessed based on today's date.
  picker string
      Short name of picker to export.
	
Flags:
`)

	exportFlags.PrintDefaults()

	fmt.Fprint(flag.CommandLine.Output(), "\nGlobal Flags:\n")

	flag.PrintDefaults()
}

func exportPicks() {
	err := exportFlags.Parse(flag.Args()[1:])
	if err != nil {
		log.Fatalf("Failed to parse export-picks arguments: %v", err)
	}
	if exportFlags.NArg() != 3 {
		exportFlags.Usage()
		log.Fatal("Season, week, and picker arguments required.")
	}

	season, err := strconv.Atoi(exportFlags.Arg(0))
	if err != nil {
		exportFlags.Usage()
		log.Fatalf("Cannot parse season as integer, '%s' given", exportFlags.Arg(0))
	}
	week, err := strconv.Atoi(exportFlags.Arg(1))
	if err != nil {
		exportFlags.Usage()
		log.Fatalf("Cannot parse week as integer, '%s' given", exportFlags.Arg(1))
	}
	pickerName := exportFlags.Arg(2)

	ctx := context.Background()
	fsClient, err := fs.NewClient(ctx, ProjectID)
	if err != nil {
		log.Fatalf("Unable to create Firestore client: %v", err)
	}

	_, seasonRef, err := firestore.GetSeason(ctx, fsClient, season)
	if err != nil {
		log.Fatalf("Unable to get season %d: %v", season, err)
	}
	_, weekRef, err := firestore.GetWeek(ctx, seasonRef, week)
	if err != nil {
		log.Fatalf("Unable to get week %d: %v", week, err)
	}
	_, pickerRef, err := firestore.GetPickerByLukeName(ctx, fsClient, pickerName)
	if err != nil {
		log.Fatalf("Unable to get picker %s: %v", pickerName, err)
	}

	picks, picksRef, err := firestore.GetPicks(ctx, weekRef, pickerRef)
	if err != nil {
		log.Fatalf("Unable to get picks: %v", err)
	}

	btsPick, btsPickRef, err := firestore.GetStreakPick(ctx, weekRef, pickerRef)
	if err != nil {
		if _, ok := err.(firestore.NoStreakPickError); !ok {
			log.Fatalf("Unable to get streak pick: %v", err)
		}
	}

	// Make me some rows!
	xl, err := makePicksExcelFile(ctx, picks, picksRef, btsPick, btsPickRef)
	if err != nil {
		log.Fatalf("Unable to make pick rows: %v", err)
	}

	// Figure out output location
	if exportLocation == "" || DryRun {
		// no location? Print the rows to screen
		sheetName := xl.GetSheetName(xl.GetActiveSheetIndex())
		rows, err := xl.Rows(sheetName)
		if err != nil {
			log.Fatalf("Unable to get Excel row iterator: %v", err)
		}
		for rows.Next() {
			row, err := rows.Columns()
			if err != nil {
				log.Fatalf("Unable to get Excel cells from row iterator: %v", err)
			}
			fmt.Println(strings.Join(row, ", "))
		}
		return
	}

	writer, err := openFileOrGSWriter(ctx, exportLocation)
	if err != nil {
		log.Fatalf("Unable to open '%s': %v", exportLocation, err)
	}
	defer writer.Close()

	_, err = xl.WriteTo(writer)
	if err != nil {
		log.Fatalf("Unable to write Excel file: %v", err)
	}
}

func addRow(ctx context.Context, outExcel *excelize.File, sheetName string, row int, pick firestore.SlateRowBuilder) error {
	out, err := pick.BuildSlateRow(ctx)
	if err != nil {
		return fmt.Errorf("failed making game output: %v", err)
	}
	for col, str := range out {
		index, err := excelize.CoordinatesToCellName(col+1, row+1)
		if err != nil {
			return err
		}
		outExcel.SetCellStr(sheetName, index, str)
	}
	return nil
}

func addStreakOverRow(outExcel *excelize.File, sheetName string, row int) error {
	index, err := excelize.CoordinatesToCellName(1, row+1)
	if err != nil {
		return err
	}
	outExcel.SetCellStr(sheetName, index, "BEAT THE STREAK!")
	index, err = excelize.CoordinatesToCellName(3, row+1)
	if err != nil {
		return err
	}
	outExcel.SetCellStr(sheetName, index, "STREAK OVER!")
	return nil
}

func makePicksExcelFile(ctx context.Context, picks []firestore.Pick, pickRefs []*fs.DocumentRef, btsPick firestore.StreakPick, btsPickRef *fs.DocumentRef) (*excelize.File, error) {
	// Make an excel file in memory.
	outExcel := excelize.NewFile()
	sheetName := outExcel.GetSheetName(outExcel.GetActiveSheetIndex())
	// Write the header row
	outExcel.SetCellStr(sheetName, "A1", "GAME")
	outExcel.SetCellStr(sheetName, "B1", "Instruction")
	outExcel.SetCellStr(sheetName, "C1", "Your Selection")
	outExcel.SetCellStr(sheetName, "D1", "Predicted Spread")
	outExcel.SetCellStr(sheetName, "E1", "Notes")
	outExcel.SetCellStr(sheetName, "F1", "Expected Value")

	lastPickRow := -1 // need to calculate where the BTS row is
	firstSDRow := -1
	slateGames := make([]firestore.SlateGame, len(picks))

	for i, pick := range picks {
		snap, err := pick.SlateGame.Get(ctx)
		if err != nil {
			return nil, fmt.Errorf("unable to get SlateGame for pick: %v", err)
		}
		var sg firestore.SlateGame
		if err = snap.DataTo(&sg); err != nil {
			return nil, err
		}
		slateGames[i] = sg
		if sg.Superdog {
			if firstSDRow < 0 || sg.Row < firstSDRow {
				firstSDRow = sg.Row
			}
		} else {
			if lastPickRow < 0 || sg.Row > lastPickRow {
				lastPickRow = sg.Row
			}
		}
		if err = addRow(ctx, outExcel, sheetName, sg.Row, pick); err != nil {
			return nil, err
		}
	}

	// Between the picks and dogs, closer to the picks.
	btsRow := int(math.Ceil(float64(lastPickRow) + float64(firstSDRow-lastPickRow)/2.))
	if btsPickRef != nil {
		if err := addRow(ctx, outExcel, sheetName, btsRow, btsPick); err != nil {
			return nil, err
		}
	} else {
		if err := addStreakOverRow(outExcel, sheetName, btsRow); err != nil {
			return nil, err
		}
	}

	return outExcel, nil
}
