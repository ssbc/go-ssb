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
				fmt.Fprint(b, "{\n")
				if err := formatObject(depth+1, b, dec); err != nil {
					return errors.Wrapf(err, "formatObject(%d):decend failed", depth)
				}
				isKey = true
			case '[':
				fmt.Fprint(b, "[\n")
				if err := formatArray(depth+1, b, dec); err != nil {
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
				fmt.Fprintf(b, "%q", v)
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

func EncodePreserveOrder(b []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	var buf bytes.Buffer
	t, err := dec.Token()
	if err != nil {
		return nil, errors.Wrap(err, "message Encode: expected {")
	}

	if v, ok := t.(json.Delim); !ok || v != '{' {
		return nil, errors.Wrapf(err, "message Encode: wanted { got %v", t)
	}

	fmt.Fprint(&buf, "{\n")
	if err := formatObject(1, &buf, dec); err != nil {
		return nil, errors.Wrap(err, "message Encode: failed to format message as object")
	}
	return bytes.Trim(buf.Bytes(), "\n"), nil
}
