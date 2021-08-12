// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package repo

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCatchLockedError(t *testing.T) {
	var input = []string{`failed to open KV: cannot access DB "testrun/TestRecoverFromCrash/sublogs/userFeeds/roaring/mkv": lock file "/home/cryptix/go/src/go.cryptoscope.co/ssb/cmd/go-sbot/testrun/TestRecoverFromCrash/sublogs/userFeeds/roaring/.3af2fe39bb7055138dbbafd36e23785d200b93cf" exists`,

		`cannot access DB "/var/mobile/Containers/Data/Application/1463B2ED-A58B-42D4-9BA4-BCBBB40BB716/Library/Application Support/FBTT/d4a1cb88a66f02f8db635ce26441cc5dac1b08420ceaac230839b755845a9ffb/GoSbot/sublogs/privates/roaring/mkv": lock file "/var/mobile/Containers/Data/Application/1463B2ED-A58B-42D4-9BA4-BCBBB40BB716/Library/Application Support/FBTT/d4a1cb88a66f02f8db635ce26441cc5dac1b08420ceaac230839b755845a9ffb/GoSbot/sublogs/privates/roaring/.3af2fe39bb7055138dbbafd36e23785d200b93cf" exists`}

	for _, errStr := range input {

		if !lockFileExistsRe.MatchString(errStr) {
			t.Fail()
		}

		files := lockFileExistsRe.FindStringSubmatch(errStr)
		require.Len(t, files, 3)
	}

	//require.Equal(t, files[1], "testrun/TestRecoverFromCrash/sublogs/userFeeds/roaring/mkv")
	//require.Equal(t, files[2], "/home/cryptix/go/src/go.cryptoscope.co/ssb/cmd/go-sbot/testrun/TestRecoverFromCrash/sublogs/userFeeds/roaring/.3af2fe39bb7055138dbbafd36e23785d200b93cf")
}
