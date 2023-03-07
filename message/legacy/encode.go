// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package legacy

import (
	"bytes"
	"errors"
	"fmt"
	jsoniter "github.com/json-iterator/go"
	"strings"
)

var iteratorConfig = jsoniter.Config{
	// re float encoding: https://spec.scuttlebutt.nz/datamodel.html#signing-encoding-floats
	// not particular excited to implement all of the above
	// this keeps the original value as a string
	UseNumber: true,
}.Froze()

func (pp *prettyPrinter) formatObject(depth int) error {
	if pp.iter.WhatIsNext() != jsoniter.ObjectValue {
		return errors.New("next value is not an object")
	}

	pp.buffer.WriteString("{\n")

	i := 0
	if cb := pp.iter.ReadObjectCB(func(iter *jsoniter.Iterator, s string) bool {
		if depth == 1 {
			pp.topLevelFields = append(pp.topLevelFields, s)
		}

		if i > 0 {
			pp.buffer.WriteString(",\n")
		}

		pp.buffer.WriteString(strings.Repeat("  ", depth))
		pp.writeString(s)
		pp.buffer.WriteString(": ")

		switch whatIsNext := pp.iter.WhatIsNext(); whatIsNext {
		case jsoniter.ObjectValue:
			if err := pp.formatObject(depth + 1); err != nil {
				iter.Error = err
				return false
			}

		case jsoniter.StringValue:
			if err := pp.formatString(); err != nil {
				iter.Error = err
				return false
			}

		case jsoniter.NumberValue:
			if err := pp.formatNumber(); err != nil {
				iter.Error = err
				return false
			}

		case jsoniter.NilValue:
			if err := pp.formatNil(); err != nil {
				iter.Error = err
				return false
			}

		case jsoniter.BoolValue:
			if err := pp.formatBool(); err != nil {
				iter.Error = err
				return false
			}

		case jsoniter.ArrayValue:
			if err := pp.formatArray(depth + 1); err != nil {
				iter.Error = err
				return false
			}

		default:
			iter.Error = fmt.Errorf("unexpected value: %v", whatIsNext)
			return false
		}

		i++
		return true
	}); !cb {
		return pp.iter.Error
	}

	pp.buffer.WriteString("\n")
	pp.buffer.WriteString(strings.Repeat("  ", depth-1))
	pp.buffer.WriteString("}")

	return nil
}

func (pp *prettyPrinter) formatArray(depth int) error {
	if pp.iter.WhatIsNext() != jsoniter.ArrayValue {
		return errors.New("next value is not an array")
	}

	pp.buffer.WriteString("[")
	i := 0
	for {
		ok := pp.iter.ReadArray()
		if !ok {
			break
		}

		if i == 0 {
			pp.buffer.WriteString("\n")
		}

		if i > 0 {
			pp.buffer.WriteString(",\n")
		}

		pp.buffer.WriteString(strings.Repeat("  ", depth))

		switch whatIsNext := pp.iter.WhatIsNext(); whatIsNext {
		case jsoniter.ObjectValue:
			if err := pp.formatObject(depth + 1); err != nil {
				return err
			}

		case jsoniter.StringValue:
			if err := pp.formatString(); err != nil {
				return err
			}

		case jsoniter.NumberValue:
			if err := pp.formatNumber(); err != nil {
				return err
			}

		case jsoniter.NilValue:
			if err := pp.formatNil(); err != nil {
				return err
			}

		case jsoniter.BoolValue:
			if err := pp.formatBool(); err != nil {
				return err
			}

		case jsoniter.ArrayValue:
			if err := pp.formatArray(depth + 1); err != nil {
				return err
			}

		default:
			return fmt.Errorf("unexpected value: %v", whatIsNext)
		}

		i++
	}

	if i > 0 {
		pp.buffer.WriteString("\n")
		pp.buffer.WriteString(strings.Repeat("  ", depth-1))
	}
	pp.buffer.WriteString("]")

	return nil
}

func (pp *prettyPrinter) formatString() error {
	if pp.iter.WhatIsNext() != jsoniter.StringValue {
		return errors.New("next value is not a string")
	}

	v := pp.iter.ReadString()

	pp.writeString(v)
	return nil
}

func (pp *prettyPrinter) formatNumber() error {
	if pp.iter.WhatIsNext() != jsoniter.NumberValue {
		return errors.New("next value is not a number")
	}

	v := pp.iter.ReadNumber()

	pp.buffer.WriteString(v.String())
	return nil
}

func (pp *prettyPrinter) formatNil() error {
	if pp.iter.WhatIsNext() != jsoniter.NilValue {
		return errors.New("next value is not a nil")
	}

	pp.iter.Skip()

	pp.buffer.WriteString("null")
	return nil
}

func (pp *prettyPrinter) formatBool() error {
	if pp.iter.WhatIsNext() != jsoniter.BoolValue {
		return errors.New("next value is not a bool")
	}

	v := pp.iter.ReadBool()

	if v {
		pp.buffer.WriteString("true")
	} else {
		pp.buffer.WriteString("false")
	}
	return nil
}

func (pp *prettyPrinter) writeString(v string) {
	pp.buffer.WriteByte('"')
	quoteString(pp.buffer, v)
	pp.buffer.WriteByte('"')
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

var (
	acceptedFieldOrderList = [][]string{
		{"previous", "author", "sequence", "timestamp", "hash", "content", "signature"},
		{"previous", "sequence", "author", "timestamp", "hash", "content", "signature"},
	}
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
		} else {
			if fieldListLen != sliceLen {
				panic("inconsistent length of acceptedFieldOrderList")
			}
		}
	}
}

func checkFieldOrder(fields []string) error {
	for _, acceptedFieldOrder := range acceptedFieldOrderList {
		if len(fields) != len(acceptedFieldOrder) {
			continue
		}

		for i := range acceptedFieldOrder {
			if acceptedFieldOrder[i] != fields[i] {
				continue
			}
		}

		return nil
	}

	return fmt.Errorf("ssb/verify: invalid field order: %v", fields)
}

type prettyPrinter struct {
	iter *jsoniter.Iterator

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

	pp.iter = jsoniter.ParseBytes(iteratorConfig, input)

	for _, o := range opts {
		o(&pp)
	}

	if pp.buffer == nil {
		guessedSize := len(input)
		pp.buffer = bytes.NewBuffer(make([]byte, 0, guessedSize))
	}

	if err := pp.formatObject(1); err != nil {
		return nil, fmt.Errorf("message Encode: failed to format message as object: %w", err)
	}

	if err := pp.iter.Error; err != nil {
		return nil, fmt.Errorf("message Encode: iterator error: %w", err)
	}

	if pp.checkFieldOrder {
		if err := checkFieldOrder(pp.topLevelFields); err != nil {
			return nil, err
		}
	}

	return bytes.Trim(pp.buffer.Bytes(), "\n"), nil
}
