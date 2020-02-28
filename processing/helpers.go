package processing

import (
	"encoding/binary"
)

func encodeUint16(out []byte, i uint16) []byte {
	off := len(out)
	out = append(out, 0, 0)

	binary.BigEndian.PutUint16(out[off:], i)
	return out
}

func decodeUint16(buf []byte) uint16 {
	return binary.BigEndian.Uint16(buf)
}

func encodeString(out []byte, str string) []byte {
	out = encodeUint16(out, uint16(len(str)))
	off := len(out)
	out = append(out, make([]byte, len(str))...)

	// for := range would iterate over glyphs!
	for i := 0; i < len(str); i++ {
		out[off+i] = str[i]
	}

	return out
}

func encodeStrings(out []byte, strings ...string) []byte {
	out = encodeUint16(out, uint16(len(strings)))

	for _, elem := range strings {
		out = encodeString(out, elem)
	}

	return out
}

func encodeAppendStrings(base []byte, strings ...string) []byte {
	if len(base) == 0 {
		return encodeStrings(base, strings...)
	}

	// update length
	l := decodeUint16(base) + uint16(len(strings))
	encodeUint16(base[:0], l)

	// append new elements
	for _, elem := range strings {
		base = encodeString(base, elem)
	}

	return base
}
