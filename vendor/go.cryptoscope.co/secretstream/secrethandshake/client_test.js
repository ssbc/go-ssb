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

var shs = require('secret-handshake')
var fs = require('fs')
var pull = require('pull-stream')
var toPull = require('stream-to-pull-stream')


function readKeyF(fname) {
    var tmpobj = JSON.parse(fs.readFileSync(fname).toString())
    return {
        'publicKey': new Buffer(tmpobj.publicKey, 'base64'),
        'secretKey': new Buffer(tmpobj.secretKey, 'base64'),
    }
}

var alice = readKeyF('key.alice.json')
var bob = readKeyF('key.bob.json')

var createClient = shs.createClient(alice, new Buffer('IhrX11txvFiVzm+NurzHLCqUUe3xZXkPfODnp7WlMpk=', 'base64'))

pull(
    toPull.source(process.stdin),
    createClient(bob.publicKey, function() { }),
    toPull.sink(process.stdout)
)
