package testdiff

import (
	"errors"

	"github.com/sergi/go-diff/diffmatchpatch"
)

const (
	extraNewline   = "<missing newline in first string>"
	missingNewline = "<no newline in second string>"
)

// CompareStrings compares two strings line by line and returns true if
// they are equal. Returns false and an error with the lines that failed if
// not.
func CompareStrings(a, b string) (bool, error) {
	dmp := diffmatchpatch.New()

	isDifferent := false

	diffs := dmp.DiffMain(a, b, false)

	for _, diff := range diffs {
		if diff.Type != diffmatchpatch.DiffEqual {
			isDifferent = true
			break
		}
	}

	if isDifferent {
		output := dmp.DiffPrettyText(diffs)
		lastDiff := diffs[len(diffs)-1]
		if lastDiff.Text == "\n" {
			if diffmatchpatch.DiffInsert == lastDiff.Type {
				output += extraNewline
			} else if diffmatchpatch.DiffDelete == lastDiff.Type {
				output += missingNewline
			}
		}

		err := errors.New(output)
		return false, err
	}

	return true, nil
}
