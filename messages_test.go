package ssb

import (
	"bytes"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnyRef(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)
	var tcases = []struct {
		ref  string
		want Ref
	}{

		{"@ye+QM09iPcDJD6YvQYjoQc7sLF/IFhmNbEqgdzQo3lQ=.ed25519", &FeedRef{
			ID:   []byte{201, 239, 144, 51, 79, 98, 61, 192, 201, 15, 166, 47, 65, 136, 232, 65, 206, 236, 44, 95, 200, 22, 25, 141, 108, 74, 160, 119, 52, 40, 222, 84},
			Algo: RefAlgoFeedSSB1,
		}},

		{"%AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=.sha256", &MessageRef{
			Hash: bytes.Repeat([]byte{0}, 32),
			Algo: RefAlgoMessageSSB1,
		}},

		{"&84SSLNv5YdDVTdSzN2V1gzY5ze4lj6tYFkNyT+P28Qs=.sha256", &BlobRef{
			Hash: []byte{243, 132, 146, 44, 219, 249, 97, 208, 213, 77, 212, 179, 55, 101, 117, 131, 54, 57, 205, 238, 37, 143, 171, 88, 22, 67, 114, 79, 227, 246, 241, 11},
			Algo: RefAlgoBlobSSB1,
		}},

		{"%2jDrrJEeG7PQcCLcisISqarMboNpnwyfxLnwU1ijOjc=.sha256", &MessageRef{
			Hash: []byte{218, 48, 235, 172, 145, 30, 27, 179, 208, 112, 34, 220, 138, 194, 18, 169, 170, 204, 110, 131, 105, 159, 12, 159, 196, 185, 240, 83, 88, 163, 58, 55},
			Algo: RefAlgoMessageSSB1,
		}},

		{"%2jDrrJEeG7PQcCLcisISqarMboNpnwyfxLnwU1ijOjc=.ggmsg-v1", &MessageRef{
			Hash: []byte{218, 48, 235, 172, 145, 30, 27, 179, 208, 112, 34, 220, 138, 194, 18, 169, 170, 204, 110, 131, 105, 159, 12, 159, 196, 185, 240, 83, 88, 163, 58, 55},
			Algo: RefAlgoMessageGabby,
		}},
	}
	for i, tc := range tcases {

		var testPost Post
		testPost.Type = "test"
		testPost.Text = strconv.Itoa(i)
		testPost.Mentions = []Mention{
			NewMention(tc.want, "test"),
		}

		body, err := json.Marshal(testPost)
		r.NoError(err)

		var gotPost Post
		err = json.Unmarshal(body, &gotPost)
		r.NoError(err, "case %d unmarshal", i)

		r.Len(gotPost.Mentions, 1)
		a.Equal(tc.want.Ref(), gotPost.Mentions[0].Link.Ref(), "test %d re-encode failed", i)

		if i == 2 {
			br, ok := gotPost.Mentions[0].Link.IsBlob()
			a.True(ok, "not a blob?")
			a.NotNil(br)
		}
	}
}
