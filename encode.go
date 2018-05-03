package ssb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

func formatArray(depth int, b *bytes.Buffer, dec *json.Decoder) error {
	for {
		t, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errors.Wrap(err, "message Encode: unexpected error from Token()")
		}
		switch v := t.(type) {

		case json.Delim: // [ ] { }
			switch v {
			case ']':
				fmt.Fprint(b, strings.Repeat("  ", depth-1))
				fmt.Fprint(b, "]")
				if dec.More() {
					fmt.Fprint(b, ",")
				}
				fmt.Fprintf(b, "\n")
				return nil
			case '{':
				fmt.Fprint(b, strings.Repeat("  ", depth))
				fmt.Fprint(b, "{\n")
				if err := formatObject(depth+1, b, dec); err != nil {
					return errors.Wrapf(err, "formatArray(%d): decend failed", depth)
				}
			case '[':
				fmt.Fprint(b, "[\n")
				if err := formatArray(depth+1, b, dec); err != nil {
					return errors.Wrapf(err, "formatArray(%d): decend failed", depth)
				}
			default:
				return errors.Errorf("formatArray(%d): unexpected token: %v", depth, v)
			}

		case string:
			fmt.Fprint(b, strings.Repeat("  ", depth))
			fmt.Fprintf(b, "%q", v)
			if dec.More() {
				fmt.Fprintf(b, ",")
			}
			fmt.Fprintf(b, "\n")

		case float64:
			fmt.Fprint(b, strings.Repeat("  ", depth))
			fmt.Fprintf(b, "%v", v)
			if dec.More() {
				fmt.Fprintf(b, ",")
			}
			fmt.Fprintf(b, "\n")

		default:
			fmt.Fprint(b, strings.Repeat("  ", depth))
			if v == nil {
				fmt.Fprint(b, "null")
			} else {
				fmt.Fprintf(b, "%v", v)
			}
			if dec.More() {
				fmt.Fprintf(b, ",")
			}
			fmt.Fprintf(b, "\n")
		}

	}
	return nil
}

func formatObject(depth int, b *bytes.Buffer, dec *json.Decoder) error {
	var isKey = true // key:value pair toggle
	for {
		t, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errors.Wrap(err, "message Encode: unexpected error from Token()")
		}
		switch v := t.(type) {

		case json.Delim: // [ ] { }
			switch v {
			case '}':
				fmt.Fprint(b, strings.Repeat("  ", depth-1))
				fmt.Fprint(b, "}")
				if dec.More() {
					fmt.Fprint(b, ",")
				}
				fmt.Fprintf(b, "\n")
				return nil
			case '{':
				fmt.Fprint(b, "{")
				var d = depth + 1
				if dec.More() {
					fmt.Fprint(b, "\n")
				} else {
					// empty object. no spaces between { and }
					// hint this to the next recurision by setting d=1
					// which will use depth-1
					d = 1
				}
				if err := formatObject(d, b, dec); err != nil {
					return errors.Wrapf(err, "formatObject(%d):decend failed", depth)
				}
				isKey = true
			case '[':
				fmt.Fprint(b, "[")
				var d = depth + 1
				if dec.More() {
					fmt.Fprint(b, "\n")
				} else {
					// empty array. no spaces between [ and ]
					// hint this to the next recurision by setting d=1
					// which will use depth-1
					d = 1
				}
				if err := formatArray(d, b, dec); err != nil {
					return errors.Wrapf(err, "formatObject(%d):decend failed", depth)
				}
				isKey = true
			default:
				return errors.Errorf("formatObject(%d): unexpected token: %v", depth, v)
			}

		case string:
			if isKey {
				fmt.Fprintf(b, "%s%q: ", strings.Repeat("  ", depth), v)
			} else {
				var buf bytes.Buffer
				fmt.Fprintf(&buf, "%q", v)
				// replace escaped unicode charachters
				// this is a Go thing, done by %q
				// but it also helps us to keep \n in the string
				out := unicodeRegexp.ReplaceAllFunc(buf.Bytes(), unicodeReplace)
				io.Copy(b, bytes.NewReader(out))
				if dec.More() {
					fmt.Fprint(b, ",")
				}
				fmt.Fprintf(b, "\n")
			}
			isKey = !isKey

		case float64:
			fmt.Fprint(b, strconv.FormatFloat(v, 'f', -1, 64))
			if dec.More() {
				fmt.Fprintf(b, ",")
			}
			fmt.Fprintf(b, "\n")
			isKey = !isKey

		default:
			if v == nil {
				fmt.Fprint(b, "null")
			} else {
				fmt.Fprintf(b, "%v", v)
			}
			if dec.More() {
				fmt.Fprintf(b, ",")
			}
			fmt.Fprintf(b, "\n")
			isKey = !isKey
		}
	}
	return nil
}

func EncodePreserveOrder(b []byte) ([]byte, Signature, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	var buf bytes.Buffer
	t, err := dec.Token()
	if err != nil {
		return nil, "", errors.Wrap(err, "message Encode: expected {")
	}
	if v, ok := t.(json.Delim); !ok || v != '{' {
		return nil, "", errors.Wrapf(err, "message Encode: wanted { got %v", t)
	}
	fmt.Fprint(&buf, "{\n")
	if err := formatObject(1, &buf, dec); err != nil {
		return nil, "", errors.Wrap(err, "message Encode: failed to format message as object")
	}
	formatted := buf.Bytes()
	matches := signatureRegexp.FindSubmatch(formatted)
	if n := len(matches); n != 2 {
		return nil, "", errors.Errorf("message Encode: expected signature in formatted bytes. Only %d matches", n)
	}
	sig := Signature(matches[1])
	out := signatureRegexp.ReplaceAll(formatted, []byte{})
	return bytes.Trim(out, "\n"), sig, nil
}
