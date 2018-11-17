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
	"bytes"
	"encoding/binary"
	"io"

	"github.com/pkg/errors"
)

type Writer struct{ w io.Writer }

// NewWriter creates a new packet-stream writer
func NewWriter(w io.Writer) *Writer { return &Writer{w} }

// WritePacket creates an header for the Packet and writes it and the body to the underlying writer
func (w *Writer) WritePacket(r *Packet) error {
	var hdr Header
	hdr.Flag = r.Flag
	hdr.Len = uint32(len(r.Body))
	hdr.Req = r.Req
	if err := binary.Write(w.w, binary.BigEndian, hdr); err != nil {
		return errors.Wrapf(err, "pkt-codec: header write failed")
	}
	if _, err := io.Copy(w.w, bytes.NewReader(r.Body)); err != nil {
		return errors.Wrapf(err, "pkt-codec: body write failed")
	}
	return nil
}

// Close sends 9 zero bytes and also closes it's underlying writer if it is also an io.Closer
func (w *Writer) Close() error {
	_, err := w.w.Write([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0})
	if err != nil {
		return errors.Wrapf(err, "pkt-codec: failed to write Close() packet")
	}
	if c, ok := w.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}
