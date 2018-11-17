package testdiff

import (
	"io/ioutil"
	"testing"
)

// loadTestFile loads a file for testing against, and returns a string or
// an error.
func loadTestFile(filename string) (string, error) {
	b, err := ioutil.ReadFile(filename)
	if nil != err {
		return "", err
	}

	return string(b), nil
}

// StringIsFile compares a string to the file at the passed filename line by line
// and gives an error if they are not equal with the diff.
func StringIsFile(t *testing.T, filename, b string) {
	t.Helper()

	a, err := loadTestFile(filename)
	if nil != err {
		t.Errorf("could not open test file: %s", err)
		return
	}

	StringIs(t, a, b)
}

// StringIsNotFile compares a string to the file at the passed filename line by line
// and gives an error if they are equal.
func StringIsNotFile(t *testing.T, filename, b string) {
	t.Helper()

	a, err := loadTestFile(filename)
	if nil != err {
		t.Errorf("could not open test file: %s", err)
		return
	}

	StringIsNot(t, a, b)
}
