package ssb

import (
	"reflect"
	"testing"
)

func TestParseRef(t *testing.T) {
	var tcases = []struct {
		ref  string
		err  error
		want *Ref
	}{
		{"xxxx", ErrMalformedRef, nil},
		{"+xxx.foo", ErrInvalidRefType, nil},
		{"@xxx.foo", ErrInvalidRefAlgo, nil},
		{"&YWJj.sha256", nil, &Ref{RefBlob, "abc", RefAlgoSHA256}},
		{"%YWJj.sha256", nil, &Ref{RefMessage, "abc", RefAlgoSHA256}},
		{"@YWJj.ed25519", nil, &Ref{RefFeed, "abc", RefAlgoEd25519}},
	}
	for i, tc := range tcases {
		r, err := ParseRef(tc.ref)
		if err != tc.err {
			t.Errorf("%03d: Expected err:%v got:%v", i, tc.err, err)
		}
		if !reflect.DeepEqual(r, tc.want) {
			t.Errorf("%03d: Expected Ref:%+v got:%+v", i, tc.want, r)
		}
	}
}
