// SPDX-License-Identifier: MIT

package legacy

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"testing"

	// TODO: was on a streak to remoe all the errors.Wrap but here it's used as check(errors.Wrap(err, "the msg"))
	// which uses the cases that errors.Wrap(err) returns nil if err is nil
	// replacing it here would need a lot of if err!=nil { ... "the msg "}
	// also, this is just test code and not performance critical
	"github.com/pkg/errors"

	"github.com/kylelemons/godebug/diff"
	refs "go.mindeco.de/ssb-refs"
)

type testMessage struct {
	Author          refs.FeedRef
	Hash, Signature string
	Input, NoSig    []byte
}

var testMessages []testMessage

func init() {
	r, err := zip.OpenReader("testdata.zip")
	if err != nil {
		fmt.Println("could not find testdata - run 'node encode_test.js' to create it")
		checkPanic(err)
	}
	defer r.Close()

	if len(r.File)%3 != 0 {
		checkPanic(errors.New("expecting three files per message"))
	}

	testMessages = make([]testMessage, len(r.File)/3+1)

	seq := 1
	for i := 0; i < len(r.File); i += 3 {
		full := r.File[i]
		input := r.File[i+1]
		noSig := r.File[i+2]
		// check file structure assumption
		if noSig.Name != fmt.Sprintf("%05d.noSig", seq) {
			checkPanic(fmt.Errorf("unexpected file. wanted '%05d.noSig' got %s", seq, noSig.Name))
		}
		if input.Name != fmt.Sprintf("%05d.input", seq) {
			checkPanic(fmt.Errorf("unexpected file. wanted '%05d.input' got %s", seq, input.Name))
		}
		if full.Name != fmt.Sprintf("%05d.full", seq) {
			checkPanic(fmt.Errorf("unexpected file. wanted '%05d.full' got %s", seq, full.Name))
		}

		// get some data from the full message
		var origMsg struct {
			Key   string
			Value map[string]interface{}
		}
		origRC, err := full.Open()
		checkPanic(errors.Wrapf(err, "test(%d) - failed to open full", i))
		err = json.NewDecoder(origRC).Decode(&origMsg)
		checkPanic(errors.Wrapf(err, "test(%d) - could not json decode full", i))
		testMessages[seq].Hash = origMsg.Key
		// get sig
		sig, has := origMsg.Value["signature"]
		if !has {
			checkPanic(fmt.Errorf("test(%d) - expected signature in value field", i))
		}
		testMessages[seq].Signature = sig.(string)
		// get author
		a, has := origMsg.Value["author"]
		if !has {
			checkPanic(fmt.Errorf("test(%d) - expected author in value field", i))
		}

		testMessages[seq].Author, err = refs.ParseFeedRef(a.(string))
		checkPanic(errors.Wrapf(err, "test(%d) - failed to parse author ref", i))

		// copy input
		rc, err := input.Open()
		checkPanic(errors.Wrapf(err, "test(%d) - could not open wanted data", i))
		testMessages[seq].Input, err = ioutil.ReadAll(rc)
		checkPanic(errors.Wrapf(err, "test(%d) - could not read all data", i))
		checkPanic(errors.Wrapf(rc.Close(), "test(%d) - could not close input reader", i))

		// copy wanted output
		rc, err = noSig.Open()
		checkPanic(errors.Wrapf(err, "test(%d) - could not open wanted data", i))
		testMessages[seq].NoSig, err = ioutil.ReadAll(rc)
		checkPanic(errors.Wrapf(err, "test(%d) - could not read all wanted data", i))

		// cleanup
		checkPanic(errors.Wrapf(origRC.Close(), "test(%d) - could not close reader #2", i))
		seq++
	}
	log.Printf("loaded %d messages from testdata.zip", seq)
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

func tPresve(t *testing.T, i int) []byte {
	encoded, err := EncodePreserveOrder(testMessages[i].Input)
	if err != nil {
		t.Errorf("EncodePreserveOrder(%d) failed:\n%+v", i, err)
	}
	return encoded
}

func TestComparePreserve(t *testing.T) {
	n := len(testMessages)
	if testing.Short() {
		n = min(50, n)
	}
	for i := 1; i < n; i++ {
		w := string(testMessages[i].Input)
		pBytes := tPresve(t, i)
		p := string(pBytes)

		if d := diff.Diff(w, p); len(d) != 0 && t.Failed() {
			t.Logf("Seq:%d\n%s", i, d)
		}
	}
}
