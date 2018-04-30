package ssb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/pkg/errors"
)

func EncodeSimple(i interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	err := enc.Encode(i)
	if err != nil {
		return nil, err
	}
	return bytes.Trim(buf.Bytes(), "\n"), nil
}

func EncodePreserveOrder(b []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	var buf bytes.Buffer
	t, err := dec.Token()
	if err != nil {
		return nil, errors.Wrap(err, "message Encode: expected {")
	}

	if t.(json.Delim) != '{' {
		return nil, errors.Wrapf(err, "message Encode: wanted { got %v", t)
	}

	fmt.Fprintf(&buf, "{\n")

	var depth = 1
	var isKey = true
	var isObject = true
	for {
		t, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.Wrap(err, "message Encode: unexpected error from Token()")
		}
		switch v := t.(type) {
		case json.Delim: // [ ] { }
			switch v {
			case '[':
				//isArray = true // TODO: nesting...
				depth++
			case '{':
				isObject = true
				isKey = true
				depth++
			case ']':
				fallthrough
			case '}':
				depth--
			}
			fmt.Fprintf(&buf, "%s\n%s", v, strings.Repeat("  ", depth))
		case string:
			if isObject {
				if isKey {
					fmt.Fprintf(&buf, "%q: ", v)
				} else {
					fmt.Fprintf(&buf, "%q", v)
					if dec.More() {
						fmt.Fprintf(&buf, ",")
					}
					fmt.Fprintf(&buf, "\n%s", strings.Repeat("  ", depth))
				}
				isKey = !isKey
			} else {
				fmt.Fprintf(&buf, "%q", v)
			}
		default:
			if isObject && !isKey {
				fmt.Fprintf(&buf, "%v", v)
				if dec.More() {
					fmt.Fprintf(&buf, ",")
				}
				fmt.Fprintf(&buf, "\n%s", strings.Repeat("  ", depth))
				isKey = !isKey
			} else {
				fmt.Fprintf(&buf, `%v`, v)
			}
		}
	}
	return buf.Bytes(), nil
}
