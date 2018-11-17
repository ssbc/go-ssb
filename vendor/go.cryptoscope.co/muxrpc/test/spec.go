package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/muxrpc/codec"
)

type Errorer interface {
	Error(v ...interface{})
	Errorf(fmt string, v ...interface{})
}

func MultiErrorer(rs ...Errorer) Errorer {
	return multiErrorer{
		rs: rs,
	}
}

type multiErrorer struct {
	rs []Errorer
}

func (de multiErrorer) Error(v ...interface{}) {
	for _, r := range de.rs {
		r.Error(v...)
	}
}

func (de multiErrorer) Errorf(fmt string, v ...interface{}) {
	for _, r := range de.rs {
		r.Errorf(fmt, v...)
	}
}

func PacketSpecErrors(r Errorer, spec PacketSpec, pkt DirectedPacket) bool {
	var de DoesErrorer

	if r != nil {
		r = MultiErrorer(r, &de)
	} else {
		r = &de
	}

	spec(r, pkt)
	return bool(de)
}

func TranscriptSpecErrors(r Errorer, spec TranscriptSpec, ts *Transcript) bool {
	var de DoesErrorer
	if r != nil {
		r = MultiErrorer(r, &de)
	} else {
		r = &de
	}
	spec(r, ts)
	return bool(de)
}

func BodySpecErrors(r Errorer, spec BodySpec, body codec.Body) bool {
	var de DoesErrorer
	if r != nil {
		r = MultiErrorer(r, &de)
	} else {
		r = &de
	}
	spec(r, body)
	return bool(de)
}

type DoesErrorer bool

func (de *DoesErrorer) Error(...interface{})          { *de = true }
func (de *DoesErrorer) Errorf(string, ...interface{}) { *de = true }

type LineErrorer struct {
	r Errorer
}

func (lr LineErrorer) Error(v ...interface{}) {
	v = append(v, "line:", getLineString(1))
	lr.r.Error(v)
}

func (lr LineErrorer) Errorf(format string, v ...interface{}) {
	v = append(v, "line:", getLineString(1))
	format += " line: %s"
	lr.r.Errorf(format, v)
}

func ErrorerWithPrefix(r Errorer, prefix ...interface{}) Errorer {
	return prefixErrorer{
		prefix: prefix,
		r:      r,
	}
}

type prefixErrorer struct {
	prefix []interface{}
	r      Errorer
}

func (pe prefixErrorer) Error(v ...interface{}) {
	v_ := make([]interface{}, len(pe.prefix), len(pe.prefix)+len(v))
	copy(v_, pe.prefix)

	// this should be preallocated
	v_ = append(v_, v...)

	pe.r.Error(v_...)
}

func (pe prefixErrorer) Errorf(format string, v ...interface{}) {
	v_ := make([]interface{}, len(pe.prefix), len(pe.prefix)+len(v))
	copy(v_, pe.prefix)

	// this should be preallocated
	v_ = append(v_, v...)

	format = strings.Repeat("%v ", len(pe.prefix)) + format

	pe.r.Errorf(format, v_...)
}

type TranscriptSpec func(Errorer, *Transcript)

func MergeTranscriptSpec(specs ...TranscriptSpec) TranscriptSpec {
	return func(r Errorer, ts *Transcript) {
		for i, spec := range specs {
			r := ErrorerWithPrefix(r, "spec", i, "in transcript merge:")
			spec(r, ts)
		}
	}
}

func UniqueMatchTranscriptSpec(spec PacketSpec) TranscriptSpec {
	return func(r Errorer, ts *Transcript) {
		r = ErrorerWithPrefix(r, "no unique match:")
		spec := MatchCountTranscriptSpec(spec, 1)

		spec(r, ts)
	}
}

func MatchCountTranscriptSpec(spec PacketSpec, n int) TranscriptSpec {
	return func(r Errorer, ts *Transcript) {
		var is []int

		// make a new Errorer per iteration that only checks whether an error occurred
		// and only use that to count the non-erroring specs
		for i, pkt := range ts.Get() {
			if !PacketSpecErrors(nil, spec, pkt) {
				is = append(is, i)
			}
		}

		if len(is) != n {
			r.Errorf("expected packet match count %d, got %d (%v)", n, len(is), is)
		}
	}
}

func LengthTranscriptSpec(n int) TranscriptSpec {
	return func(r Errorer, ts *Transcript) {
		if m := len(ts.Get()); m != n {
			r.Errorf("expected %d packets, got %d", n, m)
		}
	}
}

func OrderTranscriptSpec(before, after PacketSpec) TranscriptSpec {
	return func(r Errorer, ts *Transcript) {
		var (
			j = -1
			k = -1
		)

		for i, pkt := range ts.Get() {
			if PacketSpecErrors(nil, before, pkt) {
				if j != -1 {
					r.Error("transcript order: first packet spec not unique")
				}

				j = i
			}

			if PacketSpecErrors(nil, after, pkt) {
				if k != -1 {
					r.Error("transcript order: second packet spec not unique")
				}

				k = i
			}
		}

		if j == -1 {
			r.Error("transcript order: first packet spec not matched")
		}

		if k == -1 {
			r.Error("transcript order: second packet spec not matched")
		}

		if k >= j {
			r.Error("transcript order: wrong order")
		}
	}
}

type PacketSpec func(Errorer, DirectedPacket)

func MergePacketSpec(specs ...PacketSpec) PacketSpec {
	return func(r Errorer, pkt DirectedPacket) {
		for i, spec := range specs {
			r := ErrorerWithPrefix(r, "spec", i, "in packet merge:")
			spec(r, pkt)
		}
	}
}

func DirPacketSpec(dir Direction) PacketSpec {
	return func(r Errorer, pkt DirectedPacket) {
		if pkt.Dir != dir {
			r.Error("packet direction: wrong direction")
		}
	}
}

func NoErrorPacketSpec() PacketSpec {
	return func(r Errorer, pkt DirectedPacket) {
		if pkt.Err != nil {
			r.Error("did not expect an error but got:", pkt.Err)
		}
	}
}

func ErrorPacketSpec(errStr string) PacketSpec {
	return func(r Errorer, pkt DirectedPacket) {
		if pkt.Err == nil {
			r.Error("expected an error but didn't get one")
			return
		}

		if pkt.Err.Error() != errStr {
			r.Error("expected error %q but got:", errStr, pkt.Err)
		}
	}
}

func ReqPacketSpec(req int32) PacketSpec {
	return func(r Errorer, pkt DirectedPacket) {
		if pkt.Packet == nil {
			r.Error("req packet: nil")
			return
		}

		if pkt.Req != req {
			r.Errorf("req packet: expected req %d, got %d", req, pkt.Req)
		}
	}
}

func FlagEqualPacketSpec(flag codec.Flag) PacketSpec {
	return func(r Errorer, pkt DirectedPacket) {
		if pkt.Packet == nil {
			r.Error("flag eq packet: nil")
			return
		}

		if pkt.Flag != flag {
			r.Errorf("flag eq packet: expected req %d, got %d", flag, pkt.Flag)
		}
	}
}

func FlagSetPacketSpec(flag codec.Flag, expected bool) PacketSpec {
	return func(r Errorer, pkt DirectedPacket) {
		if pkt.Packet == nil {
			r.Error("flag set packet: nil")
			return
		}

		if real := (pkt.Flag&flag != 0); real != expected {
			r.Errorf("flag set packet: expected flag %s to be %v, got %v", flag, expected, real)
		}
	}
}

func CallPacketSpec(stream bool, flagJSON bool, method muxrpc.Method, tipe muxrpc.CallType, args ...interface{}) PacketSpec {
	if args == nil {
		args = []interface{}{}
	}

	callBody, err := json.Marshal(muxrpc.Request{
		Method: method,
		Type:   tipe,
		Args:   args,
	})

	if err != nil {
		panic("could not marshal request body for transcript validation")
	}

	return MergePacketSpec(
		FlagSetPacketSpec(codec.FlagStream, stream),
		FlagSetPacketSpec(codec.FlagJSON, flagJSON),
		BodyPacketSpec(EqualBodySpec(codec.Body(callBody))),
	)
}

func BodyPacketSpec(spec BodySpec) PacketSpec {
	return func(r Errorer, pkt DirectedPacket) {
		if pkt.Packet == nil {
			r.Error("body packet spec: nil")
			return
		}

		spec(r, pkt.Body)
	}
}

type BodySpec func(Errorer, codec.Body)

func MergeBodySpec(specs ...BodySpec) BodySpec {
	return func(r Errorer, body codec.Body) {
		for _, spec := range specs {
			spec(r, body)
		}
	}
}

func EqualBodySpec(exp codec.Body) BodySpec {
	return func(r Errorer, body codec.Body) {
		if !bytes.Equal(exp, body) {
			r.Errorf("body eq mismatch: expected %q but got %q", exp, body)
		}
	}
}

func ContainsBodySpec(exp codec.Body) BodySpec {
	return func(r Errorer, body codec.Body) {
		if !bytes.Contains(body, exp) {
			r.Errorf("body contains: expected that %q in in %q", exp, body)
		}
	}
}

func getLineString(n int) string {
	pc, fn, line, _ := runtime.Caller(n + 1)
	return fmt.Sprintf("%s[%s:%d]", runtime.FuncForPC(pc).Name(), fn, line)
}
