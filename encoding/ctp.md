# Cipherlink type-prefixed encoding

Used for encoding message and feed ids

The binary encoding of ids is defined as the concatenation of:
- the `type` of thing the cipherlink is referencing, as a UInt8
- the `format` which describes how the `key` was derived from the thing referenced, as a UInt8
- the `key` for the cypherlink

Note that "format" may close over how the type was encoded AND how that encoding was
mapped into a particular key

## Scuttlebutt Codes

 type code | referencing
-----------|-------------
 0         | a feed
 1         | a message
 2         | a blob

 format code | format      | specification
-------------|-------------|-----------------
 0           | "classic"   | ...see JS stack
 1           | gabby grove | ...see Go implementation
 2           | hidden      | ...coming soon


## Example: A "classic" scuttlebutt msgId

In scuttlebutt classic message ids are encoded :

```
  %R8heq/tQoxEIPkWf0Kxn1nCm/CsxG2CDpUYnAvdbXY8=.sha256
  │└─────────────────────┬────────────────────┘└───┬──┘
 sigil           base64 encoded key             suffix
```

- `sigil` here is used to encode the context this is used in (`%` = this is a message)
- `suffix` here encodes a couple of things:
  - the `key` of the cipherlink was derived using a sha256 hash of the message value, encoded using base64 to a String
  - this is a "classic" message (the nested JSON style). This is _implicit_

Here we would encode this in a Buffer (represented in hex) as:

```
  01 00  47 c8 5e ab fb 50 a3 11 08 3e 45 9f d0 ac 67 d6 70 a6 fc 2b 31 1b 60 83 a5 46 27 02 f7 5b 5d 8f
   │  │  └────────────────────┬────────────────────────────────────────────────────────────────────────┘
type  │                hex encoded key
     format 
```

Where:
- `type` of thing we're referencing is "a message", so code 1, which is `01` in hex
- `format` (encoding of message + method of derivation) which produced this key was "classic", which has code `00` in hex

## Example: A hidden scuttlebutt group

So as not to reveal who started a private group in scuttlebutt, we don't use the starting message
of a group as the public id.

Instead we use a new "hidden" style derivation
(a cryptographic combination of the group's symmetric key and the initial message)


```
  01 02  55 d4 f0 ab fb 50 a3 11 08 3e 45 9f d0 ac 67 d6 70 a6 fc 2b 31 1b 60 83 45 c8 b0 02 91 4c 3c 7e
   │  │  └────────────────────┬────────────────────────────────────────────────────────────────────────┘
type  │                hex encoded key
     format 
```


