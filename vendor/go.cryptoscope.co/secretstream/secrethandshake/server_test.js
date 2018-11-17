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
var pull = require("pull-stream")
var toPull = require('stream-to-pull-stream')

function readKeyF(fname) {
    var tmpobj = JSON.parse(fs.readFileSync(fname).toString())
    return {
        'publicKey': new Buffer(tmpobj.publicKey, 'base64'),
        'secretKey': new Buffer(tmpobj.secretKey, 'base64'),
    }
}

var appKey = new Buffer('IhrX11txvFiVzm+NurzHLCqUUe3xZXkPfODnp7WlMpk=', 'base64')

var createBob = shs.createServer(readKeyF('key.bob.json'), function(pub, cb) {
    // decide whether to allow access to pub.
    cb(null, true)
}, appKey)

pull(
    toPull.source(process.stdin),
    createBob(function(err, stream) {
        if (err) throw err
        // ...
    }),
    toPull.sink(process.stdout)
)
