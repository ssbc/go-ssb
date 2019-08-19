package gossip

// TODO: Fetch streams concurrently.

import (
	"context"
	"math"
	"sync"

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

// multiSink takes each message poured into it, and passes it on to all
// registered sinks.
//
// multiSink is like luigi.Broadcaster but with context support.
type multiSink struct {
	seq   int64
	sinks []luigi.Sink
	ctxs  map[luigi.Sink]context.Context
	until map[luigi.Sink]int64

	isClosed bool
}

var _ luigi.Sink = (*multiSink)(nil)
var _ margaret.Seq = (*multiSink)(nil)

func newMultiSink(seq int64) *multiSink {
	return &multiSink{
		seq:   seq,
		ctxs:  make(map[luigi.Sink]context.Context),
		until: make(map[luigi.Sink]int64),
	}
}

func (f *multiSink) Seq() int64 {
	return f.seq
}

// Register adds a sink to propagate messages to upto the 'until'th sequence.
func (f *multiSink) Register(
	ctx context.Context,
	sink luigi.Sink,
	until int64,
) error {
	f.sinks = append(f.sinks, sink)
	f.ctxs[sink] = ctx
	f.until[sink] = until
	return nil
}

func (f *multiSink) Unregister(
	sink luigi.Sink,
) error {
	for i, s := range f.sinks {
		if sink != s {
			continue
		}
		f.sinks = append(f.sinks[:i], f.sinks[(i+1):]...)
		delete(f.ctxs, sink)
		delete(f.until, sink)
		return nil
	}
	return nil
}

func (f *multiSink) Close() error {
	f.isClosed = true
	return nil
}

func (f *multiSink) Pour(
	ctx context.Context,
	msg interface{},
) error {
	if f.isClosed {
		return luigi.EOS{}
	}
	f.seq++

	var deadFeeds []luigi.Sink

	for _, s := range f.sinks {
		err := s.Pour(f.ctxs[s], msg)
		if luigi.IsEOS(err) || f.until[s] <= f.seq {
			deadFeeds = append(deadFeeds, s)
			continue
		} else if err != nil {
			// QUESTION: should CloseWithError be used here?
			panic(err)
		}
	}

	for _, feed := range deadFeeds {
		f.Unregister(feed)
	}

	return nil
}

// FeedManager handles serving gossip about User Feeds.
type FeedManager struct {
	RootLog   margaret.Log
	UserFeeds multilog.MultiLog
	logger    logging.Interface

	liveFeeds    map[string]*multiSink
	liveFeedsMut sync.Mutex

	// metrics
	sysGauge *prometheus.Gauge
	sysCtr   *prometheus.Counter
}

// NewFeedManager returns a new FeedManager used for gossiping about User
// Feeds.
func NewFeedManager(
	rootLog margaret.Log,
	userFeeds multilog.MultiLog,
	info logging.Interface,
	sysGauge *prometheus.Gauge,
	sysCtr *prometheus.Counter,
) *FeedManager {
	fm := &FeedManager{
		RootLog:   rootLog,
		UserFeeds: userFeeds,
		logger:    info,

		liveFeeds: make(map[string]*multiSink),
	}
	// QUESTION: How should the error case be handled?
	go fm.serveLiveFeeds()
	return fm
}

func (m *FeedManager) pour(ctx context.Context, val interface{}, _ error) error {
	m.liveFeedsMut.Lock()
	defer m.liveFeedsMut.Unlock()

	author := val.(margaret.SeqWrapper).Value().(message.StoredMessage).GetAuthor()
	sink, ok := m.liveFeeds[author.Ref()]
	if !ok {
		return nil
	}
	return sink.Pour(ctx, val)
}

func (m *FeedManager) serveLiveFeeds() {
	seqv, err := m.RootLog.Seq().Value()
	if err != nil {
		err = errors.Wrap(err, "failed to get root log sequence")
		panic(err)
	}

	src, err := m.RootLog.Query(
		margaret.Gt(seqv.(margaret.BaseSeq)),
		margaret.Live(true),
		margaret.SeqWrap(true),
	)
	if err != nil {
		panic(err)
	}

	ctx := context.TODO()
	err = luigi.Pump(ctx, luigi.FuncSink(m.pour), src)
	if err != nil {
		err = errors.Wrap(err, "error while serving live feed")
		panic(err)
	}
}

func (m *FeedManager) addLiveFeed(
	ctx context.Context,
	sink luigi.Sink,
	ssbID string,
	seq, limit int64,
) error {
	// TODO: ensure all messages make it to the live query
	//  Messages could be lost when written after the non-live portion and
	//  registering to live feed.
	m.liveFeedsMut.Lock()
	defer m.liveFeedsMut.Unlock()

	liveFeed, ok := m.liveFeeds[ssbID]
	if !ok {
		m.liveFeeds[ssbID] = newMultiSink(seq)
		liveFeed = m.liveFeeds[ssbID]
	}

	until := seq + limit
	if limit == -1 {
		until = math.MaxInt64
	}
	err := liveFeed.Register(ctx, sink, until)
	if err != nil {
		return errors.Wrapf(err, "could not create live stream for client %s", ssbID)
	}
	m.liveFeeds[ssbID] = liveFeed
	// TODO: Remove multiSink from map when complete
	return nil
}

// nonliveLimit returns the upper limit for a CreateStreamHistory request given
// the current User Feeds latest sequence.
func nonliveLimit(
	arg *message.CreateHistArgs,
	curSeq int64,
) int64 {
	if arg.Limit == -1 {
		return -1
	}
	lastSeq := arg.Seq + arg.Limit - 1
	if lastSeq > curSeq {
		lastSeq = curSeq
	}
	return lastSeq - arg.Seq + 1
}

// liveLimit returns the limit for serving the 'live' portion for a
// CreateStreamHistory request given the current User Feeds latest sequence.
func liveLimit(
	arg *message.CreateHistArgs,
	curSeq int64,
) int64 {
	if arg.Limit == -1 {
		return -1
	}

	startSeq := curSeq + 1
	lastSeq := arg.Seq + arg.Limit - 1
	if lastSeq < curSeq {
		return 0
	}
	return lastSeq - startSeq + 1
}

// getLatestSeq returns the latest Sequence number for the given log.
func getLatestSeq(log margaret.Log) (int64, error) {
	latestSeqValue, err := log.Seq().Value()
	if err != nil {
		return 0, errors.Wrapf(err, "failed to observe latest")
	}
	switch v := latestSeqValue.(type) {
	case librarian.UnsetValue: // don't have the feed - nothing to do?
		return 0, nil
	case margaret.BaseSeq:
		return v.Seq(), nil
	default:
		return 0, errors.Errorf("wrong type in index. expected margaret.BaseSeq - got %T", v)
	}
}

// newSinkCounter returns a new Sink which iterates the given counter when poured to.
func newSinkCounter(counter *int, sink luigi.Sink) luigi.FuncSink {
	return func(ctx context.Context, v interface{}, err error) error {
		if err != nil {
			return err
		}
		msg, ok := v.([]byte)
		if !ok {
			return errors.Errorf("b4pour: expected []byte - got %T", v)
		}
		*counter++
		return sink.Pour(ctx, message.RawSignedMessage{RawMessage: msg})
	}
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
	latest, err := getLatestSeq(userLog)
	if err != nil {
		return errors.Wrap(err, "userLog sequence")
	}

	if arg.Seq != 0 {
		arg.Seq--             // our idx is 0 ed
		if arg.Seq > latest { // more than we got
			return errors.Wrap(sink.Close(), "pour: failed to close")
		}
	}
	if arg.Live && arg.Limit == 0 {
		arg.Limit = -1
	}

	// Make query
	limit := nonliveLimit(arg, latest)
	resolved := mutil.Indirect(m.RootLog, userLog)
	src, err := resolved.Query(
		margaret.Gte(margaret.BaseSeq(arg.Seq)),
		margaret.Limit(int(limit)),
		margaret.Reverse(arg.Reverse),
	)
	if err != nil {
		return errors.Wrapf(err, "invalid user log query")
	}
	src = transform.NewKeyValueWrapper(src, arg.Keys)
	if err != nil {
		return err
	}

	sent := 0
	snk := newSinkCounter(&sent, sink)
	err = luigi.Pump(ctx, snk, src)
	if m.sysCtr != nil {
		m.sysCtr.With("event", "gossiptx").Add(float64(sent))
	} else {
		m.logger.Log("event", "gossiptx", "n", sent)
	}
	if errors.Cause(err) == context.Canceled {
		sink.Close()
		return nil
	} else if err != nil {
		return errors.Wrap(err, "failed to pump messages to peer")
	}

	if arg.Live {
		return m.addLiveFeed(
			ctx, sink,
			arg.Id,
			latest, liveLimit(arg, latest),
		)
	}
	return nil
}
