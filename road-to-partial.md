# go-ssb partial replication support

This document outlines the needed work to implement metafeeds and partial replication features in go-ssb

# done changes

* implement the de- and encoders new [BendyButt format](github.com/ssb-ngi-pointer/bendy-butt-spec/): [go-metafeed](https://github.com/ssb-ngi-pointer/go-metafeed)
* enable metafeed mode with the new option `sbot.WithMetaFeedMode(bool)`
* `sbot.MetaFeeds` API for managing feeds

```go
// MetaFeeds allows managing and publishing to subfeeds of a metafeed.
type MetaFeeds interface {
	// CreateSubFeed derives a new keypair, stores it in the keystore and publishes a `metafeed/add` message on the housing metafeed.
	// It takes purpose whic will be published and added to the keystore, too.
	// The subfeed will use the format.
	CreateSubFeed(purpose string, format refs.RefAlgo) (refs.FeedRef, error)

	// TombstoneSubFeed removes the keypair from the store and publishes a `metafeed/tombstone` message to the feed.
	// Afterwards the referenced feed is unusable.
	TombstoneSubFeed(refs.FeedRef) error

	// ListSubFeeds returns a list of all _active_ subfeeds of the housing metafeed
	ListSubFeeds() ([]SubfeedListEntry, error)

	// Publish works like normal `Sbot.Publish()` but takes an additional feed reference,
	// which specifies the subfeed on which the content should be published.
	Publish(as refs.FeedRef, content interface{}) (refs.MessageRef, error)
}
```

* patch `createHistoryStream` to support sending and receiving bendy-butt messages (_legacy_ replication)

# upcoming changes

These are necessary to get a functional partial replication. They are orderd by dependence/necessity (A needs B) not complexity.

## Restore from an existing bendy-butt keypair

With classic keypairs this is fairly simple. If the database is empty, it just requests it's own feed, too.

The added complexity here is that we have to "index" our own feed and reinstate the subfeed keypairs from the nonces and save them to the local keystore.

## Handle migrating from an existing keypair

If `sbot.WithMetaFeedMode(true)` is passed to `sbot.New()` and it finds a keypair that isn't for a metafeed, the bot should create a new metafeed and announce it on the existing one.

## implement content [validation](https://github.com/ssb-ngi-pointer/bendy-butt-spec#validation) of metafeeds
Right now go-metafeed only handles the overal entry verification but we also need to verify the content portion. There already exists [VerifySubSignedContent](https://pkg.go.dev/github.com/ssb-ngi-pointer/go-metafeed#VerifySubSignedContent) to help with this but it needs to be integrated (aka the buisness logic of what to do with these messages).

**note to self**: instead of wanting a _prefix query_ for `messagesByTypes` (like `metafeed/*`) we could open the individual types and join them into a single source.

## feed replicator
Overhaul the `graph` package, which currently only consumes `type:contact` messages.

The wanted feature is that it can tell us which (sub)feeds belong to an identity. Which in turn will be fed into the replication engine.

With this structure in place, it's then possible to implement rules of how to walk a tree of identities and which ones to replicate in full and which ones to fetch partially. See [this outline](https://github.com/ssb-ngi-pointer/ssb-secure-partial-replication-impl-spec#ssb-feed-replicator) for how JS does it.

## subset replication support
Implement [new RPC method `getSubset`](https://github.com/ssb-ngi-pointer/ssb-subset-replication-spec#getsubsetquery-options-source).

We wouldn't have to implement `ssb-ql-1` since the query data is transmitted as JSON. What we do need to implement is a generic query planer that can receive the JSON structures, combine the different multi/sublogs and evaluate the intended bitmaps before returning the messages.

## [index feed](https://github.com/ssb-ngi-pointer/ssb-meta-feed-spec#claims-or-indexes) writer.
Come up with a way/API to configure index feed creation for _own_ feeds.

One major gotcha here is that a straightforward way for this would be inside the indexes _BUTT_ it's not possible to publish new messages inside those. A workaround could be some kind of queueing system.

## index feed resolving

To fetch partial feeds we need to be able to replicate the corresponding _index feeds_. For that purpose there will be a [new RPC method](https://github.com/ssb-ngi-pointer/ssb-subset-replication-spec#getindexfeedfeedid-source) which inlines/joins the _actual_ index feed contents to the message content.

Fetching and ingesting these streams shouldn't need much changes. Splice out the contents and pipe them into a [verification sink](https://pkg.go.dev/go.cryptoscope.co/ssb/message#VerifySink).

Tangential needed change here is to loosen the restrictions on _in order_ fetching and change it to _ignore duplicates_, most likly via a bitmap per feed.

# Future _nice to have_'s

These would be _cool_ but the list above is already large enough as it is.

## add HMAC support to go-metafeed
This should be done if only to achive feature parity with test networks.

## implement binary support for EBT

Currently the implementation can only replicate _legacy_ feed format. The proposed solution for binary support is to have multiple ebt streams (one per format) which then allows to send binary data in the muxrpc frames instead of JSON. The required refactor is not that large. The feed format needs to be configured and the _EBT session spawning_ code needs to account for the enabled/supported formats.

## [fusion identities](https://github.com/ssb-ngi-pointer/fusion-identity-spec) identities

_should_ mostly just need some box2 handwaving for the key exchange, tangle weaving (already exist for private-groups) and managment APIs similar to the `MetaFeeds` interface.

## Unify `publish` APIs

It's cumbersome for application developers to know which publish API to use when. The first draft will have `sbot.Publish({ content })` for the root/main key (which might be a metafeed) and `sbot.MetaFeeds.Publish("@sub.ed25519", { content })` to publish from subfeeds.

It would be much better to not have to know about this stuff. A far out solution would be some kind of configuration to shard by type automatically and just have a single `Publish({content})` but it's not necessary to get started with this.

Additonally, there was a `sbot.PublishAs(nick, {content})` which enable [ssb-identites](https://github.com/ssbc/ssb-identities) like things, which should also be unified.

## support hosting the TBD example application

We still have to decide what we build but it _could_ be an interesting target, to make sure go-ssb can also host those (as a websocket server, similar to browser-core)

# Links
* https://ssb-ngi-pointer.github.io/
* https://github.com/ssb-ngi-pointer/ssb-secure-partial-replication-spec
* https://github.com/ssb-ngi-pointer/ssb-meta-feed-spec
* https://github.com/ssb-ngi-pointer/bendy-butt-spec
* https://github.com/ssb-ngi-pointer/ssb-subset-replication-spec