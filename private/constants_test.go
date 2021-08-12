// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package private

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupsSalt(t *testing.T) {
	h := sha256.New()
	fmt.Fprint(h, "envelope-dm-v1-extract-salt")

	require.Equal(t, dmSalt, h.Sum(nil))
}
