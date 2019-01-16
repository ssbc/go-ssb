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
	"encoding/binary"
	"io"
	"os"

	"github.com/pkg/errors"
)

type Reader struct{ r io.Reader }

func NewReader(r io.Reader) *Reader { return &Reader{r} }

// ReadPacket decodes the header from the underlying writer, and reads as many bytes as specified in it
// TODO: pass in packet pointer as arg to reduce allocations
func (r *Reader) ReadPacket() (*Packet, error) {
	var hdr Header
	err := binary.Read(r.r, binary.BigEndian, &hdr)
	// TODO does os.ErrClosed belong here?!
	if e := errors.Cause(err); e == os.ErrClosed || e == io.EOF || e == io.ErrClosedPipe {
		return nil, io.EOF
	} else if err != nil {
		return nil, errors.Wrapf(err, "pkt-codec: header read failed")
	}

	// detect EOF pkt. TODO: not sure how to do this nicer
	if hdr.Flag == 0 && hdr.Len == 0 && hdr.Req == 0 {
		return nil, io.EOF
	}

	// copy header info
	var p = Packet{
		Flag: hdr.Flag,
		Req:  hdr.Req,
		Body: make([]byte, hdr.Len),
	}

	_, err = io.ReadFull(r.r, p.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "pkt-codec: read body failed. Packet: %+v", p)
	}

	return &p, nil
}
