package message

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAbstractStored(t *testing.T) {
	r := require.New(t)

	var m StoredMessage
	m.Author = testMessages[1].Author
	m.Raw = testMessages[1].Input
	t.Log(string(m.Raw))
	a, ok := interface{}(m).(Abstract)
	r.True(ok)

	c := a.GetContent()
	r.NotNil(c)
	r.True(len(c) > 0)

	var contentMap map[string]interface{}
	err := json.Unmarshal(c, &contentMap)
	r.NoError(err)
	r.NotNil(contentMap["type"])

	author := a.GetAuthor()
	r.Equal(m.Author.ID, author.ID)
}
