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

/*
Package codec implements readers and writers for https://github.com/dominictarr/packet-stream-codec

Packet structure:

	(
		[flags (1byte), length (4 bytes, UInt32BE), req (4 bytes, Int32BE)] # Header
		[body (length bytes)]
	) *
	[zeros (9 bytes)]

Flags:

	[ignored (4 bits), stream (1 bit), end/err (1 bit), type (2 bits)]
	type = {0 => Buffer, 1 => String, 2 => JSON} # PacketType
*/
package codec
