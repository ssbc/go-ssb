/*
This file is part of go-muxrpc.

go-muxrpc is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

go-muxrpc is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with go-muxrpc.  If not, see <http://www.gnu.org/licenses/>.
*/

package codec

import (
	"fmt"
	"strings"
)

type Body []byte

func (b Body) String() string {
	return fmt.Sprintf("%s", []byte(b))
}

// Packet is the decoded high-level representation
type Packet struct {
	Flag Flag
	Req  int32
	Body Body
}

// Flag is the first byte of the Header
type Flag byte

func (f Flag) Set(g Flag) Flag {
	return f | g
}

func (f Flag) Clear(g Flag) Flag {
	return f & ^g
}

func (f Flag) Get(g Flag) bool {
	return f&g == g
}

func (f Flag) String() string {
	var flags []string

	if f.Get(FlagString) {
		flags = append(flags, "FlagString")
	}
	if f.Get(FlagJSON) {
		flags = append(flags, "FlagJSON")
	}
	if f.Get(FlagStream) {
		flags = append(flags, "FlagStream")
	}
	if f.Get(FlagEndErr) {
		flags = append(flags, "FlagEndErr")
	}

	return "{" + strings.Join(flags, ", ") + "}"
}

// Flag bitmasks
const (
	FlagString Flag = 1 << iota // type
	FlagJSON                    // bits
	FlagEndErr
	FlagStream
)

// Header is the wire representation of a packet header
type Header struct {
	Flag Flag
	Len  uint32
	Req  int32
}
