package testutils

import (
	"os"
	"testing"
)

func RunsOnCI() bool {
	return os.Getenv("RUNNING_ON_CI") == "YES"
}

func SkipOnCI(t *testing.T) bool {
	yes := RunsOnCI()
	if yes {
		t.Log("running on CI, skipping this test.")
	}
	return yes
}
