// SPDX-License-Identifier: MIT

package ssb

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/pkg/errors"
	"github.com/shurcooL/go-goon"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRef(t *testing.T) {
	a := assert.New(t)
	var tcases = []struct {
		ref  string
		err  error
		want Ref
	}{
		{"xxxx", ErrInvalidRef, nil},
		{"+xxx.foo", ErrInvalidHash, nil},
		{"@xxx.foo", ErrInvalidHash, nil},

		{"%wayTooShort.sha256", ErrInvalidHash, nil},
		{"&tooShort.sha256", NewHashLenError(6), nil},
		{"@tooShort.ed25519", NewFeedRefLenError(6), nil},
		{"&c29tZU5vbmVTZW5zZQo=.sha256", NewHashLenError(14), nil},

		{"@ye+QM09iPcDJD6YvQYjoQc7sLF/IFhmNbEqgdzQo3lQ=.ed25519", nil, &FeedRef{
			ID:   []byte{201, 239, 144, 51, 79, 98, 61, 192, 201, 15, 166, 47, 65, 136, 232, 65, 206, 236, 44, 95, 200, 22, 25, 141, 108, 74, 160, 119, 52, 40, 222, 84},
			Algo: RefAlgoFeedSSB1,
		}},

		// {"@ye+QM09iPcDJD6YvQYjoQc7sLF/IFhmNbEqgdzQo3lQ=.bamboo?", nil, &FeedRef{
		// 	ID:   []byte{201, 239, 144, 51, 79, 98, 61, 192, 201, 15, 166, 47, 65, 136, 232, 65, 206, 236, 44, 95, 200, 22, 25, 141, 108, 74, 160, 119, 52, 40, 222, 84},
		// 	Algo: RefAlgoFeed?????,
		// }},

		{"@ye+QM09iPcDJD6YvQYjoQc7sLF/IFhmNbEqgdzQo3lQ=.ggfeed-v1", nil, &FeedRef{
			ID:   []byte{201, 239, 144, 51, 79, 98, 61, 192, 201, 15, 166, 47, 65, 136, 232, 65, 206, 236, 44, 95, 200, 22, 25, 141, 108, 74, 160, 119, 52, 40, 222, 84},
			Algo: RefAlgoFeedGabby,
		}},

		{"&84SSLNv5YdDVTdSzN2V1gzY5ze4lj6tYFkNyT+P28Qs=.sha256", nil, &BlobRef{
			Hash: []byte{243, 132, 146, 44, 219, 249, 97, 208, 213, 77, 212, 179, 55, 101, 117, 131, 54, 57, 205, 238, 37, 143, 171, 88, 22, 67, 114, 79, 227, 246, 241, 11},
			Algo: RefAlgoBlobSSB1,
		}},

		{"%2jDrrJEeG7PQcCLcisISqarMboNpnwyfxLnwU1ijOjc=.sha256", nil, &MessageRef{
			Hash: []byte{218, 48, 235, 172, 145, 30, 27, 179, 208, 112, 34, 220, 138, 194, 18, 169, 170, 204, 110, 131, 105, 159, 12, 159, 196, 185, 240, 83, 88, 163, 58, 55},
			Algo: RefAlgoMessageSSB1,
		}},

		{"%2jDrrJEeG7PQcCLcisISqarMboNpnwyfxLnwU1ijOjc=.ggmsg-v1", nil, &MessageRef{
			Hash: []byte{218, 48, 235, 172, 145, 30, 27, 179, 208, 112, 34, 220, 138, 194, 18, 169, 170, 204, 110, 131, 105, 159, 12, 159, 196, 185, 240, 83, 88, 163, 58, 55},
			Algo: RefAlgoMessageGabby,
		}},
	}
	for i, tc := range tcases {
		r, err := ParseRef(tc.ref)
		if tc.err == nil {
			if !a.NoError(err, "got error on test %d (%v)", i, tc.ref) {
				continue
			}
			input := a.Equal(tc.ref, tc.want.Ref(), "test %d input<>output failed", i)
			want := a.Equal(tc.want.Ref(), r.Ref(), "test %d re-encode failed", i)
			if !input || !want {
				goon.Dump(r)
			}
			t.Log(i, r.ShortRef())
		} else {
			a.EqualError(errors.Cause(err), tc.err.Error(), "%d wrong error", i)
		}
	}
}

func TestStorageRef(t *testing.T) {
	a := assert.New(t)
	var tcases = []struct {
		input []byte
		want  string
		tipe  StorageRefType
		err   error
	}{
		{
			input: []byte("short"),
			err:   ErrRefLen{algo: "unknown", n: 5},
		},
		{
			input: append([]byte{0xff}, bytes.Repeat([]byte("beef"), 8)...),
			err:   ErrInvalidRefType,
		},
		{
			input: []byte{byte(StorageRefFeedLegacy), 201, 239, 144, 51, 79, 98, 61, 192, 201, 15, 166, 47, 65, 136, 232, 65, 206, 236, 44, 95, 200, 22, 25, 141, 108, 74, 160, 119, 52, 40, 222, 84},
			want:  "@ye+QM09iPcDJD6YvQYjoQc7sLF/IFhmNbEqgdzQo3lQ=.ed25519",
			tipe:  StorageRefFeedLegacy,
		},
		{
			input: []byte{byte(StorageRefMessageLegacy), 218, 48, 235, 172, 145, 30, 27, 179, 208, 112, 34, 220, 138, 194, 18, 169, 170, 204, 110, 131, 105, 159, 12, 159, 196, 185, 240, 83, 88, 163, 58, 55},
			want:  "%2jDrrJEeG7PQcCLcisISqarMboNpnwyfxLnwU1ijOjc=.sha256",
			tipe:  StorageRefMessageLegacy,
		},
		{
			input: []byte{byte(StorageRefMessageGabby), 218, 48, 235, 172, 145, 30, 27, 179, 208, 112, 34, 220, 138, 194, 18, 169, 170, 204, 110, 131, 105, 159, 12, 159, 196, 185, 240, 83, 88, 163, 58, 55},
			want:  "%2jDrrJEeG7PQcCLcisISqarMboNpnwyfxLnwU1ijOjc=.ggfeed-v1",
			tipe:  StorageRefMessageGabby,
		},
		{
			input: []byte{byte(StorageRefBlob), 243, 132, 146, 44, 219, 249, 97, 208, 213, 77, 212, 179, 55, 101, 117, 131, 54, 57, 205, 238, 37, 143, 171, 88, 22, 67, 114, 79, 227, 246, 241, 11},
			want:  "&84SSLNv5YdDVTdSzN2V1gzY5ze4lj6tYFkNyT+P28Qs=.sha256",
			tipe:  StorageRefBlob,
		},
	}
	for i, tc := range tcases {
		var r StorageRef

		err := r.Unmarshal(tc.input)

		if tc.err == nil {
			if !a.NoError(err, "got error on test %d", i) {
				continue
			}
			rt, err := r.valid()
			a.NoError(err)
			a.Equal(tc.tipe, rt, "failed on %s", tc.want)
			a.Equal(tc.want, r.Ref())

			// re-marshal
			out, err := r.Marshal()
			a.NoError(err)
			a.Equal(tc.input, out)
		} else {
			a.EqualError(errors.Cause(err), tc.err.Error(), "%d wrong error", i)
		}
	}
}

func TestParseBranches(t *testing.T) {
	r := require.New(t)

	var got struct {
		Refs MessageRefs `json:"refs"`
	}
	var input = []byte(`{
		"refs": "%HG1p299uO2nCenG6YwR3DG33lLpcALAS/PI6/BP5dB0=.sha256"
	}`)

	err := json.Unmarshal(input, &got)
	r.NoError(err)
	r.Equal(1, len(got.Refs))
	r.Equal(got.Refs[0].Ref(), "%HG1p299uO2nCenG6YwR3DG33lLpcALAS/PI6/BP5dB0=.sha256")

	var asArray = []byte(`{
		"refs": [
			"%hCM+q/zsq8vseJKwIAAJMMdsAmWeSfG9cs8ed3uOXCc=.sha256",
			"%yJAzwPO7HSjvHRp7wrVGO4sbo9GHSwKk0BXOSiUr+bo=.sha256"
		]
	}`)

	err = json.Unmarshal(asArray, &got)
	require.NoError(t, err)
	r.Equal(2, len(got.Refs))
	r.Equal(got.Refs[0].Ref(), `%hCM+q/zsq8vseJKwIAAJMMdsAmWeSfG9cs8ed3uOXCc=.sha256`)
	r.Equal(got.Refs[1].Ref(), `%yJAzwPO7HSjvHRp7wrVGO4sbo9GHSwKk0BXOSiUr+bo=.sha256`)

	var empty = []byte(`{
		"refs": []
	}`)

	err = json.Unmarshal(empty, &got)
	require.NoError(t, err)
	r.Equal(0, len(got.Refs))
}
