package gossip

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/cryptix/go/logging"
	"github.com/cryptix/go/logging/logtest"
	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/ctxutils"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/multilogs"
	pluginTest "go.cryptoscope.co/ssb/plugins/test"
	"go.cryptoscope.co/ssb/repo"
)

func requireFeedRef(
	t *testing.T,
	arg string,
) *ssb.FeedRef {
	ret, err := ssb.ParseFeedRef(arg)
	require.NoError(t, err)
	return ret
}

func requireRecvMessage(
	t *testing.T,
	msg message.StoredMessage,
	log margaret.Log,
	rcv *luigi.SliceSink,
) {
	before := len(*rcv)
	_, err := log.Append(msg)
	require.NoError(t, err)
	time.Sleep(200 * time.Millisecond)
	require.Equal(t, 1, len(*rcv)-before)
}

func loadTestRepo(
	t *testing.T,
	repoPath string,
) (
	margaret.Log,
	multilog.MultiLog,
	*ssb.KeyPair,
) {
	r := pluginTest.LoadTestDataPeer(t, repoPath)
	keyPair, err := repo.OpenKeyPair(r)
	require.NoError(t, err, "error opening src key pair")

	rootLog, err := repo.OpenLog(r)
	require.NoError(t, err, "error opening source repository")

	userFeeds, _, _, err := multilogs.OpenUserFeeds(r)
	require.NoError(t, err, "error getting dst userfeeds multilog")

	return rootLog, userFeeds, keyPair
}

func testLogger(t *testing.T) logging.Interface {
	var logger logging.Interface
	if testing.Verbose() {
		l := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
		logger = log.With(l, "bot", "alice")
	} else {
		logger, _ = logtest.KitLogger("alice", t)
	}
	return logger
}

func sendMessages(t *testing.T, log margaret.Log, num int, author *ssb.FeedRef) {
	for i := 0; i < num; i++ {
		msg := message.StoredMessage{
			Author: author,
		}
		_, err := log.Append(msg)
		require.NoError(t, err)
	}
}

func TestCreateHistoryStream(t *testing.T) {
	repoPath := "testdata/largeRepo"
	infoAlice := testLogger(t)
	userFeedLen := 432

	tests := []struct {
		Name          string
		Args          message.CreateHistArgs
		LiveMessages  int
		TotalReceived int
	}{
		{
			Name: "Fetching of entire feed",
			Args: message.CreateHistArgs{
				Id:    "@FCX/tsDLpubCPKKfIrw4gc+SQkHcaD17s7GI6i/ziWY=.ed25519",
				Type:  "source",
				Seq:   0,
				Limit: -1,
			},
			TotalReceived: userFeedLen,
		},
		{
			// The sequence number here is not intuitive
			Name: "Stream with sequence set",
			Args: message.CreateHistArgs{
				Id:    "@FCX/tsDLpubCPKKfIrw4gc+SQkHcaD17s7GI6i/ziWY=.ed25519",
				Type:  "source",
				Seq:   6,
				Limit: -1,
			},
			TotalReceived: userFeedLen - 5,
		},
		{
			// TODO: investigate what the expected sequence value is for live feeds
			Name: "Fetching of live stream",
			Args: message.CreateHistArgs{
				Id:   "@FCX/tsDLpubCPKKfIrw4gc+SQkHcaD17s7GI6i/ziWY=.ed25519",
				Type: "source",
				Seq:  int64(userFeedLen),
				Live: true,
			},
			LiveMessages:  4,
			TotalReceived: 5,
		},
		{
			Name: "Live stream should respect limit",
			Args: message.CreateHistArgs{
				Id:    "@FCX/tsDLpubCPKKfIrw4gc+SQkHcaD17s7GI6i/ziWY=.ed25519",
				Type:  "source",
				Seq:   int64(userFeedLen),
				Limit: 5,
				Live:  true,
			},
			LiveMessages:  10,
			TotalReceived: 5,
		},
		{
			Name: "Live stream should respect limit with old messages",
			Args: message.CreateHistArgs{
				Id:    "@FCX/tsDLpubCPKKfIrw4gc+SQkHcaD17s7GI6i/ziWY=.ed25519",
				Type:  "source",
				Seq:   int64(userFeedLen) - 5,
				Limit: 10,
				Live:  true,
			},
			LiveMessages:  15,
			TotalReceived: 10,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			t.Log("Test case:", test.Name)

			ctx, cancel := ctxutils.WithError(context.Background(), ssb.ErrShuttingDown)
			defer cancel()

			rootLog, userFeeds, keyPair := loadTestRepo(t, repoPath)
			defer userFeeds.Close()

			test.Args.Id = keyPair.Id.Ref()
			sink := luigi.SliceSink([]interface{}{})

			fm := NewFeedManager(rootLog, userFeeds, infoAlice, nil, nil)

			err := fm.CreateStreamHistory(ctx, &sink, &test.Args)
			require.NoError(t, err)
			sendMessages(t, rootLog, test.LiveMessages, keyPair.Id)
			time.Sleep(200 * time.Millisecond)

			require.Len(t, []interface{}(sink), test.TotalReceived,
				"expect %d, but got %d", test.TotalReceived, len([]interface{}(sink)))
		})
	}
}
