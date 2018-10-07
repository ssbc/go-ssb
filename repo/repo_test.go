package repo

import (
	"context"
	"io/ioutil"
	"math/rand"
	"os"
	"reflect"
	"testing"
	"testing/quick"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"

	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/message"
)

type testMessage message.StoredMessage

var (
	testMsgCnt = 0

	testAlice  = &ssb.FeedRef{Algo: "faketest", ID: []byte("alice")}
	testBob    = &ssb.FeedRef{Algo: "faketest", ID: []byte("bob")}
	testClaire = &ssb.FeedRef{Algo: "faketest", ID: []byte("claire")}
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

	tm.Previous = pVal.Interface().(*ssb.MessageRef)
	tm.Key = keyVal.Interface().(*ssb.MessageRef)
	tm.Sequence = seqVal.Interface().(margaret.BaseSeq)
	tm.Timestamp = time.Unix(timeVal.Int()%9999, 0)
	tm.Raw = []byte(`{"content":{"type":"testdata"}}`)
	return reflect.ValueOf(&tm)
}

func TestNew(t *testing.T) {
	r := require.New(t)

	rpath, err := ioutil.TempDir("", t.Name())
	r.NoError(err)

	repo := New(rpath)

	_, err = OpenKeyPair(repo)
	r.NoError(err, "failed to open key pair")

	rl, err := OpenLog(repo)
	r.NoError(err, "failed to open root log")
	seq, err := rl.Seq().Value()
	r.NoError(err, "failed to get log seq")
	r.Equal(margaret.BaseSeq(-1), seq)

	if !t.Failed() {
		os.RemoveAll(rpath)
	}
}

// getUserFeeds is a copy of multilogs.getUserFeeds. We can't use that here because that would be an import cycle.
func getUserFeeds(r Interface) (multilog.MultiLog, *badger.DB, func(context.Context, margaret.Log) error, error) {
	return OpenMultiLog(r, "userFeeds", func(ctx context.Context, seq margaret.Seq, value interface{}, mlog multilog.MultiLog) error {
		msg, ok := value.(message.StoredMessage)
		if !ok {
			return errors.Errorf("error casting message. got type %T", value)
		}

		authorID := msg.Author.ID
		authorLog, err := mlog.Get(librarian.Addr(authorID))
		if err != nil {
			return errors.Wrap(err, "error opening sublog")
		}

		_, err = authorLog.Append(seq)
		return errors.Wrap(err, "error appending new author message")
	})
}

func TestMakeSomeMessages(t *testing.T) {
	r := require.New(t)

	rpath, err := ioutil.TempDir("", t.Name())
	r.NoError(err)

	repo := New(rpath)

	_, err = OpenKeyPair(repo)
	r.NoError(err, "failed to open key pair")

	rl, err := OpenLog(repo)
	r.NoError(err, "failed to open root log")
	seq, err := rl.Seq().Value()
	r.NoError(err, "failed to get log seq")
	r.Equal(margaret.BaseSeq(-1), seq, "not empty")

	userFeeds, _, userFeedsServe, err := getUserFeeds(repo)
	r.NoError(err, "failed to get user feeds multilog")

	go func() {
		err := userFeedsServe(context.TODO(), rl)
		r.NoError(err, "failed to pump log into userfeeds multilog")

	}()

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
		subLog, err := userFeeds.Get(librarian.Addr(testUser))
		r.NoError(err, "failed to get sublog for alice")
		currSeq, err := subLog.Seq().Value()
		r.NotEqual(margaret.SeqEmpty, currSeq)
		r.Equal(margaret.BaseSeq(n/3)-1, currSeq)
	}

	if t.Failed() {
		t.Log("test repo at ", rpath)
	} else {
		os.RemoveAll(rpath)
	}
}
