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

// reades packets from stdin to test the writer implementation
var pull = require('pull-stream')
var psc = require('packet-stream-codec')
var toPull = require('stream-to-pull-stream')

var want = [
  {'req': 0,'stream': false,'end': false,'value': ['event', {'okay': true}],'length': 23,'type': 2},
  {'req': 1,'stream': false,'end': false,'value': 'whatever','length': 8,'type': 1},
  {'req': 2,'stream': true,'end': false,'value': {'type': 'Buffer','data': [104, 101, 108, 108, 111]},'length': 5,'type': 0},
  {'req': -2,'stream': true,'end': false,'value': {'type': 'Buffer','data': [103, 111, 111, 100, 98, 121, 101]},'length': 7,'type': 0},
  {'req': -3,'stream': false,'end': true,'value': {'message': 'intentional','name': 'Error'},'length': 40,'type': 2},
  {'req': 2,'stream': true,'end': true,'value': true,'length': 4,'type': 2},
  {'req': -2,'stream': true,'end': true,'value': true,'length': 4,'type': 2},
  'GOODBYE'
]

pull(
  toPull.source(process.stdin),
  psc.decode(),
  pull.collect(function (err, got) {
    if (err) throw err
    var n = got.length
    if (n != want.length) {
      throw Error('test data length missmatch. got:' + n + ' wanted:' + want.length)
    }
    for (var idx = 0; idx < n; idx++) {
      var gotObj = got[idx]
      var wantObj = want[idx]
      if (JSON.stringify(gotObj) != JSON.stringify(wantObj)) {
        throw Error('test data divergent. idx[' + idx + ']' + JSON.stringify(gotObj))
      }
    }
  })
)
