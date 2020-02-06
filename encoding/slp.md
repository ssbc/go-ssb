# Shallow Length-prefixed encoding

Used for encoding of the `info` field in secret key derivations (see `DeriveSecret` function)

The key derivation should include all information of the context of the derivation. This includes information that is valid only for this particular message, as well as the intended use of the key.

Once we have agreed on a suitable selection of values to include, we also must encode them in such a way that it can not collide with the honest encoding of a different set of values.  Obviously, an adversary can perform a different encoding, but this does not matter, as we assume they can not trick the user into performing off-standard derivations or use an off-standard encoding for their derivations.  Implementations must make sure this assumption holds.

## Notation and Definitions

In this document, we write lists as comma seperated value, enclosed in parentheses. Therefore a list of values 1, 2 and 3 would be written as `(1, 2, 3)`.  Binary concatenation is `||`.

For the purpose of this document, **common byte order is defined as little endian**. (see [Code section][LE-code] for more detail)

## Requirements

We require the format to support lists of buffers. This is enough for most simple operations, and can be extended to key-value stores.
A property of many structured data encodings is schemalessness. To get a simpler encoding, we do not require this property. Therefore, derivation steps need to define a schema for the encoding of the info field. For the core derivations, this is given. Custom applications that are defined in extensions need to provide this as well.

 Note that schemalessness would not help in this context, because all users who derive a key agree, by following the specification, on what that key is supposed to be used for up front, so even in a schema

We require the format to be able to handle buffers that are at most 65535 bytes in size.

At this point, we do not require to be able to encode nested data. However, we will later propose two options for backwards-compatible changes to this proposal that support that.

## Format

The format is called _shallow length prefixed (SLP)_ encoding. Shallow, because it does not specify nesting. Length-prefixed, because each element is prefixed with its length.

Specifically, the encoding is the element-wise concatenation of the the length of the elements and the element. The length is a uint16 encoded using common byte order. The pseudocode of the algorithm can be found in the [Code section][SLP-code].

For example, the list
```
(a, b, c)
```
is encoded as:
```
encodeU16(len(a)) || a || encodeU16(len(b)) || b || encodeU16(len(c)) || c
```

## Ordered Key-Value Datasets

Ordered key-value datasets can be encoded by first constructing and then encoding a list from the dataset.  To construct the list, start with an empty list, iterate over the dataset, and then for each key-value pair in the dataset first add the key and then the value to the list.

**NOTE - `derive_secret` assumes known order of entries and just encodes values** (Example 2 below)

### Example 1 - encode keys and values

To derive the read key from the message key, we perform a derivation with the info argument of HKDF.Expand set to the encoding of the following key-value list, displayed here to look a bit like a JSON object:
```js
{
	"purpose": "box2",
	"feed": "@feedID", // this is a placeholder
	"prev": "%msgID",   // this is a placeholder
	"type": "read key"
}
```

This would be list-encoded by alternating between keys and values:
```
("purpose", "box2", "type", "read key", "feed", "@feedID", "prev", "%msgID")
```

Here the "schema" is the expected keys and their values, along with the order in which they're encoded.

The encoding of this list is
```
     p u r p o s e       b o x 2       f e e d       @ f e e d I D       p r e v       @ m s g I D       t y p e       r e a d   k e y
0700 707572706f7365 0400 626f7832 0400 66646463 0700 40666565644944 0400 70726576 0600 406d73674944 0400 74797065 0800 72656164206b6579
^^^^                ^^^^          ^^^^          ^^^^                ^^^^          ^^^^              ^^^^          ^^^^
length              length        length        length              length        length            length        length
```

### Example 2 - encode values (key implied by order)

Alternatively, we could also ditch the keys and just use the values. Using the same data from the previous example, we would get this list:
```
("box2", "@feedID", "%msgID", "read key")
```
That list encodes to
```
     b o x 2       @ f e e d I D       @ m s g I D       r e a d   k e y
0400 626f7832 0700 40666565644944 0600 406d73674944 0800 72656164206b6579
^^^^          ^^^^                ^^^^              ^^^^
length        length              length            length
```
Note that this is more in line with the idea of using schemas. For example, the schema for the box2 read key dictates that what follows is first a feed ID, and then a message ID. Also, in reality the feed and message IDs would be the binary encoding of the ID.


## Code

### SLP Encode

```
function encode(list List) Buffer {
	var out Buffer

	for elem in list {
		len = encodeU16(elem.length())
		out.append(len)
		out.append(elem)
	}

	return out
}
```

#### Go

```go
import "encoding/binary"

func binEncodeUint16(b []byte, i uint16) {
	binary.LittleEndian.PutUint16(b, i)
}

func Encode(list [][]byte) []byte {
	var (
		out []byte
		l  = make([]byte, 2)
	)

	for _, elem := range list {
		binEncodeUint16(l, uint16(len(elem)))
		out = append(out, l...)
		out = append(out, elem...)
	}

	return out
}
```

#### JavaScript

```js
function binEncodeUInt16(target, number) {
  target.writeUInt16LE(number, 0)
}

function encode (list) {
  var out = Buffer.alloc(0)

  for (let buf in list) {
    const length = Buffer.alloc(2)
	binEncodeUInt16(length, buf.length)

    out = Buffer.concat([out, length, buf])
  }

  return out
}
```
where
- `list` is an Array of Buffers
- `encode` returns a Buffer


### Little Endian Encode

In pseudocode, the type `uint16` has the method `encode`, that returns a string containing the common byte order encoding of the number:
```
method encode() of uint16 {
	var out [2]byte

	out[0] = byte(self & 0xff)
	out[1] = byte(self >> 8)

	return out
}
```

For convenience, the function `encodeU16` takes any positive integer as input, converts it to `uint16` and returns the result of `encode`:
```
method encodeU16(i int) {
	assert i >= 0
	return uint16(i).encode()
}
```

---

## Nested Data

Later applications may require nested data. Here, we propose two ways to achieve this feature in a backwards-compatible fashion. Again, we stress that due to the lack of schemalessness, new derivations can assign whatever meaning to the contents of the buffers. This does not limit the usefulness or security of the simpler, shallow encoding.

### Path-Style Keys

When using the lists to describe key-value sets, the keys can be chosen to represent paths. This requires defining a path delimiter (e.g. '.', '/' or '\0'), or using length-prefixed lists to separate path segments. Then, if two paths begin with the same sequence of path segments, they are in the same compound data structure. The compound data structure itself is named by the shared prefix.

### _Recursive Length Prefix (RLP)_ Encoding

RLP is similar to SLP, except that it allows list elements to themselves be lists. If that is the case, the element is first RLP-encoded itself, before the length is taken and it is concatenated.

Example: The list
```
(a, (b, c))
```
becomes:
```
encodeU16(len(a)) || a || encodeU16(4 + len(b) + len(c)) || encodeU16(b) || b || encodeU16(c) || c
```


[SLP-code]: #slp-encode
[LE-code]: #little-endian-encode
