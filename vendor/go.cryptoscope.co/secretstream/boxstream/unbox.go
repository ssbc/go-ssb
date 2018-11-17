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
	"errors"
	"io"

	"golang.org/x/crypto/nacl/secretbox"
)

// Unboxer decrypts everything that is read from it
type Unboxer struct {
	input  io.Reader
	output *io.PipeWriter
	secret *[32]byte
	nonce  *[24]byte
}

// NewUnboxer wraps the passed input Reader into an Unboxer
func NewUnboxer(input io.Reader, nonce *[24]byte, secret *[32]byte) io.Reader {
	pr, pw := io.Pipe()
	unboxer := &Unboxer{
		input:  input,
		output: pw,
		secret: secret,
		nonce:  nonce,
	}
	go unboxer.readerloop()
	return pr
}

func (u *Unboxer) readerloop() {
	hdrBox := make([]byte, HeaderLength)
	hdr := make([]byte, 0, HeaderLength-secretbox.Overhead)
	var ok bool
	for {
		hdr = hdr[:0]
		_, err := io.ReadFull(u.input, hdrBox)
		if err != nil {
			u.output.CloseWithError(err)
			return
		}

		hdr, ok = secretbox.Open(hdr, hdrBox, u.nonce, u.secret)
		if !ok {
			u.output.CloseWithError(errors.New("boxstream: error opening header box"))
			return
		}

		// zero header indicates termination
		if bytes.Equal(final[:], hdr) {
			u.output.Close()
			return
		}

		n := binary.BigEndian.Uint16(hdr[:2])

		buf := make([]byte, n+secretbox.Overhead)

		tag := hdr[2:] // len(tag) == seceretbox.Overhead

		copy(buf[:secretbox.Overhead], tag)

		_, err = io.ReadFull(u.input, buf[len(tag):])
		if err != nil {
			u.output.CloseWithError(err)
			return
		}

		out := make([]byte, 0, n)
		out, ok = secretbox.Open(out, buf, increment(u.nonce), u.secret)
		if !ok {
			u.output.CloseWithError(errors.New("boxstream: error opening body box"))
			return
		}

		_, err = io.Copy(u.output, bytes.NewReader(out))
		if err != nil {
			u.output.CloseWithError(err)
			return
		}
		increment(u.nonce)
	}
}
