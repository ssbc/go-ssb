// SPDX-License-Identifier: MIT

package sbot

import (
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	kitlog "github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/rs/cors"
	"go.cryptoscope.co/librarian"
	libmkv "go.cryptoscope.co/librarian/mkv"
	"go.cryptoscope.co/margaret/multilog/roaring"
	"go.cryptoscope.co/muxrpc"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/blobstore"
	"go.cryptoscope.co/ssb/graph"
	"go.cryptoscope.co/ssb/indexes"
	"go.cryptoscope.co/ssb/internal/ctxutils"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/multilogs"
	"go.cryptoscope.co/ssb/network"
	"go.cryptoscope.co/ssb/plugins/blobs"
	"go.cryptoscope.co/ssb/plugins/control"
	"go.cryptoscope.co/ssb/plugins/friends"
	"go.cryptoscope.co/ssb/plugins/get"
	"go.cryptoscope.co/ssb/plugins/gossip"
	"go.cryptoscope.co/ssb/plugins/legacyinvites"
	"go.cryptoscope.co/ssb/plugins/partial"
	privplug "go.cryptoscope.co/ssb/plugins/private"
	"go.cryptoscope.co/ssb/plugins/publish"
	"go.cryptoscope.co/ssb/plugins/rawread"
	"go.cryptoscope.co/ssb/plugins/replicate"
	"go.cryptoscope.co/ssb/plugins/status"
	"go.cryptoscope.co/ssb/plugins/whoami"
	"go.cryptoscope.co/ssb/plugins2/names"
	"go.cryptoscope.co/ssb/plugins2/tangles"
	"go.cryptoscope.co/ssb/private"
	"go.cryptoscope.co/ssb/private/keys"
	"go.cryptoscope.co/ssb/repo"
	refs "go.mindeco.de/ssb-refs"

	multifs "go.cryptoscope.co/margaret/multilog/roaring/fs"
)

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
			s.closeErr = errors.Wrap(err, "sbot: failed to close own network node")
			return s.closeErr
		}
		s.Network.GetConnTracker().CloseAll()
		level.Debug(closeEvt).Log("msg", "connections closed")
	}

	if err := s.idxDone.Wait(); err != nil {
		s.closeErr = errors.Wrap(err, "sbot: index group shutdown failed")
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
	s.rootCtx, s.Shutdown = ctxutils.WithError(s.rootCtx, ssb.ErrShuttingDown)
	ctx := s.rootCtx

	r := repo.New(s.repoPath)

	// optionize?!
	s.ReceiveLog, err = repo.OpenLog(r)
	if err != nil {
		return nil, errors.Wrap(err, "sbot: failed to open rootlog")
	}
	s.closers.addCloser(s.ReceiveLog.(io.Closer))

	if s.BlobStore == nil { // load default, local file blob store
		s.BlobStore, err = repo.OpenBlobStore(r)
		if err != nil {
			return nil, errors.Wrap(err, "sbot: failed to open blob store")
		}
	}

	wantsLog := kitlog.With(log, "module", "WantManager")
	wm := blobstore.NewWantManager(s.BlobStore,
		blobstore.WantWithLogger(wantsLog),
		blobstore.WantWithContext(s.rootCtx),
		blobstore.WantWithMetrics(s.systemGauge, s.eventCounter),
	)
	s.WantManager = wm
	s.closers.addCloser(wm)

	for _, opt := range s.lateInit {
		err := opt(s)
		if err != nil {
			return nil, errors.Wrap(err, "sbot: failed to apply late option")
		}
	}

	s.SeqResolver, err = repo.NewSequenceResolver(r)
	if err != nil {
		return nil, errors.Wrap(err, "error opening sequence resolver")
	}
	s.closers.addCloser(s.SeqResolver)

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
		mlogPath := r.GetPath(repo.PrefixMultiLog, index.Name, "fs-bitmaps")

		ml, err := multifs.NewMultiLog(mlogPath)
		if err != nil {
			return nil, errors.Wrapf(err, "sbot: failed to open multilog %s", index.Name)
		}
		s.closers.addCloser(ml)
		s.mlogIndicies[index.Name] = ml

		if err := ml.CompressAll(); err != nil {
			return nil, errors.Wrapf(err, "sbot: failed compress multilog %s", index.Name)
		}

		*index.Mlog = ml
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
		return nil, errors.Wrap(err, "sbot: failed to create publish log")
	}

	err = MountSimpleIndex("get", indexes.OpenGet)(s)
	if err != nil {
		return nil, err
	}

	// groups2
	pth := r.GetPath(repo.PrefixIndex, "groups-keys", "mkv")
	err = os.MkdirAll(pth, 0700)
	if err != nil {
		return nil, errors.Wrap(err, "openIndex: error making index directory")
	}

	db, err := repo.OpenMKV(pth)
	if err != nil {
		return nil, errors.Wrap(err, "openIndex: failed to open MKV database")
	}

	idx := libmkv.NewIndex(db, keys.Recipients{})
	ks := &keys.Store{
		Index: idx,
	}
	s.closers.addCloser(idx)

	s.Groups = private.NewManager(s.KeyPair, s.PublishLog, ks, s.Tangles)

	combIdx, err := multilogs.NewCombinedIndex(
		s.repoPath,
		s.Groups,
		s.KeyPair.Id,
		s.SeqResolver,
		s.Users,
		s.Private,
		s.ByType,
		s.Tangles)
	if err != nil {
		return nil, errors.Wrap(err, "sbot: failed to open combined application index")
	}
	s.serveIndex("combined", combIdx)
	s.closers.addCloser(combIdx)

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
	contactLog, err := s.ByType.Get(librarian.Addr("contact"))
	if err != nil {
		return nil, errors.Wrap(err, "sbot: failed to open message contact sublog")
	}
	justContacts := mutil.Indirect(s.ReceiveLog, contactLog)

	// LogBuilder doesn't fully work yet
	if false {
		level.Warn(s.info).Log("event", "bot init", "msg", "using experimental bytype:contact graph implementation")

		s.GraphBuilder, err = graph.NewLogBuilder(s.info, justContacts)
		if err != nil {
			return nil, errors.Wrap(err, "sbot: NewLogBuilder failed")
		}
	} else {
		gb, seqSetter, updateIdx, err := indexes.OpenContacts(kitlog.With(log, "module", "graph"), r)
		if err != nil {
			return nil, errors.Wrap(err, "sbot: OpenContacts failed")
		}

		s.serveIndexFrom("contacts", updateIdx, justContacts)
		s.closers.addCloser(seqSetter)
		s.GraphBuilder = gb
	}

	// abouts
	aboutSeqs, err := s.ByType.Get(librarian.Addr("about"))
	if err != nil {
		return nil, errors.Wrap(err, "sbot: failed to open message about sublog")
	}
	aboutsOnly := mutil.Indirect(s.ReceiveLog, aboutSeqs)

	var namesPlug names.Plugin
	_, aboutSnk, err := namesPlug.MakeSimpleIndex(r)
	if err != nil {
		return nil, errors.Wrap(err, "sbot: failed to open about idx")
	}
	s.closers.addCloser(aboutSnk)
	s.serveIndexFrom("abouts", aboutSnk, aboutsOnly)

	// from here on just network related stuff
	if s.disableNetwork {
		return s, nil
	}

	// which feeds to replicate
	if s.Replicator == nil {
		s.Replicator, err = s.newGraphReplicator()
		if err != nil {
			return nil, err
		}
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
			return nil, errors.Wrap(err, "sbot: expected an address containing an shs-bs addr")
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
		remote.Algo = refs.RefAlgoFeedGabby
		err = auth.Authorize(remote)
		if err == nil {
			level.Debug(log).Log("TODO", "found gg feed, using that. overhaul shs1 to support more payload in the handshake")
			return s.public.MakeHandler(conn)
		}
		if lst, err := s.Users.List(); err == nil && len(lst) == 0 {
			level.Warn(log).Log("event", "no stored feeds - attempting re-sync with trust-on-first-use")
			return s.public.MakeHandler(conn)
		}
		return nil, err
	}

	// publish
	s.master.Register(publish.NewPlug(kitlog.With(log, "plugin", "publish"), s.PublishLog, s.ReceiveLog))

	// private
	// TODO: box2
	userPrivs, err := s.Private.Get(librarian.Addr("box1:") + s.KeyPair.Id.StoredAddr())
	if err != nil {
		return nil, errors.Wrap(err, "failed to open user private index")
	}
	s.master.Register(privplug.NewPlug(
		kitlog.With(log, "plugin", "private"),
		s.KeyPair.Id,
		s.Groups,
		s.PublishLog,
		private.NewUnboxerLog(s.ReceiveLog, userPrivs, s.KeyPair)))

	// whoami
	whoami := whoami.New(kitlog.With(log, "plugin", "whoami"), s.KeyPair.Id)
	s.public.Register(whoami)
	s.master.Register(whoami)

	// blobs
	blobs := blobs.New(kitlog.With(log, "plugin", "blobs"), *s.KeyPair.Id, s.BlobStore, wm)
	s.public.Register(blobs)
	s.master.Register(blobs) // TODO: does not need to open a createWants on this one?!

	// outgoing gossip behavior
	var histOpts = []interface{}{
		gossip.HopCount(s.hopCount),
		gossip.Promisc(s.promisc),
	}

	if s.systemGauge != nil {
		histOpts = append(histOpts, s.systemGauge)
	}

	if s.eventCounter != nil {
		histOpts = append(histOpts, s.eventCounter)
	}

	if s.signHMACsecret != nil {
		var k [32]byte
		copy(k[:], s.signHMACsecret)
		histOpts = append(histOpts, gossip.HMACSecret(&k))
	}

	fm := gossip.NewFeedManager(
		ctx,
		s.ReceiveLog,
		s.Users,
		kitlog.With(log, "feedmanager"),
		s.systemGauge,
		s.eventCounter,
	)
	s.public.Register(gossip.New(ctx,
		kitlog.With(log, "plugin", "gossip"),
		s.KeyPair.Id, s.ReceiveLog, s.Users, fm, s.Replicator.Lister(),
		histOpts...))

	// incoming createHistoryStream handler
	hist := gossip.NewHist(ctx,
		kitlog.With(log, "plugin", "gossip/hist"),
		s.KeyPair.Id,
		s.ReceiveLog, s.Users,
		s.Replicator.Lister(),
		fm,
		histOpts...)
	s.public.Register(hist)

	// get idx muxrpc handler
	s.master.Register(get.New(s, s.ReceiveLog))

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

	// raw log plugins

	s.master.Register(rawread.NewByTypePlugin(s.info, s.ReceiveLog, s.ByType, s.SeqResolver))
	s.master.Register(rawread.NewSequenceStream(s.ReceiveLog))
	s.master.Register(rawread.NewRXLog(s.ReceiveLog))                               // createLogStream
	s.master.Register(rawread.NewSortedStream(s.info, s.ReceiveLog, s.SeqResolver)) // createLogStream
	s.master.Register(hist)                                                         // createHistoryStream

	s.master.Register(replicate.NewPlug(s.Users))

	s.master.Register(friends.New(log, *s.KeyPair.Id, s.GraphBuilder))

	mh := namedPlugin{
		h:    manifestHandler(manifestBlob),
		name: "manifest"}
	s.master.Register(mh)

	var tplug tangles.Plugin
	tplug.WantRootLog(s.ReceiveLog)
	tplug.UseMultiLog(s.Tangles)
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

	s.Network, err = network.New(opts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create network node")
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
	s.Network.HandleHTTP(h)

	inviteService, err = legacyinvites.New(
		kitlog.With(log, "plugin", "legacyInvites"),
		r,
		s.KeyPair.Id,
		s.Network,
		s.PublishLog,
		s.ReceiveLog,
	)
	if err != nil {
		return nil, errors.Wrap(err, "sbot: failed to open legacy invites plugin")
	}
	s.master.Register(inviteService.MasterPlugin())

	// TODO: should be gossip.connect but conflicts with our namespace assumption
	s.master.Register(control.NewPlug(kitlog.With(log, "plugin", "ctrl"), s.Network, s))
	s.master.Register(status.New(s))

	return s, nil
}
