# Scuttlebutt Private Groups Documentation

Documentation for how Scuttlebutt will work and why

For a long time people wanted different sorts of private groups and we are now trying to make them possible. Yay!

We have explored a number of designs, which have different strengths and weaknesses. To make the best decision we try to describe accurately what the properties of each approach are. Because there are many ways people want to communicate, we'll probably end up needing more than one method.

[TOC]

## Key Store

Even on the simpler schemes, quite a bit of state needs to be kept track of. Although ssb is a database, to enable forward security,
it is necessary to _not_ store keys directly in database views,
but in a separate key store, which can be deleted.

That means to migrate an identity to another ssb implementation, the key store must be exported and imported. Therefore, the key store export format will need to be well specified. However, at this point, we leave that as an implementation detail, and will return to this once the message formats are decided.

Another thing that needs to be tracked is whether or not a key has been learned through a forward-secure channel. If so, it must not be leaked through a non-forward-secure channel. Thus, implementations either only use forward-secure channels or keep track of how they learned a key.

- different group types need their own section in the key store
- may mean own file or subdir
- we should specify an interchange format so we can take keys to a different implementation
- body keys/read caps for messages is easy

## Entrusting Keys

A fundamental building block is _entrusting capabilities_ (also known as keys). In a capability system, access tokens are called "capabilities" because "possessing the capability" means you can do the thing, and do not need furthur permissions.

Although in the capability system literature they use the term "delegation" to describe granting a capability, we prefer the term "entrusting" because it reminds the reader that you really do have to trust what that person does with the key. 

The most basic message read capability is just the body key for a message and the message id, so that you know what message to decrypt with it.

This is already a feature of ssb, with [private-box](http://github.com/auditdrivencrypto/private-box) messages. The format is just an ordinary message id, followed by a standard query string, with the message body key (base64 encoded) as the "unbox" query parameter `%{sha256(msg).toString('base64')}.sha256?unbox={key.toString('base64')` when parsed, it also appears in the mentions array of an ssb message as `{link: msg_id, query: {unbox: key }}`

There is not currently a specification for a binary version of this format.

There is not currently a defined way to perform bulk entrustments, but this will be needed for the "symmetric group" and "hash ratchet" schemes.

## Group Key Schedule

### Problem Space

- different kinds of groups:
  - community:
    - relatively open
    - many people
    - a little trust
    - shared interest
  - team:
    - rather closed
    - not too big
    - strong trust
    - maybe working together on something

The more people you tell a secret, the less secret it is. It seems this is reflected in the problem of private communication, because it is much easier to design a crypto system with good properties for a small group than it is for a big group.

strong privacy is difficult for communities, because they are very large. If the whole community knows a secret, any one of them may leak it. However, it is still very desirable to have some sort of "privacy"
if only to be able to keep any random strangers out. It's good to think about this as a cryptographic equivalent of a insider lingo or culture that outsiders are not familiar with. Wether you feel you can automatically trust someone simply because they are part of a community says more about the particular community than communities in general.

On the other hand, a smaller group, like a "team" or "family" can be quite different. Here people know more about each other, have a longer relationship, and have much more trust. We can also afford more bytes and more complicated encryption for smaller groups.

<!--
strong trust in teams, and strong privacy achievable

- should group members need to agree on who is in group?
  - yes:
    - may allow having a single key for group, very efficient
  - no:
    - let users have agency who they encrypt to
  - if no, then schedule needs to allow users to keep agency
-->

### Solution Space

We have defined a solution space as one of three answers to two questions.

firstly, what approach to key rotation?

* (s) static (do not rotate keys)
* (h) hash-ratchet (rotate a symmetric key forward after every message)
* (d) double-ratchet (combine asymmetric and symmetric keys a la signal)

secondly, how do keys relate to users and groups?

* (uu) per-user-per-user (aka pairwise, each user has a key for any other user they communicate with)
* (ug) per-user-per-group  (each user has a key for each group they communicate with)
* (g) per-group (the group has a key, and every member shares that key)


metrics/properties to discuss:

- add: messages size when adding s/one to group
- rm: complexity to remove someone
- post: message overhead when posting to group
- pfs: forwards security
- pcs: post-compromise-security
- "easy"
- design & impl complexity (not quantifiable)

-> Based on these metrics/properties, which are the most interesting?


### table: possible group encryption designs

|     name        |  system  |  add   |   rm    |   post   | pfs | pcs | "easy" |
|-----------------|----------|--------|---------|----------|-----|-----|--------|
|                 |   s-uu   |  O(n)  |  O(n-2) |  O(n-1)  |  0  |  0  |   1    |
|                 |   s-gu   |  O(n)  |  O(n-2) |  O(n-1)  |  0  |  0  |   1    |
| symmetric group |   s-g    |  O(1)  |  O(n-2) |  O(1)    |  0  |  0  |   1    |
|                 |   h-uu   |  O(n)  |  O(n-2) |  O(n-1)  |  1  |  0  |   1    |
| sender key      |   h-gu   |  O(n)  |  O(n-2) |  O(1)    |  1  |  0  |   1    |
|                 |   h-g    |  O(1)  |  O(n-2) |  O(1)    |  1  |  0  |   0    |
| double ratchet  |   d-uu   |  O(n)  |  O(n-2) |  O(n-1)  |  1  |  1  |   1    |
|                 |   d-gu   |  O(n)  |  O(n-2) |  O(n-1)  |  1  |  1  |   0    |
|                 |   d-g    |  O(1)  |  O(n-2) |  O(1)    |  1  |  1  |   0    |

> possible group encryption designs, and bandwidth complexity to post messages, add, and remove members. 
  
Some positions are not attainable
within the scuttlebutt architecture, due to interactions between hard problems in cryptography and hard problems in distributed systems. For example, it would be great if it
was possible to remove a peer with less than `O(n)` time, such as `O(log(n))`,
but we did not come up with a way to do this. Another desirable goal would be a double ratchet with a single key per group. This would support very large groups. A single double ratchet between two peers is easy enough, because messages from a single peer are always received in a predictable order. When there are many peers, they can receive messages interleaved in many different orders. Allowing for this is one of the things that makes scuttlebutt work well, but this greatly complicates encryption.

Observations:

- overhead for per-group keys
  - add:  O(1)
  - post: O(1)
  - -> very desirable!
- removing someone is equally painful everywhere (from a factor point of view)
  (_maybe_ it's possible to do an O(log(n)) remove?)
- security: static < hash-ratchet < double-ratchet
- combining ratchet + single group key not possible due to async/concurrency issues
- beyond this model, thus interesting:
  - MLS-style key trees (logarithmic in n)
    - really difficult, works only with a server that stictly orders messages
  - Per-Group Hash Ratchet
    - let key schedule graph resemble tange graph??
      - not single key but avg case sublinear, depends on tip count
	  - unlikely to grow very large??
	  - too complex for now

### "symmetric group" a group defined by a single static key

This is very easy to implement but has only minimal security properties. Anyone who has the key to the group is a member of the group, and any member can add additional members (by entrusting the key to them)

symmetric groups have the best scaling properties. Sending a message to even a very large group is the same overhead as sending another a message to a single recipient in another scheme. Adding a new member to a group is also efficient - you simply send them the key. The group is not forward secure, but if the key is entrusted to you via a forward secure mechanism (such as double ratchet) then the key can be "forgotten" by deleting the key to the message that entrusted you the group key. So you can leave _the whole group_ but not forget a particular message.

### "hash ratchet" a key for each group member

each member in the group encrypts their messages to a different
key, and after each message, hashes the key, and uses that as the next key. Peers should immediately forget the hashed key, but remember the next key. If they also store the body key, they can look at the message again, and if they forget this the message is gone. Thus this system can have forward secrecy.

However, to join a group, a new member must be informed of every other member's current ratchet state. This takes O(N) of the number of group members. Also, the hash ratchet is not forward secret unless it's first entrusted within a forward secret message. On it's own, you cannot use hash ratchets to send a message to a particular peer, so it must be used in conjunction with double ratchet (or another forward secret mechanism)

Because hash ratchet is not useful on it's own, but still quite complicated, obviously if we do eventually implement hash ratchet, we should obviously implement double ratchet first.

### "double ratchet" Pairwise Double-Ratchet

medium-term, good for smaller groups ("teams"), and private messages between individual peers.

Double ratchet is the same basic design as used in signal and other modern messaging protocols. Each peer must set a diffie-helman key for each other peer they wish to communicate with. When ever they have received a new message from another peer, they derive a new shared key, but if they post another message before that peer has responded, they advance the key with a hash ratchet.

in all following messages, there also is a group-wide tangle



      ```js
      "content": {
        "type":    "group/invite",
        "groupid": "groupid=.group",
        "users":   [ ... ]
      }
        ```
- @bob initiates missing double ratchets
    - maybe this can happen before all the ratchets are available
      - bob could later entrust this message to the rest
      - alice shouldn't do it because maybe bob deliberately
        didn't encrypt to (one of) the missing people

There can still be a governance mechanism for membership management before sending these messages. Also, users who don't agree with adding or removing people can still encrypt to the same set of users as before. This leaves agency at the individuals, which is especially important if we consider that some members may block others, or someone adds a new member even though the group decided they didn't want to have them in the group.

### Per-Group Hash-Ratchet

really difficult, deferred

very interesting, but deferred

good for very large groups

## Application-Level Group Types

We need to be able to refer to groups on the application layer, and to be able to tell whether a message we receive belongs to the group it claims to belong to or not. In this sense, the application-layer group type cares about group _identity_ and _controlling write access_.

We identified two helpful kind of application layer groups: Teams and Communities. They implement identity the same way, but the way they determine whether or not a message belongs to the group is different.

A team is a set of users, and everyone knows about it. A message belongs to a team group if it claims it belongs into it and the author is member of the team.

A community does not have a rigid membership model. While we suggest that users still let the rest of the community know that they added someone, the group would still work if the didn't. To let someone join the community, all they need is the group secret.

The group state is defined by two kinds of documents: the group document itself and the membership document. Both their roots are determined by the group id, but with metadata describing their purpose. The root of the group document is `"group:<groupid in base64>"`, while the root of the membership document is `"group-membership:<groupid in base64>"`.

Each group is a document. By publishing a message in the group, the group state is extended by that message, similar to how a post message extends the state of a thread. Thus, all group messages are tangled together. This means (a) all messages reference the group id, and are easily identifiable and (b) messages from a group can be requested through tanglesync.

Each group also has a membership document. This document maintains a set of users that are members of the group. This set can be modified by members, by adding people to or removing them from the set. For communities, the membership may be purely informational, for teams, we suggest to render messages authored by non-members differently or not at all.

Rendering these messages differently is a precaution to prevent the situation where someone learns a teams group id (which is not necessarily public) and sends a message to the team, and it looks like they are on the team. Or even worse, they _guess_ whether a message was in the team (maybe they know when the team has a call and during that they exchange messages), and then respond to that message. If it is rendered like other messages, inside a regular thread, it would seem like they are taking part in the conversation, even though they are not and just sneaked in a message into a private thread of other users.

Note that these considerations are mostly interesting to _private_ teams. You could also do public teams, and relax this a bit here - but we remind application designers that asking people not to respond has been social practice for a while now, maybe teams could be a step in that direction as well.


Groups that make sense:

|            | public | symmetric group | pairwise double ratchet |
|------------|--------|-----------------|-------------------------|
| community  |    ✓   |       ✓         |                         |
| team       |    ✓   |                 |            ✓            |

&nbsp;

Note that public communities are very similar to channels, in the sense that everyone can read and write to them. The main difference is that a public community would have a founding message that posts to the community would refer to, instead of referencing it by name. That way it's possible to have multiple communities with the same name.

### Mutating the Membership Document

The membership document is mutated in both teams and communities, but in communities it is *informational*, while in teams it's *authoritative*.

First, a user (say, @alice) creates a group.

```js
{
  "type": "group/new",
  "members": ["@alice"],
  "group-type": "team" OR "community",
  "key": "rootkey=.key" // only if it's a private group
}
```

Let's say publishing this message results in message hash `%group-new`.

If the group is public, the key field is not set. It is only used in private groups such that an identifier is used that doesn't link the group id to the group creator. Since this is not possible in public groups, we just leave it and use the `%group-new` as group id directly.

In our case however, @alice derives the group id from the key and the hash:
```
group_id = SecretDerive(key, {
    "purpose": "group-id",
    "group-new-hash": "%group-new"
  }, 256)
```

Let's say this returns base64 `group_id=`.

Messages that change the membership set will be weaved into the tangle. For example, let @alice add @bob:

```
{
  "type": "group/add",
  "user": "@bob",
  
  "tangles": {
    "group-membership": {
      "root": "group-membership:group_id=",
      "branches": [ "%group-new" ]
    }
  }
}
```

Let's say the hash of this message is `%group-add-bob`.

Also, @alice entrusts @bob with the read capabilities to access all previous messages that changes the group membership document.

Since @alice was alone in the group, nobody did concurrent changes to the set, so the only branch is the group creation message. If this is a private group, @alice needs to entrust @bob with the group's creation message and all messages that changed the group state. This is so @bob can verify he's actually in the group @alice claims they invited them to. If @bob successfully verifies that, they post a message to the group, acknowloging that they are now part of the group:

```
{
  "type": "group/accept",
  
  "tangles": {
    "group-membership": {
      "root": "group-membership:group_id=",
      "branches": [ "%group-add-bob" ]
    }
  }
}
```

Which results in message hash `%group-bob-accept`

Let @alice and @bob chat for a while in the group. These messages will not reference the tangle, because they do not modify the membership state.

Then, @bob wants to remove @alice from the group:

```
{
  "type": "group/rm",
  "user": "@alice"
  "reason": "left project"
  
  "tangles": {
    "group-membership": {
      "root": "group-membership:group_id=",
      "branches": [ "%group-bob-accept" ]
    }
  }
}
```

### Mutating the Group Document

The group document is just a collection of all messages that are authored to group. Having a group document makes it possible to use tanglesync to pull in updates to the group.
    
## Tangle Sync

%G6FVdfLPIxROrJdrHCU5UnwN+29o8ZJsYaC3FiNsct8=.sha256

## Message Encryption Boxes (`.box2`)

Here we describe a multi recipient encryption format that can be used for multiple encryption schemes.

Boxes are encrypted to zero\* or more symmetric keys. Unlike the previous design, [private-box](http://github.com/auditdrivencrypto/private-box) we do not need to do any asymmetric operations when attempting to decrypt. This makes decryption much faster, especially since in ssb you attempt (and fail) to decrypt many more messages that are not for you, than messages that are for you. Asymmetric operations are still used in the double ratchet scheme, but only _after_ a message is successfully decrypted.

Also, we made some effort to make the overhead as minimal as reasonable. This is mainly for reasons of metadata privacy.
An earlier draft of the double ratchet scheme had 81 bytes per recipient overhead on each message. We worried that this would make the number of recipients a lot more obvious, which could make it easier to correlate messages in a group. This is particularily important in ssb, as it is a decentralized system and many peers see the message cyphertext.

\* that's not a typo. A message can be encrypted to zero keys, by encrypting the message body without recipients. If the message body key is stored, it can be entrusted via another message.

### Format Proposal

box2:

```
+---------------------------------+
|           header_box            |
+---------------------------------+
|      msg_key xor recp_key_0     |
+---------------------------------+
|      msg_key xor recp_key_1     |
+---------------------------------+
|               ...               |
+---------------------------------+
|      msg_key xor recp_key_n     |
+---------------------------------+
|         [ extensions ]          |
+---------------------------------+
|            body_box             |
+---------------------------------+
```

where

header_box:

```
+----------------+-----------------+
|   mac_tag/16   |    header/16    |
+----------------+-----------------+
```
header:

```
+--------+-----+-------------------+
| offs/2 | f/1 |    hdr_ext/13     |
+--------+-----+-------------------+
```

- offs: offset of body in bytes 
- f: extension flags

Flags:
- FHasEphemerals indicates there are ephemeral keys
  - allocates one byte `n_eph` in `hdr_ext`
  - `n_eph` stores how many ephemerals there are
  - `n_eph` is xored with key derived from `msg_key` (`n_eph_pad`)
  - allocates `(n_eph+1) * 32` byte in the extensions for `ext_eph_box`

Extensions:
- Padding
  - Can be used to conceil
    - true body size to outsiders
    - number of ephs from holders of read cap
  - We can just put any amount of padding at the beginning of the
    extensions area, without specifying it's length anywhere.
  - Since we provide an offset to the start of the body, that can
    be decrypted by anyone with the body key.
  - And since the length of `ext_eph_box` is determined by n_eph,
	  users who can decrypt that know where to start looking.
- Ephemeral DH Shares
  - Encrypted list of ephemeral keys
  - Byte length is `(n_eph+1) * 32`

If using pairwise double ratchet, ephemeral keys may be sent.

ext_eph_box:
```
+----------------+-----------------+
|   mac_tag/16   | eph_header/16   |
+----------------+-----------------+
|                                  |
|          ext_eph_keys            |
|                                  |
+----------------------------------+
```
note: the mac_tag is over both `eph_header` and `ext_eph_keys`

eph_header, only used by double ratchet:
```
+----------------+----------------+
|   bitfield/8   |   reserved/8   |
+----------------+----------------+

bf = popcount(bitfield)
```
the bitfield marks which recipients get a new ephemeral key.
`bf` is the number of recipients - the number of 1s in the bitfield.


ext_eph_keys:
```
+----------------+----------------+
|            eph_0/32             |
+---------------------------------+
|            eph_1/32             |
+---------------------------------+
|               ...               |
+---------------------------------+
|         eph_{|bf|-1}/32         |
+---------------------------------+
```


body_box:
```
+----------------+-----------------+
|   mac_tag/16   |                 |
+----------------+                 |
|                                  |
|               body               |
|                                  |
+----------------------------------+
```

Helper Definition and Key Schedule:

Note that what looks like json objects here just describes the provided data, not the encoding. We need to use something like a sane signing format here.
```
DeriveSecret(Secret, Label, Length) =
            HKDF-Expand(Secret, {
              "purpose": "box2",
              "label": Label,
              TODO more context?
            }, Length)

msg_key
 |
 +---> DeriveSecret(., "read_cap", 256)
 |     = msg_read_cap
 |       |
 |       +---> DeriveSecret(., "header", 256)
 |       |     = hdr_key
 |       |
 |       +---> DeriveSecret(., "body", 256)
 |             = body_key
 |               |
 |               +---> DeriveSecret(., "box", 256)
 |               |
 |               +---> DeriveSecret(., "box_nonce", 192)
 |
 +---> DeriveSecret(., "ext", 256)
         |
		 +---> DeriveSecret(., "eph", 256)
		         |
				 +---> DeriveSecret(., "n_eph_pad", 8)
				 |
				 +---> DeriveSecret(., "box", 256)
				 |
				 +---> DeriveSecret(., "box_nonce", 192)
```

`msg_key` is the symmetric key that is encrypted to each recipient or group.
When entrusting the message, instead of sharing the `msg_key` instead the `msg_read_cap` is shared.
this gives access to header metadata and body but not ephemeral keys.

---

clarifications on format
* padding should come after ephemerals
* ephemerals will be left out entirely if header[3] = 0
* FHasEphemerals = 1
* if(header[3] & FHasEphemerals) then n_eph = header[4] (first byte in hdr_ext)
* all numbers are little endian
* does the header have a nonce?
* the bitfield in the eph_header is /8 ... so 64 bits. that means format can do double ratchet with up to 64 other devices max.
* box and box_nonce is derived from body_key... this is unnecessary step.
