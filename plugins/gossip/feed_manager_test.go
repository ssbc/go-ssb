// SPDX-License-Identifier: MIT

package gossip

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"go.cryptoscope.co/muxrpc/v2/codec"

	"github.com/stretchr/testify/require"
	"go.mindeco.de/log"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/asynctesting"
	"go.cryptoscope.co/ssb/internal/ctxutils"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/multilogs"
	"go.cryptoscope.co/ssb/repo"
)

func requireFeedRef(
	t *testing.T,
	arg string,
) refs.FeedRef {
	ret, err := refs.ParseFeedRef(arg)
	require.NoError(t, err)
	return ret
}

func loadTestRepo(
	t *testing.T,
	repoPath string,
) (
	func(t *testing.T, num int, text string),
	margaret.Log,
	multilog.MultiLog,
	*ssb.KeyPair,
) {

	os.RemoveAll(repoPath)
	r := repo.New(repoPath)

	keyPair, err := repo.DefaultKeyPair(r)
	require.NoError(t, err, "error opening src key pair")

	rootLog, err := repo.OpenLog(r)
	require.NoError(t, err, "error opening source repository")

	userFeeds, refresh, err := multilogs.OpenUserFeeds(r)
	require.NoError(t, err, "error getting dst userfeeds multilog")

	pub, err := message.OpenPublishLog(rootLog, userFeeds, keyPair)
	require.NoError(t, err, "error getting dst userfeeds multilog")

	return createMessages(pub, refresh, rootLog), rootLog, userFeeds, keyPair
}

func createMessages(pub ssb.Publisher, fill librarian.SinkIndex, rootLog margaret.Log) func(t *testing.T, num int, text string) {

	return func(t *testing.T, num int, text string) {
		t.Log("creating", num, text)
		for i := 0; i < num; i++ {
			msg, err := pub.Publish(fmt.Sprintf("hello world #%d - %s", i, text))
			require.NoError(t, err)
			errc := asynctesting.ServeLog(context.TODO(), "helper", rootLog, fill, false)
			require.NoError(t, <-errc, "refresh failed")
			t.Log("msg:", i, msg.Ref())
		}
	}
}

func TestCreateHistoryStream(t *testing.T) {

	userFeedLen := 23

	tests := []struct {
		Name          string
		Args          message.CreateHistArgs
		LiveMessages  int
		TotalReceived int
	}{
		{
			Name: "Fetching of entire feed",
			Args: message.CreateHistArgs{
				Seq:        0,
				StreamArgs: message.StreamArgs{Limit: -1},
			},
			TotalReceived: userFeedLen,
		},
		{
			// The sequence number here is not intuitive
			Name: "Stream with sequence set",
			Args: message.CreateHistArgs{
				Seq:        6,
				StreamArgs: message.StreamArgs{Limit: -1},
			},
			TotalReceived: userFeedLen - 5,
		},
		// {
		// 	// TODO: investigate what the expected sequence value is for live feeds
		// 	Name: "Fetching of live stream",
		// 	Args: message.CreateHistArgs{
		// 		Seq:        int64(userFeedLen),
		// 		CommonArgs: message.CommonArgs{Live: true},
		// 	},
		// 	LiveMessages:  4,
		// 	TotalReceived: 5,
		// },
		// {
		// 	Name: "Live stream should respect limit",
		// 	Args: message.CreateHistArgs{
		// 		Seq:        int64(userFeedLen),
		// 		StreamArgs: message.StreamArgs{Limit: 5},
		// 		CommonArgs: message.CommonArgs{Live: true},
		// 	},
		// 	LiveMessages:  10,
		// 	TotalReceived: 5,
		// },
		// {
		// 	Name: "Live stream should respect limit with old messages",
		// 	Args: message.CreateHistArgs{
		// 		Seq:        int64(userFeedLen) - 5,
		// 		StreamArgs: message.StreamArgs{Limit: 10},
		// 		CommonArgs: message.CommonArgs{Live: true},
		// 	},
		// 	LiveMessages:  15,
		// 	TotalReceived: 10,
		// },
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			t.Log("Test case:", test.Name)
			r := require.New(t)
			l := testutils.NewRelativeTimeLogger(nil)
			infoAlice := log.With(l, "bot", "alice")

			ctx, cancel := ctxutils.WithError(context.Background(), ssb.ErrShuttingDown)
			defer cancel()

			repoPath := filepath.Join("testrun", t.Name())
			create, rootLog, userFeeds, keyPair := loadTestRepo(t, repoPath)
			defer userFeeds.Close()

			create(t, userFeedLen, "prefill")
			t.Log("created prefil")
			log, err := userFeeds.Get(storedrefs.Feed(keyPair.Id))
			r.NoError(err)
			seqv, err := log.Seq().Value()
			r.NoError(err)
			r.EqualValues(userFeedLen-1, seqv)

			test.Args.ID = keyPair.Id
			var buf = new(bytes.Buffer)
			var sink = muxrpc.NewTestSink(buf)

			fm := NewFeedManager(context.TODO(), rootLog, userFeeds, infoAlice, nil, nil)

			err = fm.CreateStreamHistory(ctx, sink, test.Args)
			r.NoError(err)
			t.Log("serving")
			create(t, test.LiveMessages, "post/live")

			cnt := len(readAllPackets(buf))
			// -1 for the EndErr packet (which isnt a message)
			require.Equal(t, cnt-1, test.TotalReceived)
		})
	}
}

func readAllPackets(r io.Reader) []*codec.Packet {
	var pkts []*codec.Packet
	cr := codec.NewReader(r)
	for {
		pkt, err := cr.ReadPacket()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			panic(err)
		}
		pkts = append(pkts, pkt)
	}
	return pkts
}
