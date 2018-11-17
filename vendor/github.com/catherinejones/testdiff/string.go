package testdiff

import (
	"testing"
)

// StringIs compares two strings line by line and gives an error if they are not
// equal with the diff.
func StringIs(t *testing.T, a, b string) {
	t.Helper()

	if equal, err := CompareStrings(a, b); !equal {
		t.Errorf("strings are not identical: \n%s", err)
	}
}

// StringIsNot compares two strings line by line and gives an error if they are
// equal.
func StringIsNot(t *testing.T, a, b string) {
	t.Helper()

	if equal, _ := CompareStrings(a, b); equal {
		t.Errorf("no differences between a and b")
	}
}
