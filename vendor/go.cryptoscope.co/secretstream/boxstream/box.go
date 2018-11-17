/*
This file is part of secretstream.

secretstream is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

secretstream is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with secretstream.  If not, see <http://www.gnu.org/licenses/>.
*/

package boxstream

import (
	"bytes"
	"encoding/binary"
	"io"
	"log"

	"golang.org/x/crypto/nacl/secretbox"
)

const (
	// HeaderLength defines the length of the header packet before the body
	HeaderLength = 2 + 16 + 16

	// MaxSegmentSize is the maximum body size for boxstream packets
	MaxSegmentSize = 4 * 1024
)

var final [18]byte

// Boxer encrypts everything that is written to it
type Boxer struct {
	input  *io.PipeReader
	output io.Writer
	secret *[32]byte
	nonce  *[24]byte
}

// NewBoxer returns a Boxer wich encrypts everything that is written to the passed writer
func NewBoxer(w io.Writer, nonce *[24]byte, secret *[32]byte) io.WriteCloser {
	pr, pw := io.Pipe()
	b := &Boxer{
		input:  pr,
		output: w,
		secret: secret,
		nonce:  nonce,
	}
	go b.loop()
	return pw
}

func increment(b *[24]byte) *[24]byte {
	var i int
	for i = len(b) - 1; i >= 0 && b[i] == 0xff; i-- {
		b[i] = 0
	}

	if i < 0 {
		return b
	}

	b[i] = b[i] + 1

	return b
}

func (b *Boxer) loop() {
	var running = true
	var eof = false
	var nonce1, nonce2 [24]byte

	check := func(err error) {
		if err != nil {
			running = false
			if err2 := b.input.CloseWithError(err); err2 != nil {
				log.Print("Boxer/CloseWithErr: ", err)
			}
		}
	}

	// prepare nonces
	copy(nonce1[:], b.nonce[:])
	copy(nonce2[:], b.nonce[:])

	for running {

		msg := make([]byte, MaxSegmentSize)
		n, err := io.ReadAtLeast(b.input, msg, 1)
		if err == io.EOF {
			eof = true
			running = false
		} else {
			check(err)
		}
		msg = msg[:n]

		// buffer for box of current
		boxed := secretbox.Seal(nil, msg, increment(&nonce2), b.secret)
		// define and populate header
		var hdrPlain = bytes.NewBuffer(nil)
		err = binary.Write(hdrPlain, binary.BigEndian, uint16(len(msg)))
		check(err)

		// slice mac from box
		_, err = hdrPlain.Write(boxed[:16]) // ???
		check(err)

		if eof {
			hdrPlain.Reset()
			hdrPlain.Write(final[:])
		}
		hdrBox := secretbox.Seal(nil, hdrPlain.Bytes(), &nonce1, b.secret)

		increment(increment(&nonce1))
		increment(&nonce2)

		_, err = io.Copy(b.output, bytes.NewReader(hdrBox))
		check(err)

		_, err = io.Copy(b.output, bytes.NewReader(boxed[secretbox.Overhead:]))
		check(err)
	}
}
