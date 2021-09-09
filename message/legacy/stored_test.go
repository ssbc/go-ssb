// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package legacy

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	refs "go.mindeco.de/ssb-refs"
)

func TestAbstractStored(t *testing.T) {
	r := require.New(t)

	var m StoredMessage
	m.Author_ = storedrefs.SerialzedFeed{
		FeedRef: testMessages[1].Author,
	}
	m.Raw_ = testMessages[1].Input

	var a refs.Message = m
	c := a.ContentBytes()
	r.NotNil(c)
	r.True(len(c) > 0)

	var contentMap map[string]interface{}
	err := json.Unmarshal(c, &contentMap)
	r.NoError(err)
	r.NotNil(contentMap["type"])

	author := a.Author()
	r.True(m.Author_.Equal(author))

}
