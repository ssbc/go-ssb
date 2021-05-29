// SPDX-License-Identifier: MIT

package sbot

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rs/cors"
	"go.cryptoscope.co/margaret"
	librarian "go.cryptoscope.co/margaret/indexes"
	libmkv "go.cryptoscope.co/margaret/indexes/mkv"
	"go.cryptoscope.co/margaret/multilog/roaring"
	multibadger "go.cryptoscope.co/margaret/multilog/roaring/badger"
	"go.cryptoscope.co/muxrpc/v2"
	kitlog "go.mindeco.de/log"
	"go.mindeco.de/log/level"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/blobstore"
	"go.cryptoscope.co/ssb/graph"
	"go.cryptoscope.co/ssb/indexes"
	"go.cryptoscope.co/ssb/internal/ctxutils"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/statematrix"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/multilogs"
	"go.cryptoscope.co/ssb/network"
	"go.cryptoscope.co/ssb/plugins/blobs"
	"go.cryptoscope.co/ssb/plugins/control"
	"go.cryptoscope.co/ssb/plugins/ebt"
	"go.cryptoscope.co/ssb/plugins/friends"
	"go.cryptoscope.co/ssb/plugins/get"
	"go.cryptoscope.co/ssb/plugins/gossip"
	"go.cryptoscope.co/ssb/plugins/groups"
	"go.cryptoscope.co/ssb/plugins/legacyinvites"
	"go.cryptoscope.co/ssb/plugins/partial"
	privplug "go.cryptoscope.co/ssb/plugins/private"
	"go.cryptoscope.co/ssb/plugins/publish"
	"go.cryptoscope.co/ssb/plugins/rawread"
	"go.cryptoscope.co/ssb/plugins/replicate"
	"go.cryptoscope.co/ssb/plugins/status"
	"go.cryptoscope.co/ssb/plugins/tangles"
	"go.cryptoscope.co/ssb/plugins/whoami"
	"go.cryptoscope.co/ssb/plugins2/names"
	"go.cryptoscope.co/ssb/private"
	"go.cryptoscope.co/ssb/private/keys"
	"go.cryptoscope.co/ssb/repo"
	refs "go.mindeco.de/ssb-refs"
)

func (sbot *Sbot) Replicate(r refs.FeedRef) {
	slog, err := sbot.Users.Get(storedrefs.Feed(r))
	if err != nil {
		panic(err)
	}

	l := int64(-1)
	v, err := slog.Seq().Value()
	if err == nil {
		l = v.(margaret.Seq).Seq()
	}

	sbot.ebtState.Fill(sbot.KeyPair.Id, []statematrix.ObservedFeed{
		{Feed: r, Note: ssb.Note{Seq: l, Receive: true, Replicate: true}},
	})

	sbot.Replicator.Replicate(r)
}

func (sbot *Sbot) DontReplicate(r refs.FeedRef) {
	slog, err := sbot.Users.Get(storedrefs.Feed(r))
	if err != nil {
		panic(err)
	}

	l := int64(-1)
	v, err := slog.Seq().Value()
	if err == nil {
		l = v.(margaret.Seq).Seq()
	}

	sbot.ebtState.Fill(sbot.KeyPair.Id, []statematrix.ObservedFeed{
		{Feed: r, Note: ssb.Note{Seq: l, Receive: false, Replicate: true}},
	})

	sbot.Replicator.DontReplicate(r)
}

// ShutdownContext returns a context that returns ssb.ErrShuttingDown when canceld.
// The server internals use this error to cleanly shut down index processing.
func ShutdownContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return ctxutils.WithError(ctx, ssb.ErrShuttingDown)
}

// Close closes the bot by stopping network connections and closing the internal databases
func (s *Sbot) Close() error {
	s.closedMu.Lock()
	defer s.closedMu.Unlock()

	if s.closed {
		return s.closeErr
	}

	closeEvt := kitlog.With(s.info, "event", "sbot closing")
	s.closed = true

	if s.Network != nil {
		if err := s.Network.Close(); err != nil {
			s.closeErr = fmt.Errorf("sbot: failed to close own network node: %w", err)
			return s.closeErr
		}
		s.Network.GetConnTracker().CloseAll()
		level.Debug(closeEvt).Log("msg", "connections closed")
	}

	if err := s.idxDone.Wait(); err != nil {
		s.closeErr = fmt.Errorf("sbot: index group shutdown failed: %w", err)
		return s.closeErr
	}
	level.Debug(closeEvt).Log("msg", "waited for indexes to close")

	if err := s.closers.Close(); err != nil {
		s.closeErr = err
		return s.closeErr
	}

	level.Info(closeEvt).Log("msg", "closers closed")
	return nil
}

// is called by New() in options, sorry
func initSbot(s *Sbot) (*Sbot, error) {
	log := s.info
	var err error
	ctx := s.rootCtx

	r := repo.New(s.repoPath)

	// optionize?!
	s.ReceiveLog, err = repo.OpenLog(r)
	if err != nil {
		return nil, fmt.Errorf("sbot: failed to open rootlog: %w", err)
	}
	s.closers.AddCloser(s.ReceiveLog.(io.Closer))

	if s.BlobStore == nil { // load default, local file blob store
		s.BlobStore, err = repo.OpenBlobStore(r)
		if err != nil {
			return nil, fmt.Errorf("sbot: failed to open blob store: %w", err)
		}
	}

	wantsLog := kitlog.With(log, "module", "WantManager")
	wm := blobstore.NewWantManager(s.BlobStore,
		blobstore.WantWithLogger(wantsLog),
		blobstore.WantWithContext(s.rootCtx),
		blobstore.WantWithMetrics(s.systemGauge, s.eventCounter),
	)
	s.WantManager = wm
	s.closers.AddCloser(wm)

	for _, opt := range s.lateInit {
		err := opt(s)
		if err != nil {
			return nil, fmt.Errorf("sbot: failed to apply late option: %w", err)
		}
	}

	sm, err := statematrix.New(
		r.GetPath("ebt-state-matrix"),
		s.KeyPair.Id,
	)
	if err != nil {
		return nil, err
	}
	s.closers.AddCloser(sm)
	s.ebtState = sm

	// open timestamp and sequence resovlers
	s.SeqResolver, err = repo.NewSequenceResolver(r)
	if err != nil {
		return nil, fmt.Errorf("error opening sequence resolver: %w", err)
	}
	idxTimestamps := indexes.NewTimestampSorter(s.SeqResolver)
	s.closers.AddCloser(idxTimestamps)
	s.serveIndex("timestamps", idxTimestamps)

	mlogStore, err := repo.OpenBadgerDB(r.GetPath(repo.PrefixMultiLog, "shared-badger"))
	if err != nil {
		return nil, err
	}

	// default multilogs
	var mlogs = []struct {
		Name string
		Mlog **roaring.MultiLog
	}{
		{multilogs.IndexNameFeeds, &s.Users},
		{multilogs.IndexNamePrivates, &s.Private},
		{"msgTypes", &s.ByType},
		{"tangles", &s.Tangles},
	}
	for _, index := range mlogs {
		mlog, err := multibadger.NewShared(mlogStore, []byte("mlog-"+index.Name))
		if err != nil {
			return nil, err
		}
		s.closers.AddCloser(mlog)
		s.mlogIndicies[index.Name] = mlog

		*index.Mlog = mlog
	}

	// publish
	var pubopts = []message.PublishOption{
		message.UseNowTimestamps(true),
	}
	if s.signHMACsecret != nil {
		pubopts = append(pubopts, message.SetHMACKey(s.signHMACsecret))
	}
	s.PublishLog, err = message.OpenPublishLog(s.ReceiveLog, s.Users, s.KeyPair, pubopts...)
	if err != nil {
		return nil, fmt.Errorf("sbot: failed to create publish log: %w", err)
	}

	//
	getIdx, updateSink := indexes.OpenGet(mlogStore)
	s.closers.AddCloser(updateSink)
	s.serveIndex("get", updateSink)
	s.simpleIndex["get"] = getIdx

	// groups2
	pth := r.GetPath(repo.PrefixIndex, "groups", "keys", "mkv")
	err = os.MkdirAll(pth, 0700)
	if err != nil {
		return nil, fmt.Errorf("openIndex: error making index directory: %w", err)
	}

	db, err := repo.OpenMKV(pth)
	if err != nil {
		return nil, fmt.Errorf("openIndex: failed to open MKV database: %w", err)
	}

	idx := libmkv.NewIndex(db, keys.Recipients{})
	ks := &keys.Store{
		Index: idx,
	}
	s.closers.AddCloser(idx)

	s.Groups = private.NewManager(s.KeyPair, s.PublishLog, ks, s.ReceiveLog, s, s.Tangles)

	groupsHelperMlog, err := multibadger.NewShared(mlogStore, []byte("group-member-helper"))
	if err != nil {
		return nil, err
	}
	s.closers.AddCloser(groupsHelperMlog)

	combIdx, err := multilogs.NewCombinedIndex(
		s.repoPath,
		s.Groups,
		s.KeyPair.Id,
		s.ReceiveLog,
		s.Users,
		s.Private,
		s.ByType,
		s.Tangles,
		groupsHelperMlog,
		sm,
	)
	if err != nil {
		return nil, fmt.Errorf("sbot: failed to open combined application index: %w", err)
	}
	s.serveIndex("combined", combIdx)
	s.closers.AddCloser(combIdx)

	// groups re-indexing
	members, membersSnk := multilogs.NewMembershipIndex(mlogStore, s.KeyPair.Id, s.Groups, combIdx)
	s.closers.AddCloser(members)
	s.closers.AddCloser(membersSnk)

	addMemberIdxAddr := librarian.Addr("string:group/add-member")

	addMemberSeqs, err := groupsHelperMlog.Get(addMemberIdxAddr)
	if err != nil {
		return nil, fmt.Errorf("sbot: failed to open sublog for add-member messages: %w", err)
	}
	justAddMemberMsgs := mutil.Indirect(s.ReceiveLog, addMemberSeqs)

	s.serveIndexFrom("group-members", membersSnk, justAddMemberMsgs)

	/* TODO: fix deadlock in index update locking
	if _, ok := s.simpleIndex["content-delete-requests"]; !ok {
		var dcrTrigger dropContentTrigger
		dcrTrigger.logger = kitlog.With(log, "module", "dcrTrigger")
		dcrTrigger.root = s.ReceiveLog
		dcrTrigger.feeds = uf
		dcrTrigger.nuller = s
		err = MountSimpleIndex("content-delete-requests", dcrTrigger.MakeSimpleIndex)(s)
		if err != nil {
			return nil, errors.Wrap(err, "sbot: failed to open load default DCR index")
		}
	}
	*/

	// contact/follow graph
	contactLog, err := s.ByType.Get(librarian.Addr("string:contact"))
	if err != nil {
		return nil, fmt.Errorf("sbot: failed to open message contact sublog: %w", err)
	}
	justContacts := mutil.Indirect(s.ReceiveLog, contactLog)

	// LogBuilder doesn't fully work yet
	if false {
		level.Warn(s.info).Log("event", "bot init", "msg", "using experimental bytype:contact graph implementation")

		s.GraphBuilder, err = graph.NewLogBuilder(s.info, justContacts)
		if err != nil {
			return nil, fmt.Errorf("sbot: NewLogBuilder failed: %w", err)
		}
	} else {
		gb, seqSetter, updateIdx := indexes.OpenContacts(kitlog.With(log, "module", "graph"), mlogStore)

		s.serveIndexFrom("contacts", updateIdx, justContacts)
		s.closers.AddCloser(seqSetter)
		s.GraphBuilder = gb
	}

	// abouts
	aboutSeqs, err := s.ByType.Get(librarian.Addr("string:about"))
	if err != nil {
		return nil, fmt.Errorf("sbot: failed to open message about sublog: %w", err)
	}
	aboutsOnly := mutil.Indirect(s.ReceiveLog, aboutSeqs)

	var namesPlug names.Plugin
	_, aboutSnk := namesPlug.OpenSharedIndex(mlogStore)

	s.closers.AddCloser(aboutSnk)
	s.serveIndexFrom("abouts", aboutSnk, aboutsOnly)

	// need to close mlogStore _after_ the all the indexes closed and flushed
	s.closers.AddCloser(mlogStore)

	// which feeds to replicate
	if s.Replicator == nil {
		s.Replicator, err = s.newGraphReplicator()
		if err != nil {
			return nil, err
		}
	}

	selfNf, err := s.ebtState.Inspect(s.KeyPair.Id)
	if err != nil {
		return nil, err
	}

	// no ebt state yet
	if len(selfNf) == 0 {
		// use the replication lister and determin the stored feeds lenghts
		lister := s.Replicator.Lister().ReplicationList()

		feeds, err := lister.List()
		if err != nil {
			return nil, fmt.Errorf("ebt init state: failed to get userlist: %w", err)
		}

		for i, feed := range feeds {
			if feed.Algo() != refs.RefAlgoFeedSSB1 {
				// skip other formats (TODO: gg support)
				continue
			}

			seq, err := s.CurrentSequence(feed)
			if err != nil {
				return nil, fmt.Errorf("failed to get sequence for entry %d: %w", i, err)
			}
			selfNf[feed.Ref()] = seq
		}

		selfNf[s.KeyPair.Id.Ref()], err = s.CurrentSequence(s.KeyPair.Id)
		if err != nil {
			return nil, fmt.Errorf("failed to get our sequence: %w", err)
		}

		_, err = s.ebtState.Update(s.KeyPair.Id, selfNf)
		if err != nil {
			return nil, err
		}
	}

	// from here on just network related stuff
	if s.disableNetwork {
		return s, nil
	}

	// TODO: make plugabble
	// var peerPlug *peerinvites.Plugin
	// if mt, ok := s.mlogIndicies[multilogs.IndexNameFeeds]; ok {
	// 	peerPlug = peerinvites.New(kitlog.With(log, "plugin", "peerInvites"), s, mt, s.ReceiveLog, s.PublishLog)
	// 	s.public.Register(peerPlug)
	// 	_, peerServ, err := peerPlug.OpenIndex(r)
	// 	if err != nil {
	// 		return nil, errors.Wrap(err, "sbot: failed to open about idx")
	// 	}
	// 	s.serveIndex(ctx, "contacts", peerServ)
	// }

	var inviteService *legacyinvites.Service

	// muxrpc handler creation and authoratization decider
	mkHandler := func(conn net.Conn) (muxrpc.Handler, error) {
		// bypassing badger-close bug to go through with an accept (or not) before closing the bot
		s.closedMu.Lock()
		defer s.closedMu.Unlock()

		remote, err := ssb.GetFeedRefFromAddr(conn.RemoteAddr())
		if err != nil {
			return nil, fmt.Errorf("sbot: expected an address containing an shs-bs addr: %w", err)
		}
		if s.KeyPair.Id.Equal(remote) {
			return s.master.MakeHandler(conn)
		}

		// if peerPlug != nil {
		// 	if err := peerPlug.Authorize(remote); err == nil {
		// 		return peerPlug.Handler(), nil
		// 	}
		// }

		if inviteService != nil {
			err := inviteService.Authorize(remote)
			if err == nil {
				return inviteService.GuestHandler(), nil
			}
		}

		if s.promisc {
			return s.public.MakeHandler(conn)
		}

		auth := s.authorizer
		if auth == nil {
			auth = s.Replicator.Lister()
		}

		if s.latency != nil {
			start := time.Now()
			defer func() {
				s.latency.With("part", "graph_auth").Observe(time.Since(start).Seconds())
			}()
		}
		err = auth.Authorize(remote)
		if err == nil {
			return s.public.MakeHandler(conn)
		}

		// shit - don't see a way to pass being a different feedtype with shs1
		// we also need to pass this up the stack...!
		// remote.Algo = refs.RefAlgoFeedGabby
		// err = auth.Authorize(remote)
		// if err == nil {
		// 	level.Debug(log).Log("TODO", "found gg feed, using that. overhaul shs1 to support more payload in the handshake")
		// 	return s.public.MakeHandler(conn)
		// }

		// TOFU restore/resync
		if lst, err := s.Users.List(); err == nil && len(lst) == 0 {
			level.Warn(log).Log("event", "no stored feeds - attempting re-sync with trust-on-first-use")
			s.Replicate(s.KeyPair.Id)
			return s.public.MakeHandler(conn)
		}
		return nil, err
	}

	// publish
	authorLog, err := s.Users.Get(storedrefs.Feed(s.KeyPair.Id))
	if err != nil {
		return nil, fmt.Errorf("failed to open user private index: %w", err)
	}
	s.master.Register(publish.NewPlug(kitlog.With(log, "unit", "publish"), s.PublishLog, s.Groups, authorLog))

	// private
	// TODO: box2
	userPrivs, err := s.Private.Get(librarian.Addr("box1:") + storedrefs.Feed(s.KeyPair.Id))
	if err != nil {
		return nil, fmt.Errorf("failed to open user private index: %w", err)
	}
	s.master.Register(privplug.NewPlug(
		kitlog.With(log, "unit", "private"),
		s.KeyPair.Id,
		s.Groups,
		s.PublishLog,
		private.NewUnboxerLog(s.ReceiveLog, userPrivs, s.KeyPair)))

	// whoami
	whoami := whoami.New(kitlog.With(log, "unit", "whoami"), s.KeyPair.Id)
	s.public.Register(whoami)
	s.master.Register(whoami)

	// blobs
	blobs := blobs.New(kitlog.With(log, "unit", "blobs"), s.KeyPair.Id, s.BlobStore, wm)
	s.public.Register(blobs)
	s.master.Register(blobs) // TODO: does not need to open a createWants on this one?!

	// gossiping (legacy and ebt)
	fm := gossip.NewFeedManager(
		ctx,
		s.ReceiveLog,
		s.Users,
		kitlog.With(log, "unit", "gossip"),
		s.systemGauge,
		s.eventCounter,
	)

	// outgoing gossip behavior
	var histOpts = []interface{}{
		gossip.Promisc(s.promisc),
	}

	if s.systemGauge != nil {
		histOpts = append(histOpts, s.systemGauge)
	}

	if s.eventCounter != nil {
		histOpts = append(histOpts, s.eventCounter)
	}

	var verifySink *message.VerifySink
	if s.signHMACsecret != nil {
		histOpts = append(histOpts, gossip.HMACSecret(s.signHMACsecret))
	}

	verifySink, err = message.NewVerificationSinker(s.ReceiveLog, s.Users, s.signHMACsecret)
	if err != nil {
		return nil, err
	}

	if s.disableLegacyLiveReplication {
		histOpts = append(histOpts, gossip.WithLive(!s.disableLegacyLiveReplication))
	}

	gossipPlug := gossip.NewFetcher(ctx,
		kitlog.With(log, "plugin", "gossip"),
		r,
		s.KeyPair.Id,
		s.ReceiveLog, s.Users,
		fm, s.Replicator.Lister(),
		verifySink,
		histOpts...)

	if s.disableEBT {
		s.public.Register(gossipPlug)
	} else {
		ebtPlug := ebt.NewPlug(
			kitlog.With(log, "plugin", "ebt"),
			s.KeyPair.Id,
			s.ReceiveLog,
			s.Users,
			fm,
			sm,
			verifySink,
		)
		s.public.Register(ebtPlug)

		rn := negPlugin{replicateNegotiator{
			logger: kitlog.With(log, "module", "replicate-negotiator"),

			lg:  gossipPlug.LegacyGossip,
			ebt: ebtPlug.MUXRPCHandler,
		}}
		s.public.Register(rn)
	}

	// incoming createHistoryStream handler
	hist := gossip.NewServer(ctx,
		kitlog.With(log, "unit", "gossip/hist"),
		s.KeyPair.Id,
		s.ReceiveLog, s.Users,
		s.Replicator.Lister(),
		fm,
		histOpts...)
	s.public.Register(hist)

	// get idx muxrpc handler
	s.master.Register(get.New(s, s.ReceiveLog, s.Groups))

	//
	s.master.Register(namesPlug)

	// partial wip
	plug := partial.New(s.info,
		fm,
		s.Users,
		s.ByType,
		s.Tangles,
		s.ReceiveLog, s)
	s.public.Register(plug)
	s.master.Register(plug)

	// group managment
	s.master.Register(groups.New(s.info, s.Groups))

	// raw log plugins

	sc := selfChecker{s.KeyPair.Id}
	s.master.Register(rawread.NewByTypePlugin(
		s.info,
		s.ReceiveLog,
		s.ByType,
		s.Private,
		s.Groups,
		s.SeqResolver,
		sc))
	s.master.Register(rawread.NewSequenceStream(s.ReceiveLog))
	s.master.Register(rawread.NewRXLog(s.ReceiveLog)) // createLogStream
	s.master.Register(rawread.NewSortedStream(s.info, s.ReceiveLog, s.SeqResolver))
	s.master.Register(hist) // createHistoryStream

	s.master.Register(replicate.NewPlug(s.Users))

	s.master.Register(friends.New(log, s.KeyPair.Id, s.GraphBuilder))

	mh := namedPlugin{
		h:    manifestBlob,
		name: "manifest"}
	s.master.Register(mh)
	s.public.Register(mh)

	var tplug = tangles.NewPlugin(s, s.ReceiveLog, s.Tangles, s.Private, s.Groups, sc)
	s.master.Register(tplug)

	// tcp+shs
	opts := network.Options{
		Logger:              s.info,
		Dialer:              s.dialer,
		ListenAddr:          s.listenAddr,
		AdvertsSend:         s.enableAdverts,
		AdvertsConnectTo:    s.enableDiscovery,
		KeyPair:             s.KeyPair,
		AppKey:              s.appKey[:],
		MakeHandler:         mkHandler,
		ConnTracker:         s.networkConnTracker,
		BefreCryptoWrappers: s.preSecureWrappers,
		AfterSecureWrappers: s.postSecureWrappers,

		EventCounter:    s.eventCounter,
		SystemGauge:     s.systemGauge,
		EndpointWrapper: s.edpWrapper,
		Latency:         s.latency,

		WebsocketAddr: s.websocketAddr,
	}

	networkNode, err := network.New(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create network node: %w", err)
	}
	blobsPathPrefix := "/blobs/get/"
	h := cors.Default().Handler(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !strings.HasPrefix(req.URL.Path, blobsPathPrefix) {
			http.Error(w, "404", http.StatusNotFound)
			return
		}

		rest := strings.TrimPrefix(req.URL.Path, blobsPathPrefix)
		blobRef, err := refs.ParseBlobRef(rest)
		if err != nil {
			level.Error(log).Log("http-err", err.Error())
			http.Error(w, "bad blob", http.StatusBadRequest)
			return
		}

		br, err := s.BlobStore.Get(blobRef)
		if err != nil {
			s.WantManager.Want(blobRef)
			level.Error(log).Log("http-err", err.Error())
			http.Error(w, "no such blob", http.StatusNotFound)
			return
		}

		// wh := w.Header()
		// sniff content-type?
		w.WriteHeader(http.StatusOK)
		_, err = io.Copy(w, br)
		if err != nil {
			level.Error(log).Log("http-blob", err.Error())
		}
	}))
	networkNode.HandleHTTP(h)

	inviteService, err = legacyinvites.New(
		kitlog.With(log, "unit", "legacyInvites"),
		r,
		s.KeyPair.Id,
		networkNode,
		s.PublishLog,
		s.ReceiveLog,
	)
	if err != nil {
		return nil, fmt.Errorf("sbot: failed to open legacy invites plugin: %w", err)
	}
	s.master.Register(inviteService.MasterPlugin())

	// TODO: should be gossip.connect but conflicts with our namespace assumption
	s.master.Register(control.NewPlug(kitlog.With(log, "unit", "ctrl"), networkNode, s))
	s.master.Register(status.New(s))

	s.public.Register(networkNode.TunnelPlugin())
	s.Network = networkNode

	return s, nil
}

type selfChecker struct {
	me refs.FeedRef
}

func (sc selfChecker) Authorize(remote refs.FeedRef) error {
	if sc.me.Equal(remote) {
		return nil
	}
	return fmt.Errorf("not authorized")
}

func (s *Sbot) CurrentSequence(feed refs.FeedRef) (ssb.Note, error) {
	l, err := s.Users.Get(storedrefs.Feed(feed))
	if err != nil {
		return ssb.Note{}, fmt.Errorf("failed to get user log for %s: %w", feed.ShortRef(), err)
	}
	sv, err := l.Seq().Value()
	if err != nil {
		return ssb.Note{}, fmt.Errorf("failed to get sequence for user log %s: %w", feed.ShortRef(), err)
	}

	currSeq := sv.(margaret.BaseSeq)
	if currSeq != -1 {
		currSeq++
	}

	return ssb.Note{
		Seq:       currSeq.Seq(),
		Replicate: true,
		Receive:   true, // TODO: not exactly... we might be getting this feed from somewhre else
	}, nil
}
