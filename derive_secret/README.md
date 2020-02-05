# Derive Secret

Aa function for deriving new secrets from an initial secret.
It's defined as:

```js
function Derive (key, label, length) {
  var info = [feed_id, prev_msg_id, label]
  return HKDF.Expand(key, encode(info), length)
}
```

- `feed_id` and `prev_msg_id` are encoded in with [standard binary format](../encoding/ctp.md)
- `HKDF.Expand` is a hmac-like function which is specifically designed to generate random buffers of a given length.
  - we specify `sha256` for hashing in HKDF-Expand 
  - example of a node.js implementation : [futoin-hkdf](https://www.npmjs.com/package/futoin-hkdf#hkdfexpandhash-hash_len-prk-length-info-%E2%87%92-buffer)
- `encode` is a [shallow lenth-prefixed (SLP) encoding](../encoding/slp.md) of an ordered list

## Design

We want to derive unique keys which are very unlikely to collide with other keys, where "unlikely" means:
- won't happen by chance
- won't be easy to trick a user to performing a particular derivation

By baking the context (`feed_id` and `prev_msg_id`) into the `info` used to derive new secrets,
we make the derive function very specific.

This has the side-effect that we can use zero'd nonces for encryption, because we can be very sure
that the same key will not be generated twice.

