# Encoding of the `info` Field in Derivations

The key derivation should include all information of the context
of the derivation.
This includes information that is valid only for this particular message,
as well as the intended use of the key.

Once we have agreed on a suitable selection of values to include, we
also must encode them in such a way that it can not collide with
the honest encoding of a different set of values.
Obviously, an adversary can perform a different encoding, but this
does not matter, as we assume they can not trick the user into performing
off-standard derivations or use an off-standard encoding for their derivations.
Implementations must make sure this assumption holds.

## Notation

In this document, we write lists as comma seperated value, enclosed
in parentheses. Therefore a list of values 1, 2 and 3 would be written
as `(1, 2, 3)`.
Binary concatenation is `||`.

## Requirements

We require the format to support lists of buffers.
This is enough for most simple operations, and can be extended to
key-value stores.

To get a simpler encoding, we do not require schemalessness.
Therefore, derivation steps need to define a schema for the encoding
of the info field.
For the core derivations, this is given.
Custom applications need to provide this as well.
Note that schemalessness would not help in this context, because 
all users that are derive a key are on the same page what that key
is supposed to be used for.
If they are not, the adversary got them to execute their code already,
which means that particular user is in trouble anyways (REPHRASE).

We require the format to be able to handle buffers that are less than 2<sup>16</sup>B in size.

At this point, we do not require to be able to encode nested data.
However, we will later propose two options for backwards-compatible
changes to this proposal that support that.

## Format

The proposed format is called _shallow length prefixed (SLP)_ encoding.
Shallow, because it does not specify nesting.
Length-prefixed, because each element is prefixed with its length.

Specifically, the encoding is the element-wise concatenation of the the
length of the elements and the element.
The length is encoded as big endian uint16. (REALLY BIG ENDIAN? PRO/CON?)
The pseudocode of the algorithm can be found in section [Code].

For example, the list
```
(a, b, c)
```
is encoded as:
```
uint16BE(len(a)) || a || uint16BE(len(b)) || b || uint16BE(len(c)) || c
```

## Nested Data

Later applications may require nested data.
Here, we propose two ways to achieve this feature in a backwards-compatible
fashion.
Again, we stress that due to the lack of schemalessness, new derivations
can assign whatever meaning to the contents of the buffers.
This does not limit the usefulness or security of the simpler, shallow
encoding.

### Path-Style Keys

When using the lists to describe key-value sets, the keys can be chosen
to represent paths.
This requires defining a path delimiter (e.g. '.', '/' or '\0'), or using
length-prefixed lists to separate path segments.
Then, if two paths begin with the same sequence of path segments, they
are in the same compound data structure.
The compound data structure itself is named by the shared prefix.

### _Recursive Length Prefix (RLP)_ Encoding

RLP is similar to SLP, except that it allows list elements to themselves
be lists.
If that is the case, the element is first RLP-encoded itself, before the
length is taken and it is concatenated.

Example: The list
```
(a, (b, c))
```
becomes:
```
uint16BE(len(a)) || uint16BE(4 + len(b) + len(c)) || uint16BE(b) || b || uint16BE(c) || c
```

<a href name=Code>&nsbp;</a>
## Code

```
function encode(l List) Buffer {
	var out Buffer

	for elem in l {
		len = elem.
			length().
			toUInt16().
			toBigEndian()
		out.append(len)
		out.append(elem)
	}

	return out
}
```

and in Go (untested):

```
import "encoding/binary"

func Encode(l [][]byte) []byte {
	var (
		out []byte
		l  = make([]byte, 2)
	)
	
	for _, elem := range l {
		binary.BigEndian.PutUint16(l, uint16(len(elem)))
		append(out, l...)
		append(out, elem...)
	}

	return out
}
```

[Code]: #code
