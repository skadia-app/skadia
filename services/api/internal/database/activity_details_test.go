package database

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestInsertLapsColumnAlignment is a regression test for the bug where
// InsertLaps had $117–$124 in VALUES with no corresponding column names,
// causing SQLSTATE 42601 on every recompute.
//
// It parses activity_details.go and verifies:
//
//	column_count == max_$N_placeholder
//
// When adding a new lap column: update BOTH the INSERT column list AND the
// VALUES placeholder list. Missing one causes runtime SQL errors.
func TestInsertLapsColumnAlignment(t *testing.T) {
	src, err := os.ReadFile("activity_details.go")
	if err != nil {
		t.Fatalf("cannot read activity_details.go: %v", err)
	}
	content := string(src)

	const insertMarker = "INSERT INTO activity_laps ("
	insertPos := strings.Index(content, insertMarker)
	if insertPos < 0 {
		t.Fatal("INSERT INTO activity_laps not found in activity_details.go")
	}

	const valuesMarker = ") VALUES ("
	valuesPos := strings.Index(content[insertPos:], valuesMarker)
	if valuesPos < 0 {
		t.Fatal(") VALUES ( not found after INSERT INTO activity_laps")
	}

	// Column text: between the opening ( and ) VALUES (
	colText := content[insertPos+len(insertMarker) : insertPos+valuesPos]

	// Strip SQL line comments
	commentRe := regexp.MustCompile(`--[^\n]*`)
	colClean := commentRe.ReplaceAllString(colText, "")

	colCount := 0
	for _, tok := range strings.Split(colClean, ",") {
		if strings.TrimSpace(tok) != "" {
			colCount++
		}
	}

	// VALUES text: between ) VALUES ( and the closing backtick line
	valStart := insertPos + valuesPos + len(valuesMarker)
	const endMarker = "\n\t\t\t)`,"
	endPos := strings.Index(content[valStart:], endMarker)
	if endPos < 0 {
		t.Fatal("closing backtick of InsertLaps SQL not found — marker changed?")
	}
	valText := content[valStart : valStart+endPos]

	paramRe := regexp.MustCompile(`\$(\d+)`)
	maxParam := 0
	for _, m := range paramRe.FindAllStringSubmatch(valText, -1) {
		n, _ := strconv.Atoi(m[1])
		if n > maxParam {
			maxParam = n
		}
	}

	if colCount != maxParam {
		t.Errorf(
			"InsertLaps misaligned: %d column names but max placeholder is $%d\n"+
				"When adding a lap column update BOTH the column list AND the VALUES list.",
			colCount, maxParam,
		)
	}
}
