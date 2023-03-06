// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package legacy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func (pp *prettyPrinter) formatArray(depth int) error {

	b := pp.buffer
	dec := pp.decoder

	for {
		t, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("message Encode: unexpected error from Token(): %w", err)
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
				if err := pp.formatObject(depth + 1); err != nil {
					return fmt.Errorf("formatArray(%d): decend failed: %w", depth, err)
				}
			case '[':
				fmt.Fprint(b, strings.Repeat("  ", depth))
				fmt.Fprint(b, "[\n")
				if err := pp.formatArray(depth + 1); err != nil {
					return fmt.Errorf("formatArray(%d): decend failed: %w", depth, err)
				}
			default:
				return fmt.Errorf("formatArray(%d): unexpected token: %v", depth, v)
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
			b.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
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
}

var replacer = strings.NewReplacer("\\", `\\`, "\t", `\t`, "\n", `\n`, "\r", `\r`, `"`, `\"`)

func (pp *prettyPrinter) formatObject(depth int) error {
	var isKey = true // key:value pair toggle

	b := pp.buffer
	dec := pp.decoder

	for {
		t, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("message Encode: unexpected error from Token(): %w", err)
		}
		switch v := t.(type) {

		case json.Delim: // [ ] { }
			switch v {
			case '}':
				b.WriteString(strings.Repeat("  ", depth-1))
				b.WriteString("}")
				if dec.More() {
					b.WriteString(",")
				}
				b.WriteString("\n")
				return nil
			case '{':
				b.WriteString("{")
				var d = depth + 1
				if dec.More() {
					b.WriteString("\n")
				} else {
					// empty object. no spaces between { and }
					// hint this to the next recurision by setting d=1
					// which will use depth-1
					d = 1
				}
				if err := pp.formatObject(d); err != nil {
					return fmt.Errorf("formatObject(%d): decend failed: %w", depth, err)
				}
				isKey = true
			case '[':
				b.WriteString("[")
				var d = depth + 1
				if dec.More() {
					b.WriteString("\n")
				} else {
					// empty array. no spaces between [ and ]
					// hint this to the next recurision by setting d=1
					// which will use depth-1
					d = 1
				}
				if err := pp.formatArray(d); err != nil {
					return fmt.Errorf("formatObject(%d): decend failed: %w", depth, err)
				}
				isKey = true
			default:
				return fmt.Errorf("formatObject(%d): unexpected token: %v", depth, v)
			}

		case string:
			if isKey {
				if depth == 1 {
					pp.topLevelFields = append(pp.topLevelFields, v)
				}
				b.WriteString(strings.Repeat("  ", depth))
				fmt.Fprintf(b, "%q: ", v)
			} else {
				b.WriteByte('"')
				b.WriteString(unicodeEscapeSome(replacer.Replace(v)))
				b.WriteByte('"')
				if dec.More() {
					b.WriteString(",")
				}
				b.WriteString("\n")
			}
			isKey = !isKey

		case float64:
			b.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
			if dec.More() {
				b.WriteString(",")
			}
			b.WriteString("\n")
			isKey = !isKey

		default:
			if v == nil {
				b.WriteString("null")
			} else {
				fmt.Fprintf(b, "%v", v)
			}
			if dec.More() {
				b.WriteString(",")
			}
			b.WriteString("\n")
			isKey = !isKey
		}
	}
}

type PrettyPrinterOption func(pp *prettyPrinter)

func WithBuffer(buf *bytes.Buffer) PrettyPrinterOption {
	return func(pp *prettyPrinter) {
		pp.buffer = buf
	}
}

// WithStrictOrderChecking enables verification of the field names in the first level of the object
func WithStrictOrderChecking(yes bool) PrettyPrinterOption {
	return func(pp *prettyPrinter) {
		pp.checkFieldOrder = yes
	}
}

// some constants for field order checking
const acceptedFieldOrderDelimiter = ":"

// (slices can't be const, though)
var (
	acceptedFieldOrderList = [][]string{
		{"previous", "author", "sequence", "timestamp", "hash", "content", "signature"},
		{"previous", "sequence", "author", "timestamp", "hash", "content", "signature"},
	}

	acceptedFieldOrderLength int

	acceptedFieldOrders = make([]string, len(acceptedFieldOrderList))
)

// init the strings.Joined version of acceptedFieldOrderList for checkFieldOrder()
// also assert that all values in acceptedFieldOrderList have the same length
func init() {
	fieldListLen := -1
	for i, order := range acceptedFieldOrderList {

		// length assertion
		sliceLen := len(order)
		if i == 0 {
			fieldListLen = sliceLen
			acceptedFieldOrderLength = sliceLen
		} else {
			if fieldListLen != sliceLen {
				panic("inconsistent length of acceptedFieldOrderList")
				// TODO: change checkFieldOrder length check
			}
		}

		acceptedFieldOrders[i] = strings.Join(order, acceptedFieldOrderDelimiter)
	}
}

func checkFieldOrder(fields []string) error {
	if n := len(fields); n != acceptedFieldOrderLength {
		return fmt.Errorf("ssb/verify: invalid field order length (%d)", n)
	}

	gotFields := strings.Join(fields, acceptedFieldOrderDelimiter)

	for _, accepted := range acceptedFieldOrders {
		if accepted == gotFields {
			return nil
		}
	}

	return fmt.Errorf("ssb/verify: invalid field order: %v", fields)
}

type prettyPrinter struct {
	decoder *json.Decoder

	buffer *bytes.Buffer

	checkFieldOrder bool
	topLevelFields  []string
}

// PrettyPrinter formats and indents byte slice b using json.Token izer
// using two spaces like this to mimics JSON.stringify(....)
//
//	{
//	  "field": "val",
//	  "arr": [
//		"foo",
//		"bar"
//	  ],
//	  "obj": {}
//	}
//
// while preserving the order in which the keys appear
func PrettyPrint(input []byte, opts ...PrettyPrinterOption) ([]byte, error) {
	var pp prettyPrinter

	pp.decoder = json.NewDecoder(bytes.NewReader(input))

	// re float encoding: https://spec.scuttlebutt.nz/datamodel.html#signing-encoding-floats
	// not particular excited to implement all of the above
	// this keeps the original value as a string
	pp.decoder.UseNumber()

	for _, o := range opts {
		o(&pp)
	}

	if pp.buffer == nil {
		pp.buffer = new(bytes.Buffer)
	}

	// start encoding
	t, err := pp.decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("message Encode: expected {: %w", err)
	}
	if v, ok := t.(json.Delim); !ok || v != '{' {
		return nil, fmt.Errorf("message Encode: wanted { got %v: %w", t, err)
	}
	pp.buffer.WriteString("{\n")
	if err := pp.formatObject(1); err != nil {
		return nil, fmt.Errorf("message Encode: failed to format message as object: %w", err)
	}

	if pp.checkFieldOrder {
		if err := checkFieldOrder(pp.topLevelFields); err != nil {
			return nil, err
		}
	}

	return bytes.Trim(pp.buffer.Bytes(), "\n"), nil
}
