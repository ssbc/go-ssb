package ssb

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/catherinejones/testdiff"
	"github.com/kylelemons/godebug/diff"
	"github.com/pkg/errors"
)

type testMessage struct {
	Hash  string
	Bytes []byte
}

var testMessages []testMessage

func init() {
	r, err := zip.OpenReader("testdata.zip")
	checkPanic(errors.Wrap(err, "could not find testdata - run 'node encode_test.js' to create it"))
	defer r.Close()

	if len(r.File)%2 != 0 {
		checkPanic(errors.New("expecting two files per message"))
	}

	testMessages = make([]testMessage, len(r.File)/2+1)

	seq := 1
	for i := 0; i < len(r.File); i += 2 {
		enc := r.File[i]
		orig := r.File[i+1]

		// check file structure assumption
		if enc.Name != fmt.Sprintf("%05d.encoded", seq) {
			checkPanic(errors.Errorf("unexpected file. wanted '%05d.encoded' got %s", seq, enc.Name))
		}

		if orig.Name != fmt.Sprintf("%05d.orig", seq) {
			checkPanic(errors.Errorf("unexpected file. wanted '%05d.orig' got %s", seq, orig.Name))
		}

		// only need the hash of the _original_ for now
		var origMsg struct {
			Key string
		}
		origRC, err := orig.Open()
		err = json.NewDecoder(origRC).Decode(&origMsg)
		checkPanic(errors.Wrapf(err, "test(%d) - could not json decode orig", i))
		testMessages[seq].Hash = origMsg.Key

		// copy encoded message
		rc, err := enc.Open()
		checkPanic(errors.Wrapf(err, "test(%d) - could not open data", i))
		testMessages[seq].Bytes, err = ioutil.ReadAll(rc)
		checkPanic(errors.Wrapf(err, "test(%d) - could not read all data", i))

		// cleanup
		checkPanic(errors.Wrapf(rc.Close(), "test(%d) - could not close reader #1", i))
		checkPanic(errors.Wrapf(origRC.Close(), "test(%d) - could not close reader #2", i))
		seq++
	}
}

func checkPanic(err error) {
	if err != nil {
		panic(err)
	}
}

func TestEncodeSimple(t *testing.T) {
	for i := 1; i < 20; i++ {
		tSimple(t, i)
	}
}

func tSimple(t *testing.T, i int) []byte {
	var smsg SignedMessage
	if err := json.Unmarshal(testMessages[i].Bytes, &smsg); err != nil {
		t.Logf("%s", testMessages[i].Bytes)
		t.Fatalf("json.Unmarshal failed: %s", err)
	}
	encoded, err := EncodeSimple(smsg)
	if err != nil {
		t.Fatalf("json.Unmarshal failed: %s", err)
	}
	return encoded
}

func TestPreserveOrder(t *testing.T) {
	for i := 1; i < 20; i++ {
		tPresve(t, i)
	}
}

func tPresve(t *testing.T, i int) []byte {
	encoded, err := EncodePreserveOrder(testMessages[i].Bytes)
	if err != nil {
		t.Errorf("EncodePreserveOrder(%d) failed:\n%+v", i, err)
	}
	return encoded
}

func TestCompareSimple(t *testing.T) {
	n := len(testMessages)
	if testing.Short() {
		n = 50
	}
	for i := 1; i < n; i++ {
		o := string(testMessages[i].Bytes)
		s := string(tSimple(t, i))
		testdiff.StringIs(t, o, s)
		if t.Failed() {
			t.Logf("Seq:%d\n%s", i, diff.Diff(o, s))
		}
	}
}

func TestComparePreserve(t *testing.T) {
	n := len(testMessages)
	if testing.Short() {
		n = 50
	}
	for i := 1; i < n; i++ {
		o := string(testMessages[i].Bytes)
		p := string(tPresve(t, i))
		testdiff.StringIs(t, o, p)
		if d := diff.Diff(o, p); len(d) != 0 && t.Failed() {
			t.Logf("Seq:%d\n%s", i, d)
		}
	}
}
