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

// writes some packets to stdout to test the reader implementation
var pull = require('pull-stream')
var psc = require('packet-stream-codec')
var toPull = require('stream-to-pull-stream')
var split = require('pull-randomly-split')

function flat (err) {
  return {
    message: err.message,
    name: err.name,
   // stack: err.stack // TODO: comparing stacks is annoying across systems
  }
}

var examples = [
  {req: 0, stream: false, end: false, value: ['event', {okay: true}]}, // an event

  {req: 1, stream: false, end: false, value: 'whatever'}, // a request
  {req: 2, stream: true, end: false, value: new Buffer('hello')}, // a stream packet
  {req: -2, stream: true, end: false, value: new Buffer('goodbye')}, // a stream response
  {req: -3, stream: false, end: true, value: flat(new Error('intentional'))},
  {req: 2, stream: true, end: true, value: true}, // a stream packet
  {req: -2, stream: true, end: true, value: true}, // a stream response
  'GOODBYE'
]
pull(
  pull.values(examples),
  psc.encode(),
  split(),
  toPull.sink(process.stdout)
)


