// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package migrate

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"

	"github.com/ssbc/go-luigi"
	"github.com/ssbc/margaret"
	libbadger "github.com/ssbc/margaret/indexes/badger"
	extralog "go.mindeco.de/log"

	"github.com/ssbc/go-metafeed"
	"github.com/ssbc/go-metafeed/metakeys"
	"github.com/ssbc/go-metafeed/metamngmt"
	"github.com/ssbc/go-ssb"
	"github.com/ssbc/go-ssb/internal/mutil"
	"github.com/ssbc/go-ssb/internal/slp"
	"github.com/ssbc/go-ssb/internal/storedrefs"
	"github.com/ssbc/go-ssb/message/legacy"
	"github.com/ssbc/go-ssb/private/box"
	"github.com/ssbc/go-ssb/private/keys"
	"github.com/ssbc/go-ssb/query"
	"github.com/ssbc/go-ssb/repo"
	"github.com/ssbc/go-ssb/sbot"
	refs "github.com/ssbc/go-ssb-refs"
	"github.com/ssbc/go-ssb-refs/tfk"
)

func migrationAlreadyRun(ssbdir string) bool {
	exists, err := existsMigrationSentinel(ssbdir)
	check(err)
	return exists
}

func check(err error) {
	if err != nil {
		log.Fatalln(err)
	}
}

func Run(ssbdir string) error {
	var err error
	if migrationAlreadyRun(ssbdir) {
		inform("migration already ran")
		return nil
	}

	inform("migrating", ssbdir)
	// load the classic keypair from disk & read its pubkey
	classicKeypair, err := loadClassicKeypair(ssbdir)
	// the main ref containted in the classic secret
	mainRef := classicKeypair.ID()

	// move the secret so that we don't start the metafeed sbot from it
	err = moveExistingSecret(ssbdir)
	if err != nil {
		return err
	}
	// start a metafeed sbot & get its id (then power it down again; we want the keystore to be available)
	mfID, err := getMetafeedID(ssbdir)
	if err != nil {
		return err
	}
	// open the key store and insert the classic key
	err = ingestKeypair(mfID, classicKeypair, ssbdir)
	if err != nil {
		return err
	}
	// start the metafeed sbot
	bot, err := setupMetafeedSbot(ssbdir)
	if err != nil {
		return err
	}

	aboutMessages, err := getMessages(bot, mainRef, "about")
	if err != nil {
		return err
	}
	contactMessages, err := getMessages(bot, mainRef, "contact")
	if err != nil {
		return err
	}

	// create & get the index feeds ontop of the `indexes` subfeed
	aboutIndex, err := bot.MetaFeeds.GetOrCreateIndex(mfID, mainRef, "index", "about")
	if err != nil {
		return err
	}
	inform("created about index")
	contactIndex, err := bot.MetaFeeds.GetOrCreateIndex(mfID, mainRef, "index", "contact")
	if err != nil {
		return err
	}
	inform("created contact index")

	// post & index the about messages we found in the main feed to the about index
	err = indexMessages(bot, aboutIndex, aboutMessages)
	if err != nil {
		return err
	}
	// post & index the contact messages we found in the main feed to the contact index
	err = indexMessages(bot, contactIndex, contactMessages)
	if err != nil {
		return err
	}

	// register indices
	err = bot.MetaFeeds.RegisterIndex(mfID, mainRef, "about")
	if err != nil {
		return err
	}
	err = bot.MetaFeeds.RegisterIndex(mfID, mainRef, "contact")
	if err != nil {
		return err
	}

	// post the metafeed/announce message (announcing that "hey i am upgrading to metafeeds!! go get partial w/ me")
	err = postAnnouncement(bot, mainRef)
	if err != nil {
		return err
	}

	// ACK the main feed from the metafeed, making the meta-ness mutual
	err = registerMainOnMetafeed(bot, mfID, classicKeypair)
	if err != nil {
		return err
	}

	// assert the keypair type -> lets us access the metafeed seed for sending in pm
	mfKeypair, ok := bot.KeyPair.(metakeys.KeyPair)
	if !ok {
		return ew("asset metafeed keypair")("failed to assert")
	}
	err = sendSeedInPM(bot, mfKeypair, mainRef)
	if err != nil {
		return err
	}

	// cleanly shutdown the bot
	bot.Shutdown()
	err = bot.Close()
	if err != nil {
		return err
	}

	return nil
}

// GetDefaultPath returns the default go-ssb repo location ($HOME/.ssb-go).
func GetDefaultPath() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", ew("default path")("error getting info on current user", err)
	}
	return filepath.Join(u.HomeDir, ".ssb-go"), nil
}

func inform(parts ...interface{}) {
	// TODO: add -v flag to toggle inform output once tool is stable
	// insert our prefix string `gossb-migrate-mf` before the other elements in `parts`
	parts = append(parts[:1], parts[:]...)
	parts[0] = "gossb-migrate-mf:"
	fmt.Println(parts...)
}

// error wrap.
// string header will be prefixed before each message. typically it is the context we're generating errors within.
// msg is the specific message, err is the error (if passed)
func ew(header string) func(msg string, err ...error) error {
	inform(header)
	return func(msg string, err ...error) error {
		if len(err) > 0 {
			return fmt.Errorf("[gossb-migrate-mf: %s] %s (%w)", header, msg, err[0])
		}
		return fmt.Errorf("[gossb-migrate-mf: %s] %s", header, msg)
	}
}

const sentinelName = ".metafeeds-upgraded"
// Creates an empty file, signaling that the migration has run successfully. go-sbot will look for this file to
// determine if it is to start up in metafeedsEnabled mode or not—and this tool looks for the file before continuing to
// run the migration (should the sentinel not exist yet).
func persistMigrationSentinel(ssbdir string) error {
	e := ew("migration sentinel")
	exists, err := existsMigrationSentinel(ssbdir)
	if err != nil {
		return e("error when reading sentinel", err)
	}
	// sentinel already exists, just exit
	if exists {
		return nil
	}
	sentinel := filepath.Join(ssbdir, sentinelName)
	fmt.Println(sentinel)
	os.MkdirAll(ssbdir, 0777)
	err = os.WriteFile(sentinel, []byte{}, 0666)
	if err != nil {
		return e("failed to write sentinel", err)
	}
	return nil
}

func existsMigrationSentinel(ssbdir string) (bool, error) {
	sentinel := filepath.Join(ssbdir, sentinelName)
	info, err := os.Stat(sentinel)
	// file exists
	if info != nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// Read the classic pre-migration secret and returns the keypair representing it
func loadClassicKeypair(ssbdir string) (ssb.KeyPair, error) {
	e := ew("load classic keypair")
	secretdir := filepath.Join(ssbdir, "secret")
	// read secret file contents
	keypair, err := ssb.LoadKeyPair(secretdir)
	if err != nil {
		return ssb.LegacyKeyPair{}, e("failed to read secret file", err)
	}
	return keypair, nil
}

// TODO (2021-10-08): note down length of offset log before migrating -> allow for resetting offset log by trimming to
// old offset
func moveExistingSecret(ssbdir string) error {
	var err error
	e := ew("move backup")
	backupdir := filepath.Join(ssbdir, "backup")
	err = os.MkdirAll(backupdir, 0777)
	if err != nil {
		return e("failed to make directory", err)
	}

	secretpath := filepath.Join(ssbdir, "secret")
	newpath := filepath.Join(backupdir, "secret")
	// move the secret file to another location (~/.ssb-go/backup/secret)
	err = os.Rename(secretpath, newpath)
	if err != nil {
		return e(fmt.Sprintf("failed to move secret from %s to %s", secretpath, newpath), err)
	}
	return nil
}

func setupMetafeedSbot(ssbdir string) (*sbot.Sbot, error) {
	options := []sbot.Option{
		sbot.WithMetaFeedMode(true),
		sbot.WithRepoPath(ssbdir),
		sbot.WithInfo(extralog.NewNopLogger()), // silence the go bot output
	}
	bot, err := sbot.New(options...)
	if err != nil {
		return &sbot.Sbot{}, ew("start metafeed sbot")("failed to create sbot", err)
	}

	return bot, nil
}

func setupRegularSbot(ssbdir string) (*sbot.Sbot, error) {
	options := []sbot.Option{
		sbot.WithMetaFeedMode(false),
		sbot.WithRepoPath(ssbdir),
	}
	bot, err := sbot.New(options...)
	if err != nil {
		return &sbot.Sbot{}, ew("start regular sbot")("failed to create sbot", err)
	}

	return bot, nil
}

// Add the mainfeed keypair to the keys kv-store
func ingestKeypair(mfID refs.FeedRef, keypair ssb.KeyPair, ssbdir string) error {
	var err error
	inform("add the classic key", keypair.ID().ShortSigil(), "to", mfID.ShortSigil())
	e := ew("ingest keypair")

	// open the keystore
	storageRepo := repo.New(ssbdir)
	indexstore, err := repo.OpenBadgerDB(storageRepo.GetPath(repo.PrefixMultiLog, "shared-badger"))
	if err != nil {
		return e("failed to open badger", err)
	}
	idxKeys := libbadger.NewIndexWithKeyPrefix(indexstore, keys.Recipients{}, []byte("group-and-signing"))
	keystore := &keys.Store{
		Index: idxKeys,
	}

	// add the classic key to the root metafeed's keystore
	pubkeyRef := keypair.ID()

	newSubfeedAsTFK, err := tfk.Encode(pubkeyRef)
	if err != nil {
		return e("failed tfk encoding", err)
	}

	// use sequence number 0 to signal there is no corresponding message on the metafeed
	// i.e. this is just a hack to add keys to the key store (all other messages are directly correlated to sequences in
	// the metafeed, but not this sucker!!)
	//
	// TODO (2021-10-07): use the sequence number generated from adding the main feed to the root metafeed as a
	// message of type `metafeed/add/existing`
	keyTuple, err := slp.Encode(newSubfeedAsTFK, []byte{0})
	if err != nil {
		return e("failed shallow length prefix encode", err)
	}

	mfStoreID := keys.IDFromFeed(mfID)
	// first register the pubkey as a subfeed on the root metafeed
	err = keystore.AddKey(mfStoreID, keys.Recipient{
		Key:    keys.Key(keyTuple),
		Scheme: keys.SchemeMetafeedSubkey,

		/* TODO (2021-10-04) what kind of metadata would this be? `metafeed/add/existing`? */
		Metadata: keys.Metadata{
			ForFeed: &pubkeyRef,
		},
	})

	// then associate the feed's secret key with the pubkey
	feedAsTFK, err := tfk.Encode(pubkeyRef)
	if err != nil {
		return e("failed to encode pubkey as tfk", err)
	}
	keyMaterial, err := slp.Encode(keypair.Secret(), bytes.Repeat([]byte{0}, 32))
	if err != nil {
		return e("failed to encode secret", err)
	}
	pubkeyStoreID := keys.ID(feedAsTFK)
	err = keystore.AddKey(pubkeyStoreID, keys.Recipient{
		Key:    keyMaterial,
		Scheme: keys.SchemeFeedMessageSigningKey,
	})
	if err != nil {
		return e("failed to add key", err)
	}

	// close the key store
	err = idxKeys.Close()
	if err != nil {
		e("failed to close index with key prefix§", err)
	}
	err = indexstore.Close()
	if err != nil {
		e("failed to close index store", err)
	}
	return nil
}

func printLog(bot *sbot.Sbot, feedid refs.FeedRef) error {
	e := ew("print log")

	l, err := getFeed(bot, feedid)
	if err != nil {
		return e(fmt.Sprintf("couldnt get log %s", feedid.ShortSigil()), err)
	}

	src, err := l.Query()
	if err != nil {
		return e("failed to query log", err)
	}

	seq := l.Seq()
	i := int64(0)
	inform("last seqno:", seq)

	for {
		v, err := src.Next(context.TODO())
		if luigi.IsEOS(err) {
			break
		}
		inform("value:", v)
		mm, ok := v.(refs.Message)
		if !ok {
			return e(fmt.Sprintf("expected %T to be a refs.Message (wrong log type? missing indirection to receive log?)", v))
		}

		fmt.Printf("log seq: %d - %s:%d (%s)\n",
			i,
			mm.Author().ShortSigil(),
			mm.Seq(),
			mm.Key().ShortSigil())

		b := mm.ContentBytes()
		if n := len(b); n > 128 {
			fmt.Println("truncating", n, " to last 32 bytes")
			b = b[len(b)-32:]
		}
		fmt.Printf("\n%s\n", hex.Dump(b))

		i++
	}

	// margaret is 0-indexed
	seq++
	if seq != i {
		return e(fmt.Sprintf("seq differs from iterated count: %d vs %d", seq, i))
	}
	return nil
}

func getFeed(bot *sbot.Sbot, feedID refs.FeedRef) (margaret.Log, error) {
	feed, err := bot.Users.Get(storedrefs.Feed(feedID))
	if err != nil {
		return nil, ew("get feed")("failed", err)
	}

	// convert from log of seqnos-in-rxlog to log of refs.Message and return
	return mutil.Indirect(bot.ReceiveLog, feed), nil
}

func getMessages(bot *sbot.Sbot, main refs.FeedRef, msgtype string) ([]refs.Message, error) {
	e := ew("get messages")
	var err error
	inform(fmt.Sprintf("get %s for %s", msgtype, main.ShortSigil()))

	q := query.NewSubsetAndCombination(
		query.NewSubsetOpByAuthor(main),
		query.NewSubsetOpByType(msgtype),
	)
	planner := query.NewSubsetPlaner(bot.Users, bot.ByType)
	msgs, err := planner.QuerySubsetMessages(bot.ReceiveLog, q)
	inform(bot.ReceiveLog.Seq(), "messages in receive log")
	if err != nil {
		return []refs.Message{}, e("failed to query subset", err)
	}
	inform(len(msgs), "matching", msgtype, "messages")
	return msgs, nil
}

// Get (or generate) the root metafeed identity
func getMetafeedID(ssbdir string) (refs.FeedRef, error) {
	// setup a metafeed bot -> generates a new secret + a root metafeed of feed format type bendybutt v1
	e := ew("get metafeed id")
	// temporarily start the metafeed-enabled sbot to get the mf id (so we can reference it when adding the classic key to
	// the key store)
	bot, err := setupMetafeedSbot(ssbdir)
	if err != nil {
		return refs.FeedRef{}, e("failed to start metafeed sbot", err)
	}
	mfID := bot.KeyPair.ID()
	inform("root mf id is", mfID)
	bot.Shutdown()
	err = bot.Close()
	if err != nil {
		return refs.FeedRef{}, e("failed to close", err)
	}
	return mfID, nil
}

type indexMessage struct {
	Type     string          `json:"type"`
	Key      refs.MessageRef `json:"key"`
	Sequence int64           `json:"sequence"`
}

func indexMessages(bot *sbot.Sbot, indexfeed refs.FeedRef, messages []refs.Message) error {
	e := ew("index messages")
	var err error
	for _, msg := range messages {
		indexed := indexMessage{Type: "metafeed/index", Key: msg.Key(), Sequence: msg.Seq()}
		_, err = bot.MetaFeeds.Publish(indexfeed, indexed)
		if err != nil {
			return e(fmt.Sprintf("failed to publish indexed message to %s", indexfeed.ShortSigil()), err)
		}
	}
	return nil
}

func postAnnouncement(bot *sbot.Sbot, main refs.FeedRef) error {
	e := ew("post metafeed announcement")
	announcement := legacy.NewMetafeedAnnounce(bot.KeyPair.ID(), main)

	signedMsg, err := announcement.Sign(bot.KeyPair.Secret(), nil)
	if err != nil {
		return e("failed to sign", err)
	}

	_, err = bot.MetaFeeds.Publish(main, signedMsg)
	if err != nil {
		return e("failed to publish", err)
	}

	return nil
}

func registerMainOnMetafeed(bot *sbot.Sbot, mf refs.FeedRef, mainKeypair ssb.KeyPair) error {
	var err error
	e := ew("register main on root mf")
	mfAddExisting := metamngmt.NewAddExistingMessage(mf, mainKeypair.ID(), "main")
	mfAddExisting.Tangles["metafeed"] = refs.TanglePoint{Root: nil, Previous: nil}

	// sign the bendybutt message with mf.Secret + main.Secret
	signedAddExistingContent, err := metafeed.SubSignContent(mainKeypair.Secret(), mfAddExisting)
	if err != nil {
		return e("failed to sign bendy butt `metafeed/add/existing`", err)
	}

	_, err = bot.MetaFeeds.Publish(mf, signedAddExistingContent)
	if err != nil {
		return e("failed to publish signed add/existing message", err)
	}
	return nil
}

func createCiphertext(content []byte, recipients ...refs.FeedRef) ([]byte, error) {
	boxer := box.NewBoxer(nil)
	ciphertext, err := boxer.Encrypt(content, recipients...)
	if err != nil {
		return nil, fmt.Errorf("error encrypting message (box1): %w", err)
	}
	return ciphertext, nil
}

func sendSeedInPM(bot *sbot.Sbot, mfKeypair metakeys.KeyPair, mainRef refs.FeedRef) error {
	e := ew("send seed in pm")
	/* {
	"type": "metafeed/seed",
	"metafeed": ssb:feed/bendybutt-v1/bendyButtFeedID,
	"seed": seedBytesEncodedAsHexString
	*/
	type mfSeed struct {
		Type     string `json:"type"`
		Metafeed string `json:"metafeed"`
		Seed     string `json:"seed"`
	}

	content := mfSeed{
		Type:     "metafeed/seed",
		Metafeed: mfKeypair.ID().String(),
		Seed:     fmt.Sprintf("%x", mfKeypair.Seed),
	}
	encodedContent, err := json.Marshal(content)
	if err != nil {
		return e("failed to marshal mf seed", err)
	}
	ciphertext, err := createCiphertext(encodedContent, mainRef)
	if err != nil {
		return e("failed to construct ciphertext", err)
	}
	_, err = bot.MetaFeeds.Publish(mainRef, ciphertext)
	if err != nil {
		return e("failed to publish the ciphertext pm to ourselves", err)
	}
	// we did it! we sent a pm to ourselves! wow
	return nil
}
