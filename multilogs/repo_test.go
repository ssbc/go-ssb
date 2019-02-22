package multilogs

import (
	"context"
	"io/ioutil"
	"math/rand"
	"os"
	"reflect"
	"testing"
	"testing/quick"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/ctxutils"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/repo"
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
func TestMakeSomeMessages(t *testing.T) {

	r := require.New(t)

	rpath, err := ioutil.TempDir("", t.Name())
	r.NoError(err)

	testRepo := repo.New(rpath)

	_, err = repo.OpenKeyPair(testRepo)
	r.NoError(err, "failed to open key pair")

	rl, err := repo.OpenLog(testRepo)
	r.NoError(err, "failed to open root log")
	seq, err := rl.Seq().Value()
	r.NoError(err, "failed to get log seq")
	r.Equal(margaret.BaseSeq(-1), seq, "not empty")

	userFeeds, _, userFeedsServe, err := OpenUserFeeds(testRepo)
	r.NoError(err, "failed to get user feeds multilog")

	ctx, cancel := ctxutils.WithError(context.Background(), ssb.ErrShuttingDown)
	errc := make(chan error)
	go func() {
		err := userFeedsServe(ctx, rl)
		errc <- errors.Wrap(err, "failed to pump log into userfeeds multilog")
	}()
	staticRand := rand.New(rand.NewSource(42))
	refT := reflect.TypeOf(testMessage{})

	const n = 90
	r.True(n%3 == 0)
	for i := 0; i < n; i++ {
		v, ok := quick.Value(refT, staticRand)

		if !ok {
			t.Log(v)
			panic("false quick val")
		}
		tmsg := v.Interface().(*testMessage)

		_, err := rl.Append(message.StoredMessage(*tmsg))
		r.NoError(err, "failed to append testmsg %d", i)
	}

	// TODO: repo needs an _up2date_ function like jsbot.status
	// closing will just halt the indexing process
	time.Sleep(1 * time.Second)
	cancel()
	r.NoError(<-errc)

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
