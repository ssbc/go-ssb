// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package legacy

import (
	"bytes"
	"errors"
	"fmt"
	"unicode/utf8"

	"golang.org/x/text/encoding/unicode"
)

var hex = "0123456789abcdef"

// https://262.ecma-international.org/6.0/#sec-quotejsonstring
func quoteString(buf *bytes.Buffer, s string) {
	start := 0
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			if safeSet[b] {
				i++
				continue
			}

			if start < i {
				buf.WriteString(s[start:i])
			}

			buf.WriteByte('\\')

			switch b {
			case '\\', '"':
				buf.WriteByte(b)
			case '\n':
				buf.WriteByte('n')
			case '\r':
				buf.WriteByte('r')
			case '\t':
				buf.WriteByte('t')
			case '\b':
				buf.WriteByte('b')
			case '\f':
				buf.WriteByte('f')
			default:
				buf.WriteString(`u00`)
				buf.WriteByte(hex[b>>4])
				buf.WriteByte(hex[b&0xF])
			}

			i++
			start = i
			continue
		}

		c, size := utf8.DecodeRuneInString(s[i:])
		if c == utf8.RuneError && size == 1 {
			if start < i {
				buf.WriteString(s[start:i])
			}
			buf.WriteString(`\ufffd`)
			i += size
			start = i
			continue
		}

		i += size
	}
	if start < len(s) {
		buf.WriteString(s[start:])
	}
}

var utf16enc = unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewEncoder()

// InternalV8Binary does some funky v8 magic
// new Buffer(in, "binary") returns soemthing like (u16 && 0xff)
func InternalV8Binary(in []byte) ([]byte, error) {
	guessedLength := len(in) * 2
	u16b := make([]byte, guessedLength)

	nDst, nSrc, err := utf16enc.Transform(u16b, in, false)
	if err != nil {
		return nil, fmt.Errorf("internalV8bin: error transforming: %w", err)
	}
	if nSrc != len(in) {
		return nil, errors.New("internalV8bin: processed a different number of bytes than were given")
	}
	u16b = u16b[:nDst]

	// now drop every 2nd byte
	if len(u16b)%2 != 0 {
		return nil, errors.New("internalV8bin: assumed even number of bytes in u16")
	}
	for i := 0; i < len(u16b)/2; i++ {
		u16b[i] = u16b[i*2]
	}
	u16b = u16b[:len(u16b)/2]

	return u16b, nil
}

var safeSet = [utf8.RuneSelf]bool{
	' ':      true,
	'!':      true,
	'"':      false,
	'#':      true,
	'$':      true,
	'%':      true,
	'&':      true,
	'\'':     true,
	'(':      true,
	')':      true,
	'*':      true,
	'+':      true,
	',':      true,
	'-':      true,
	'.':      true,
	'/':      true,
	'0':      true,
	'1':      true,
	'2':      true,
	'3':      true,
	'4':      true,
	'5':      true,
	'6':      true,
	'7':      true,
	'8':      true,
	'9':      true,
	':':      true,
	';':      true,
	'<':      true,
	'=':      true,
	'>':      true,
	'?':      true,
	'@':      true,
	'A':      true,
	'B':      true,
	'C':      true,
	'D':      true,
	'E':      true,
	'F':      true,
	'G':      true,
	'H':      true,
	'I':      true,
	'J':      true,
	'K':      true,
	'L':      true,
	'M':      true,
	'N':      true,
	'O':      true,
	'P':      true,
	'Q':      true,
	'R':      true,
	'S':      true,
	'T':      true,
	'U':      true,
	'V':      true,
	'W':      true,
	'X':      true,
	'Y':      true,
	'Z':      true,
	'[':      true,
	'\\':     false,
	']':      true,
	'^':      true,
	'_':      true,
	'`':      true,
	'a':      true,
	'b':      true,
	'c':      true,
	'd':      true,
	'e':      true,
	'f':      true,
	'g':      true,
	'h':      true,
	'i':      true,
	'j':      true,
	'k':      true,
	'l':      true,
	'm':      true,
	'n':      true,
	'o':      true,
	'p':      true,
	'q':      true,
	'r':      true,
	's':      true,
	't':      true,
	'u':      true,
	'v':      true,
	'w':      true,
	'x':      true,
	'y':      true,
	'z':      true,
	'{':      true,
	'|':      true,
	'}':      true,
	'~':      true,
	'\u007f': true,
}
