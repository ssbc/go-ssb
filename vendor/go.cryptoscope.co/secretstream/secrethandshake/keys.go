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

package secrethandshake

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"

	"github.com/pkg/errors"
)

func LoadSSBKeyPair(fname string) (*EdKeyPair, error) {
	f, err := os.Open(fname)
	if err != nil {
		return nil, errors.Wrapf(err, "secrethandshake: could not open key file")
	}
	defer f.Close()

	var sbotKey struct {
		Curve   string `json:"curve"`
		ID      string `json:"id"`
		Private string `json:"private"`
		Public  string `json:"public"`
	}

	if err := json.NewDecoder(f).Decode(&sbotKey); err != nil {
		return nil, errors.Wrapf(err, "secrethandshake: json decoding of %q failed.", fname)
	}

	public, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(sbotKey.Public, ".ed25519"))
	if err != nil {
		return nil, errors.Wrapf(err, "secrethandshake: base64 decode of public part failed.")
	}

	private, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(sbotKey.Private, ".ed25519"))
	if err != nil {
		return nil, errors.Wrapf(err, "secrethandshake: base64 decode of private part failed.")
	}

	var kp EdKeyPair
	copy(kp.Public[:], public)
	copy(kp.Secret[:], private)
	return &kp, nil
}
