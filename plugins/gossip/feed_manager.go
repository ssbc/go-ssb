package gossip

// TODO: Fetch streams concurrently.

import (
	"context"
	"sync"
	"time"

	"github.com/cryptix/go/logging"
	"github.com/go-kit/kit/metrics/prometheus"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/transform"
	"go.cryptoscope.co/ssb/message"
)

func makeQuery(
	rootLog, userLog margaret.Log,
	withKeys bool,
	querySpecs ...margaret.QuerySpec,
) (luigi.Source, error) {
	resolved := mutil.Indirect(rootLog, userLog)
	src, err := resolved.Query(querySpecs...)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid user log query")
	}
	src = transform.NewKeyValueWrapper(src, withKeys)
	return src, nil
}

type liveStream struct {
	Request message.CreateHistArgs
	Ctxs    []context.Context
	Feeds   []luigi.Sink

	Source  luigi.Source
	rootLog margaret.Log
	userLog margaret.Log

	mut sync.Mutex
	seq int64
}

// each live stream must be uniq per CreateHistArgs
func newLiveStream(rootLog margaret.Log, userLog margaret.Log) *liveStream {
	ret := &liveStream{
		rootLog: rootLog,
		userLog: userLog,
	}

	return ret
}

func (s *liveStream) Add(ctx context.Context, sink luigi.Sink, arg *message.CreateHistArgs) error {
	if !arg.Live {
		// TODO: what happens if the Live parameter is unset, but the Limit is > then the length?
		return errors.New("request must be for a live steam")
	}
	if arg.Reverse {
		return errors.New("live stream is incompatible with reverse")
	}

	s.mut.Lock()
	defer s.mut.Unlock()

	if s.Source == nil {
		// This is the first live query to serve
		liveQuery, err := makeQuery(
			s.rootLog, s.userLog, arg.Keys,
			margaret.Gte(margaret.BaseSeq(arg.Seq)),
			margaret.Limit(int(arg.Limit)),
			margaret.Live(arg.Live),

			// The sequence number is required for merging other feeds.
			margaret.SeqWrap(true),
		)
		if err != nil {
			return err
		}
		s.Source = liveQuery
		s.seq = arg.Seq + arg.Limit
	}

	// make up for difference in feed
	limit := s.seq - arg.Seq
	if limit > arg.Limit {
		limit = arg.Limit
	}

	nonLiveQuery, err := makeQuery(
		s.rootLog, s.userLog, arg.Keys,
		margaret.Gte(margaret.BaseSeq(arg.Seq)),
		margaret.Limit(int(limit)),
		margaret.Live(false),
	)
	if err != nil {
		return err
	}
	if err := luigi.Pump(ctx, sink, nonLiveQuery); err != nil {
		return err
	}

	s.Ctxs = append(s.Ctxs, ctx)
	s.Feeds = append(s.Feeds, sink)
	return nil
}

func (s *liveStream) Seq() int64 {
	s.mut.Lock()
	defer s.mut.Unlock()

	return s.seq
}

func (s *liveStream) removeFeed(arg luigi.Sink) {
	var ctxs []context.Context
	var feeds []luigi.Sink
	for i, feed := range s.Feeds {
		if arg != feed {
			ctxs = append(ctxs, s.Ctxs[i])
			feeds = append(feeds, feed)
		}
	}
	s.Ctxs = ctxs
	s.Feeds = feeds
}

// propagate sends a value to all registered sinks.
func (s *liveStream) propagate(val margaret.SeqWrapper) {
	s.mut.Lock()
	defer s.mut.Unlock()

	var deadFeeds []luigi.Sink

	s.seq = val.Seq().Seq()
	value := val.Value()
	for i, sink := range s.Feeds {
		ctx := s.Ctxs[i]
		err := sink.Pour(ctx, value)
		if luigi.IsEOS(err) {
			deadFeeds = append(deadFeeds, sink)
			continue
		} else if err != nil {
			// TODO: use CloseWithError instead or log?, skipping to get tested and functional
			panic(err)
		}
	}

	for _, feed := range deadFeeds {
		s.removeFeed(feed)
	}
}

func earliestCtx(ctxs []context.Context) context.Context {
	if len(ctxs) == 0 {
		panic("should not be reachable")
	}
	earliest := ctxs[0]
	for _, ctx := range ctxs[1:] {
		deadline, ok := ctx.Deadline()
		if ok && deadline.Before(time.Now()) {
			earliest = ctx
		}
	}
	return earliest
}

func (s *liveStream) clearDeadSinks() {
	for i, ctx := range s.Ctxs {
		if ctx.Err() != nil {
			continue
		}
		s.removeFeed(s.Feeds[i])
	}
}

func (s *liveStream) Pump() error {
	for {
		if len(s.Feeds) <= 0 {
			return nil
		}

		ctx := earliestCtx(s.Ctxs)
		val, err := s.Source.Next(ctx)
		if luigi.IsEOS(err) {
			s.clearDeadSinks()
			continue
		} else if err != nil {
			return errors.Wrap(err, "sourcing feed in live stream")
		}
		s.propagate(val.(margaret.SeqWrapper))
	}
}

// FeedManager ...
type FeedManager struct {
	RootLog   margaret.Log
	UserFeeds multilog.MultiLog
	Info      logging.Interface

	LiveFeed    map[message.CreateHistArgs]*liveStream
	liveFeedMut sync.Mutex

	// metrics
	sysGauge *prometheus.Gauge
	sysCtr   *prometheus.Counter
}

// NewFeedManager ...
func NewFeedManager(
	rootLog margaret.Log,
	userFeeds multilog.MultiLog,
	info logging.Interface,
	sysGauge *prometheus.Gauge,
	sysCtr *prometheus.Counter,
) *FeedManager {
	return &FeedManager{
		RootLog:   rootLog,
		UserFeeds: userFeeds,
		Info:      info,

		LiveFeed: make(map[message.CreateHistArgs]*liveStream),
	}
}

func keyFor(arg *message.CreateHistArgs) message.CreateHistArgs {
	// When aggregating live feeds into one, the start (seq) should be
	// discarded.
	key := *arg
	key.Seq = 0
	return key
}

func (m *FeedManager) serveLiveFeed(arg *liveStream) {
	defer m.removeLiveFeed(&arg.Request)

	err := arg.Pump()
	if err != nil {
		m.Info.Log("event", "gossiptx", err.Error())
	}
	return
}

func (m *FeedManager) addLiveFeed(
	ctx context.Context,
	sink luigi.Sink,
	userLog margaret.Log,
	liveCreateHistArgs *message.CreateHistArgs,
) error {
	m.liveFeedMut.Lock()
	defer m.liveFeedMut.Unlock()

	key := keyFor(liveCreateHistArgs)
	liveStream, ok := m.LiveFeed[key]

	if ok {
		err := liveStream.Add(ctx, sink, liveCreateHistArgs)
		return errors.Wrapf(err, "could not add live stream for client %s", liveCreateHistArgs.Id)
	}

	liveStream = newLiveStream(m.RootLog, userLog)
	err := liveStream.Add(ctx, sink, liveCreateHistArgs)
	if err != nil {
		return errors.Wrapf(err, "could not create live stream for client %s", liveCreateHistArgs.Id)
	}
	m.LiveFeed[key] = liveStream
	go m.serveLiveFeed(liveStream)
	return nil
}

func (m *FeedManager) removeLiveFeed(
	liveCreateHistArgs *message.CreateHistArgs,
) {
	m.liveFeedMut.Lock()
	defer m.liveFeedMut.Unlock()

	key := keyFor(liveCreateHistArgs)
	delete(m.LiveFeed, key)
}

func (m *FeedManager) liveStreamSeq(
	arg *message.CreateHistArgs,
) (int64, bool) {
	key := keyFor(arg)
	liveStream, ok := m.LiveFeed[key]
	if !ok {
		return 0, false
	}
	return liveStream.Seq(), true
}

// until returns the CreateHistArgs upperbound by the given sequence.
func until(seq int64, arg *message.CreateHistArgs) *message.CreateHistArgs {
	var ret message.CreateHistArgs
	ret = *arg
	ret.Limit = seq - arg.Seq
	ret.Live = false
	return &ret
}

// from returns the CreateHistArgs starting from the given sequence.
func from(seq int64, arg *message.CreateHistArgs) *message.CreateHistArgs {
	var ret message.CreateHistArgs
	ret = *arg
	ret.Seq = seq
	ret.Limit = arg.Seq + arg.Limit - seq
	if ret.Limit < 0 {
		ret.Limit = 0
	}
	ret.Live = arg.Live
	return &ret
}

// CreateStreamHistory serves the sink a CreateStreamHistory request to the sink.
func (m *FeedManager) CreateStreamHistory(
	ctx context.Context,
	sink luigi.Sink,
	arg *message.CreateHistArgs,
) error {
	feedRef, err := ssb.ParseFeedRef(arg.Id)
	if err != nil {
		return nil // only handle valid feed refs
	}

	// check what we got
	userLog, err := m.UserFeeds.Get(librarian.Addr(feedRef.ID))
	if err != nil {
		return errors.Wrapf(err, "failed to open sublog for user")
	}
	latest, err := userLog.Seq().Value()
	if err != nil {
		return errors.Wrapf(err, "failed to observe latest")
	}
	// act accordingly
	switch v := latest.(type) {
	case librarian.UnsetValue: // don't have the feed - nothing to do?
	case margaret.BaseSeq:
		if arg.Seq != 0 {
			arg.Seq--               // our idx is 0 based
			if arg.Seq > int64(v) { // more than we got
				return errors.Wrap(sink.Close(), "pour: failed to close")
			}
		}

		// TODO: Validate whether this is still needed
		if arg.Live && arg.Limit == 0 {
			// currently having live streams is not tested
			// it might work but we have some problems with dangling rpc routines which we like to fix first
			arg.Limit = -1
		}

		nonLiveQuery := arg
		var liveQuery *message.CreateHistArgs
		if arg.Live {
			seq, ok := m.liveStreamSeq(arg)
			if !ok {
				seq = arg.Seq
			}
			nonLiveQuery = until(seq, arg)
			liveQuery = from(seq, arg)
		}

		src, err := makeQuery(m.RootLog, userLog, nonLiveQuery.Keys,
			margaret.Gte(margaret.BaseSeq(nonLiveQuery.Seq)),
			margaret.Limit(int(nonLiveQuery.Limit)),
			margaret.Reverse(nonLiveQuery.Reverse),
		)
		if err != nil {
			return err
		}

		// Update the base sequence of the query

		sent := 0
		snk := luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {

			if err != nil {
				return err
			}
			msg, ok := v.([]byte)
			if !ok {
				return errors.Errorf("b4pour: expected []byte - got %T", v)
			}
			sent++
			return sink.Pour(ctx, message.RawSignedMessage{RawMessage: msg})
		})

		err = luigi.Pump(ctx, snk, src)
		if m.sysCtr != nil {
			m.sysCtr.With("event", "gossiptx").Add(float64(sent))
		} else {
			m.Info.Log("event", "gossiptx", "n", sent)
		}
		if errors.Cause(err) == context.Canceled {
			sink.Close()
			return nil
		} else if err != nil {
			return errors.Wrap(err, "failed to pump messages to peer")
		}

		if liveQuery != nil {
			return m.addLiveFeed(ctx, sink, userLog, liveQuery)
		}
		return nil
	default:
		return errors.Errorf("wrong type in index. expected margaret.BaseSeq - got %T", latest)
	}
	return errors.Wrap(sink.Close(), "pour: failed to close")
}
