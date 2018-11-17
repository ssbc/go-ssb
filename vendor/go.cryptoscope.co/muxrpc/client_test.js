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

// a simple RPC server for client tests
var MRPC = require('muxrpc')
var pull = require('pull-stream')
var toPull = require('stream-to-pull-stream')

var api = {
  finalCall: 'async',
  version: 'sync',
  hello: 'async',
  callme: {
    'async': 'async',
    'source': 'async',
  },
  object: 'async',
  stuff: 'source'
}

var server = MRPC(api, api)({
  finalCall: function(delay, cb) {
    setTimeout(() => {
      server.close()
    },delay)
    cb(null, "ty")
  },
  version: function(some, args, i) {
    console.warn(arguments)
    if (some === "wrong" && i === 42) {
      throw new Error("oh wow - sorry")
    }
    return "some/version@1.2.3"
  },
  hello: function (name, name2, cb) {
    console.error('hello:ok')
    cb(null, 'hello, ' + name + ' and ' + name2 + '!')
  },
  callme: {
    'source': function (cb) {
      pull(server.stuff(), pull.collect(function (err, vals) {
        if (err) {
          console.error(err)
          throw err
        }
        console.error('callme:source:ok vals:', vals)
        cb(err, "call done")
      }))
    },
    'async': function (cb) {
      server.hello(function (err, greet) {
        console.error('callme:async:ok')
        cb(err, "call done")
      })
    }
  },
  object: function (cb) {
    console.error('object:ok')
    cb(null, { with: 'fields!' })
  },
  stuff: function () {
    console.error('stuff called')
    return pull.values([{ "a": 1 }, { "a": 2 }, { "a": 3 }, { "a": 4 }])
  }
})

var a = server.createStream()
pull(a, toPull.sink(process.stdout))
pull(toPull.source(process.stdin), a)
