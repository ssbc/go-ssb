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
	Hash, Signature string
	Input, Want     []byte
}

var testMessages []testMessage

func init() {
	r, err := zip.OpenReader("testdata.zip")
	checkPanic(errors.Wrap(err, "could not find testdata - run 'node encode_test.js' to create it"))
	defer r.Close()

	if len(r.File)%3 != 0 {
		checkPanic(errors.New("expecting three files per message"))
	}

	testMessages = make([]testMessage, len(r.File)/3+1)

	seq := 1
	for i := 0; i < len(r.File); i += 3 {
		full := r.File[i]
		input := r.File[i+1]
		want := r.File[i+2]
		// check file structure assumption
		if want.Name != fmt.Sprintf("%05d.want", seq) {
			checkPanic(errors.Errorf("unexpected file. wanted '%05d.want' got %s", seq, want.Name))
		}
		if input.Name != fmt.Sprintf("%05d.input", seq) {
			checkPanic(errors.Errorf("unexpected file. wanted '%05d.input' got %s", seq, input.Name))
		}
		if full.Name != fmt.Sprintf("%05d.full", seq) {
			checkPanic(errors.Errorf("unexpected file. wanted '%05d.full' got %s", seq, full.Name))
		}
		// only need the hash of the _original_ for now
		var origMsg struct {
			Key   string
			Value map[string]interface{}
		}
		origRC, err := full.Open()
		err = json.NewDecoder(origRC).Decode(&origMsg)
		checkPanic(errors.Wrapf(err, "test(%d) - could not json decode full", i))
		testMessages[seq].Hash = origMsg.Key
		sig, has := origMsg.Value["signature"]
		if !has {
			checkPanic(errors.Errorf("test(%d) - expected signature in value field", i))
		}
		testMessages[seq].Signature = sig.(string)

		// copy input
		rc, err := input.Open()
		checkPanic(errors.Wrapf(err, "test(%d) - could not open wanted data", i))
		testMessages[seq].Input, err = ioutil.ReadAll(rc)
		checkPanic(errors.Wrapf(err, "test(%d) - could not read all data", i))
		checkPanic(errors.Wrapf(rc.Close(), "test(%d) - could not close input reader", i))

		// copy wanted output
		rc, err = want.Open()
		checkPanic(errors.Wrapf(err, "test(%d) - could not open wanted data", i))
		testMessages[seq].Want, err = ioutil.ReadAll(rc)
		checkPanic(errors.Wrapf(err, "test(%d) - could not read all wanted data", i))

		// cleanup
		checkPanic(errors.Wrapf(origRC.Close(), "test(%d) - could not close reader #2", i))
		seq++
	}
}

func checkPanic(err error) {
	if err != nil {
		panic(err)
	}
}

func TestPreserveOrder(t *testing.T) {
	for i := 1; i < 20; i++ {
		tPresve(t, i)
	}
}

func tPresve(t *testing.T, i int) ([]byte, string) {
	encoded, sig, err := EncodePreserveOrder(testMessages[i].Input)
	if err != nil {
		t.Errorf("EncodePreserveOrder(%d) failed:\n%+v", i, err)
	}
	return encoded, sig
}

func TestComparePreserve(t *testing.T) {
	n := len(testMessages)
	if testing.Short() {
		n = 50
	}
	for i := 1; i < n; i++ {
		w := string(testMessages[i].Want)
		pBytes, sig := tPresve(t, i)
		p := string(pBytes)
		testdiff.StringIs(t, testMessages[i].Signature, sig)
		testdiff.StringIs(t, w, p)
		if d := diff.Diff(w, p); len(d) != 0 && t.Failed() {
			t.Logf("Seq:%d\n%s", i, d)
		}
	}
}
