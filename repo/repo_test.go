package repo

import (
	"io/ioutil"
	"math/rand"
	"os"
	"reflect"
	"testing"
	"testing/quick"
	"time"

	"github.com/cryptix/go/logging/logtest"
	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"

	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/message"
)

type testMessage message.StoredMessage

var (
	testMsgCnt = 0

	testAlice  = &sbot.FeedRef{Algo: "faketest", ID: []byte("alice")}
	testBob    = &sbot.FeedRef{Algo: "faketest", ID: []byte("bob")}
	testClaire = &sbot.FeedRef{Algo: "faketest", ID: []byte("claire")}
)

// TODO: generate proper test feeds with matching previous etc
func (tm testMessage) Generate(rand *rand.Rand, size int) reflect.Value {
	pVal, _ := quick.Value(reflect.TypeOf(tm.Previous), rand)
	keyVal, _ := quick.Value(reflect.TypeOf(tm.Key), rand)
	seqVal, _ := quick.Value(reflect.TypeOf(tm.Sequence), rand)
	timeVal, _ := quick.Value(reflect.TypeOf(int64(0)), rand)

	switch testMsgCnt % 3 {
	case 0:
		tm.Author = testAlice
	case 1:
		tm.Author = testBob
	case 2:
		tm.Author = testClaire
	}
	testMsgCnt++

	tm.Previous = pVal.Interface().(*sbot.MessageRef)
	tm.Key = keyVal.Interface().(*sbot.MessageRef)
	tm.Sequence = seqVal.Interface().(margaret.BaseSeq)
	tm.Timestamp = time.Unix(timeVal.Int()%9999, 0)
	tm.Raw = []byte(`{"content":{"type":"testdata"}}`)
	return reflect.ValueOf(&tm)
}

func TestNew(t *testing.T) {
	r := require.New(t)
	l, _ := logtest.KitLogger(t.Name(), t)

	rpath, err := ioutil.TempDir("", t.Name())
	r.NoError(err)

	repo, err := New(log.With(l, "module", "repo"), rpath)
	r.NoError(err, "failed to create repo")

	rl := repo.RootLog()
	seq, err := rl.Seq().Value()
	r.NoError(err, "failed to get log seq")
	r.Equal(margaret.BaseSeq(-1), seq)

	r.NoError(repo.Close(), "failed to close repo")

	if !t.Failed() {
		os.RemoveAll(rpath)
	}
}

func TestMakeSomeMessages(t *testing.T) {
	r := require.New(t)
	l, _ := logtest.KitLogger(t.Name(), t)

	rpath, err := ioutil.TempDir("", t.Name())
	r.NoError(err)

	repo, err := New(log.With(l, "module", "repo"), rpath)
	r.NoError(err, "failed to create repo")

	rl := repo.RootLog()
	seq, err := rl.Seq().Value()
	r.NoError(err, "failed to get log seq")
	r.Equal(margaret.BaseSeq(-1), seq, "not empty")

	rand := rand.New(rand.NewSource(42))
	refT := reflect.TypeOf(testMessage{})

	const n = 90
	r.True(n%3 == 0)
	for i := 0; i < n; i++ {
		v, ok := quick.Value(refT, rand)

		if !ok {
			t.Error("false quick val")
			t.Log(v)
			return
		}
		tmsg := v.Interface().(*testMessage)

		_, err := rl.Append(message.StoredMessage(*tmsg))
		r.NoError(err, "failed to append testmsg %d", i)
	}

	// TODO: repo needs an _up2date_ function like jsbot.status
	// closing will just halt the indexing process
	time.Sleep(1 * time.Second)

	users := [][]byte{
		testAlice.ID,
		testBob.ID,
		testClaire.ID,
	}
	for _, testUser := range users {
		subLog, err := repo.UserFeeds().Get(librarian.Addr(testUser))
		r.NoError(err, "failed to get sublog for alice")
		currSeq, err := subLog.Seq().Value()
		r.NotEqual(margaret.SeqEmpty, currSeq)
		r.Equal(margaret.BaseSeq(n/3)-1, currSeq)
	}

	r.NoError(repo.Close(), "failed to close repo")

	if t.Failed() {
		t.Log("test repo at ", rpath)
	} else {
		os.RemoveAll(rpath)
	}
}
