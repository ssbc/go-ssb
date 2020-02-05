# Encoding for `feed_id` and `prev_msg_id`

The binary encoding of ids is defined as the concatenation of:
- the `code` for the type of id, as a UInt8
- the `value` of the id

## Codes

 code | format       | type |
:----:|:------------:|:----:|
 0    | ed25519      | key  |
 1    | sha256       | feed |
 2    | sha256.gabby | feed |

## Examples

In scuttlebutt message ids are encoded like:

```
  %R8heq/tQoxEIPkWf0Kxn1nCm/CsxG2CDpUYnAvdbXY8=.sha256
  |└─────────────────────┬────────────────────┘ └─┬──┘
 sigil         base64 encoded value            feed format
```

- `sigil` here is used to encode the context this is used in (`%` = this is a message)
- `feed format` here is the hashing algorithm used to derive this message id

Here we would encode this in a Buffer as:

```
  01 47 c8 5e ab fb 50 a3 11 08 3e 45 9f d0 ac 67 d6 70 a6 fc 2b 31 1b 60 83 a5 46 27 02 f7 5b 5d 8f
  |  └────────────────────┬────────────────────────────────────────────────────────────────────────┘
 code              hex encoded value
```


