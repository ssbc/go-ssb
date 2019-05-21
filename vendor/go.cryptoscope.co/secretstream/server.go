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

package secretstream

import (
	"net"

	"go.cryptoscope.co/secretstream/boxstream"
	"go.cryptoscope.co/secretstream/secrethandshake"

	"github.com/pkg/errors"
	"go.cryptoscope.co/netwrap"
)

// Server can create net.Listeners
type Server struct {
	keyPair secrethandshake.EdKeyPair
	appKey  []byte
}

// NewServer returns a Server which uses the passed keyPair and appKey
func NewServer(keyPair secrethandshake.EdKeyPair, appKey []byte) (*Server, error) {
	return &Server{keyPair: keyPair, appKey: appKey}, nil
}

// ListenerWrapper returns a listener wrapper.
func (s *Server) ListenerWrapper() netwrap.ListenerWrapper {
	return netwrap.NewListenerWrapper(s.Addr(), s.ConnWrapper())
}

// ConnWrapper returns a connection wrapper.
func (s *Server) ConnWrapper() netwrap.ConnWrapper {
	return func(conn net.Conn) (net.Conn, error) {
		state, err := secrethandshake.NewServerState(s.appKey, s.keyPair)
		if err != nil {
			return nil, errors.Wrap(err, "error building server state")
		}

		err = secrethandshake.Server(state, conn)
		if err != nil {
			return nil, errors.Wrap(err, "error performing handshake")
		}

		enKey, enNonce := state.GetBoxstreamEncKeys()
		deKey, deNonce := state.GetBoxstreamDecKeys()

		remote := state.Remote()
		boxed := &Conn{
			ReadCloser:  boxstream.NewUnboxer(conn, &deNonce, &deKey),
			WriteCloser: boxstream.NewBoxer(conn, &enNonce, &enKey),
			conn:        conn,
			local:       s.keyPair.Public[:],
			remote:      remote[:],
		}

		return boxed, nil
	}
}

// Addr returns the shs-bs address of the server.
func (s *Server) Addr() net.Addr {
	return Addr{s.keyPair.Public[:]}
}
