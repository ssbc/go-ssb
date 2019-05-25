package legacy

import (
	"encoding/json"
	"testing"

	"go.cryptoscope.co/ssb"

	"github.com/stretchr/testify/require"
)

func TestAbstractStored(t *testing.T) {
	r := require.New(t)

	var m StoredMessage
	m.Author_ = testMessages[1].Author
	m.Raw_ = testMessages[1].Input

	var a ssb.Message = m
	c := a.ContentBytes()
	r.NotNil(c)
	r.True(len(c) > 0)

	var contentMap map[string]interface{}
	err := json.Unmarshal(c, &contentMap)
	r.NoError(err)
	r.NotNil(contentMap["type"])

	author := a.Author()
	r.Equal(m.Author_.ID, author.ID)
}
