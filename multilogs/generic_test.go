package multilogs

import (
	"encoding/json"
	"testing"

	"go.cryptoscope.co/ssb/message"

	"github.com/stretchr/testify/require"
)

func mustBytes(bs []byte, err error) []byte {
	if err != nil {
		panic(err)
	}

	return bs
}

func TestGenericExtractor(t *testing.T) {
	type testCase struct {
		name   string
		x      genericExtractor
		value  interface{}
		result map[string]string
		err    string
	}

	mkTest := func(tc testCase) func(*testing.T) {
		return func(t *testing.T) {
			r := require.New(t)

			result, err := tc.x(tc.value)

			if tc.err != "" {
				t.Log(err)
				r.EqualError(err, tc.err, "extract error")
				return
			}

			r.NoError(err, "extract error")
			r.Equal(tc.result, result, "extract result")
		}
	}

	var tcs = []testCase{
		{
			name: "toplevel",
			x:    genericExtractor(TopLevelExtract),
			value: &message.StoredMessage{
				Raw: mustBytes(json.Marshal(map[string]interface{}{
					"content": map[string]interface{}{
						"number":  23,
						"string1": "foo",
						"object": map[string]interface{}{
							"nestedString": "illegal",
						},
						"string2": "bar",
						"array": []string{
							"arrayString1",
							"arrayString2",
							"arrayString3",
						},
					},
				})),
			},
			result: map[string]string{
				"string1": "foo",
				"string2": "bar",
			},
		},
		{
			name: "composed",
			x: NewStoredMessageRawExtractor(
				NewJSONDecodeToMapExtractor(
					NewTraverseExtractor([]string{"content"},
						genericExtractor(StringsExtractor)))),
			value: &message.StoredMessage{
				Raw: mustBytes(json.Marshal(map[string]interface{}{
					"content": map[string]interface{}{
						"number":  23,
						"string1": "foo",
						"object": map[string]interface{}{
							"nestedString": "illegal",
						},
						"string2": "bar",
						"array": []string{
							"arrayString1",
							"arrayString2",
							"arrayString3",
						},
					},
				})),
			},
			result: map[string]string{
				"string1": "foo",
				"string2": "bar",
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, mkTest(tc))
	}
}
