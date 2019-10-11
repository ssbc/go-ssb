package repo

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCatchLockedError(t *testing.T) {
	var errStr = `failed to open KV: cannot access DB "testrun/TestRecoverFromCrash/sublogs/userFeeds/roaring/mkv": lock file "/home/cryptix/go/src/go.cryptoscope.co/ssb/cmd/go-sbot/testrun/TestRecoverFromCrash/sublogs/userFeeds/roaring/.3af2fe39bb7055138dbbafd36e23785d200b93cf" exists`

	if !lockFileExistsRe.MatchString(errStr) {
		t.Fail()
	}

	files := lockFileExistsRe.FindStringSubmatch(errStr)
	require.Len(t, files, 3)

	require.Equal(t, files[1], "testrun/TestRecoverFromCrash/sublogs/userFeeds/roaring/mkv")
	require.Equal(t, files[2], "/home/cryptix/go/src/go.cryptoscope.co/ssb/cmd/go-sbot/testrun/TestRecoverFromCrash/sublogs/userFeeds/roaring/.3af2fe39bb7055138dbbafd36e23785d200b93cf")
}
