// +build integration

package gossip

import (
	"context"
	"os"
	"testing"

	"github.com/cryptix/go/logging"
	"github.com/cryptix/go/logging/logtest"
	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/ctxutils"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/multilogs"
	pluginTest "go.cryptoscope.co/ssb/plugins/test"
	"go.cryptoscope.co/ssb/repo"
)

func TestCreateHistoryStream(t *testing.T) {
	repoPath := "testdata/largeRepo"
	unknownNumber := 431
	remoteKey := "@qhSpPqhWyJBZ0/w+ERa6WZvRWjaXu0dlep6L+Xi6PQ0=.ed25519"

	_ = unknownNumber
	_ = remoteKey

	// Logging
	// TODO: Rename
	var infoAlice logging.Interface
	if testing.Verbose() {
		l := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
		infoAlice = log.With(l, "bot", "alice")
	} else {
		infoAlice, _ = logtest.KitLogger("alice", t)
	}

	tests := []struct {
		Name       string
		Args       message.CreateHistArgs
		ExpectErr  bool
		ExpectLen  int
		ExpectLive bool
	}{
		//Keys    bool   `json:"keys"`
		//Values  bool   `json:"values"`
		//Live    bool   `json:"live"`
		//Id      string `json:"id"`
		//Seq     int64  `json:"seq"`
		//Limit   int64  `json:"limit"`
		//Reverse bool   `json:"reverse"`
		//Type    string `json:"type"`
		{
			Name: "Fetching of one stream",
			Args: message.CreateHistArgs{
				Id:    "@FCX/tsDLpubCPKKfIrw4gc+SQkHcaD17s7GI6i/ziWY=.ed25519",
				Type:  "source", // not sure if this is necessary
				Seq:   0,        // From beginning
				Limit: 10,
			},
			ExpectLen:  10,
			ExpectLive: false,
		},
		//{
		//	Name: "Non-live stream with sequence set",
		//	Args: message.CreateHistArgs{
		//		Id:    "@FCX/tsDLpubCPKKfIrw4gc+SQkHcaD17s7GI6i/ziWY=.ed25519",
		//		Type:  "source", // not sure if this is necessary
		//		Seq:   5,        // From beginning
		//		Limit: 10,
		//	},
		//	ExpectLen:  5,
		//	ExpectLive: false,
		//},
		{
			Name: "Fetching of live stream",
			Args: message.CreateHistArgs{
				Id:    "@FCX/tsDLpubCPKKfIrw4gc+SQkHcaD17s7GI6i/ziWY=.ed25519",
				Type:  "source", // not sure if this is necessary
				Seq:   0,        // From beginning
				Limit: 0,        // Fetch all messages
				Live:  true,
			},
			ExpectLive: true,
		},
		{
			Name: "Live stream should be closed when limit exceeded",
			Args: message.CreateHistArgs{
				Id:    "@FCX/tsDLpubCPKKfIrw4gc+SQkHcaD17s7GI6i/ziWY=.ed25519",
				Type:  "source", // not sure if this is necessary
				Seq:   0,        // From beginning
				Limit: 10,       // Fetch all messages
				Live:  true,
			},
			ExpectLen:  10,
			ExpectLive: false,
		},
		{
			Name: "Fetching of multiple live streams",
		},
		{
			Name: "Fetching of multiple non-live streams",
		},
	}

	// TODO: test if writing to one stream is carried to all live streams
	// TODO: test amount of messages fetched

	for _, test := range tests {
		t.Log("Test case:", test.Name)

		if test.Args.Id == "" {
			continue
		}

		// Context?
		ctx, cancel := ctxutils.WithError(context.Background(), ssb.ErrShuttingDown)
		defer cancel()

		// I do not know about the data of these repositories, therefor how do I know its working?

		// Test repositories
		srcRepo := pluginTest.LoadTestDataPeer(t, repoPath)
		srcKeyPair, err := repo.OpenKeyPair(srcRepo)
		require.NoError(t, err, "error opening src key pair")

		dstRepo, _ := pluginTest.MakeEmptyPeer(t)
		dstKeyPair, err := repo.OpenKeyPair(dstRepo)
		require.NoError(t, err, "error opening dst key pair")
		dstID := dstKeyPair.Id

		_ = dstKeyPair
		_ = dstID

		srcRootLog, err := repo.OpenLog(srcRepo)
		require.NoError(t, err, "error opening source repository")

		// User feed
		srcMlog, _, _, err := multilogs.OpenUserFeeds(srcRepo)
		require.NoError(t, err, "error getting dst userfeeds multilog")

		// Prepare arguments
		test.Args.Id = srcKeyPair.Id.Ref()

		// destination for our data
		sink := luigi.SliceSink([]interface{}{})

		fm := NewFeedManager(srcRootLog, srcMlog, infoAlice, nil, nil)
		err = fm.CreateStreamHistory(ctx, &sink, &test.Args)
		if test.ExpectErr {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)

		if test.ExpectLen > 0 {
			require.Len(t, []interface{}(sink), test.ExpectLen, "incorrect amount of messages received")
		}

		if test.ExpectLive {
			// TODO: Write to log of user, and check if received
		}

		err = srcMlog.Close()
		require.NoError(t, err, "error closing dst userfeeds multilog")
	}
}
