package ssb

import (
	"bytes"
	"fmt"
	"regexp"
	"unicode/utf8"
)

var (
	signatureRegexp = regexp.MustCompile(",\n  \"signature\": \"([A-Za-z0-9/+=.]+)\"")
	authorRegexp    = regexp.MustCompile(`"author": "(@[A-Za-z0-9/+=.]+.ed25519)"`)
)

func unicodeEscapeSome(s string) string {
	var b bytes.Buffer
	for i, r := range s {
		if r < 20 {
			// TODO: width for multibyte chars
			runeValue, _ := utf8.DecodeRuneInString(s[i:])
			fmt.Fprintf(&b, "\\u%04x", runeValue)
		} else {
			fmt.Fprintf(&b, "%c", r)
		}
	}
	return b.String()
}
