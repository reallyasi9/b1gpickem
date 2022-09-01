package pickem

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"strings"

	fs "cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
	"github.com/reallyasi9/b1gpickem/internal/firestore"
	excelize "github.com/xuri/excelize/v2"
)

func ExportPicks(ctx *Context) error {
	_, seasonRef, err := firestore.GetSeason(ctx, ctx.FirestoreClient, ctx.Season)
	if err != nil {
		return fmt.Errorf("ExportPicks: failed to get season %d: %w", ctx.Season, err)
	}
	_, weekRef, err := firestore.GetWeek(ctx, seasonRef, ctx.Week)
	if err != nil {
		return fmt.Errorf("ExportPicks: failed to get week %d: %w", ctx.Week, err)
	}
	_, pickerRef, err := firestore.GetPickerByLukeName(ctx, ctx.FirestoreClient, ctx.Picker)
	if err != nil {
		return fmt.Errorf("ExportPicks: failed to get picker %s: %w", ctx.Picker, err)
	}

	picks, picksRef, err := firestore.GetPicks(ctx, weekRef, pickerRef)
	if err != nil {
		return fmt.Errorf("ExportPicks: failed to get picks: %w", err)
	}

	btsPick, btsPickRef, err := firestore.GetStreakPick(ctx, weekRef, pickerRef)
	if err != nil {
		if _, ok := err.(firestore.NoStreakPickError); !ok {
			return fmt.Errorf("ExportPicks: failed to get streak pick: %w", err)
		}
	}

	// Make me some rows!
	xl, err := makePicksExcelFile(ctx, picks, picksRef, btsPick, btsPickRef)
	if err != nil {
		return fmt.Errorf("ExportPicks: failed to make pick rows: %w", err)
	}

	// Figure out output location
	if ctx.Output == "" || ctx.DryRun {
		// no location? Print the rows to screen
		sheetName := xl.GetSheetName(xl.GetActiveSheetIndex())
		rows, err := xl.Rows(sheetName)
		if err != nil {
			return fmt.Errorf("ExportPicks: failed to get Excel row iterator: %w", err)
		}
		for rows.Next() {
			row, err := rows.Columns()
			if err != nil {
				return fmt.Errorf("ExportPicks: failed to get Excel cells from row iterator: %w", err)
			}
			fmt.Println(strings.Join(row, ", "))
		}
		return nil
	}

	writer, err := openFileOrGSWriter(ctx, ctx.Output)
	if err != nil {
		return fmt.Errorf("ExportPicks: failed to open '%s': %w", ctx.Output, err)
	}
	defer writer.Close()

	_, err = xl.WriteTo(writer)
	if err != nil {
		return fmt.Errorf("ExportPicks: failed to write Excel file: %w", err)
	}

	return nil
}

func addRow(ctx context.Context, outExcel *excelize.File, sheetName string, row int, pick firestore.SlateRowBuilder) error {
	out, err := pick.BuildSlateRow(ctx)
	if err != nil {
		return fmt.Errorf("failed making game output: %w", err)
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
			return nil, fmt.Errorf("unable to get SlateGame for pick: %w", err)
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
	if firstSDRow < 0 {
		// In case there are no SDs.
		firstSDRow = lastPickRow + 1
	}
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

func openFileOrGSWriter(ctx context.Context, f string) (io.WriteCloser, error) {
	u, err := url.Parse(f)
	if err != nil {
		return nil, err
	}
	var w io.WriteCloser
	switch u.Scheme {
	case "gs":
		gsClient, err := storage.NewClient(ctx)
		if err != nil {
			return nil, err
		}
		bucket := gsClient.Bucket(u.Host)
		// URL path has leading slash, but GS expects path relative to bucket.
		path := strings.TrimPrefix(u.Path, "/")
		obj := bucket.Object(path)
		w = obj.NewWriter(ctx)

	case "file":
		fallthrough
	case "":
		w, err = os.Create(u.Path)
		if err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unable to determine how to open '%s'", f)
	}

	return w, nil
}
