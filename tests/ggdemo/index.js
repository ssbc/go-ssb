// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

'use strict'
var pull  = require('pull-stream')
var grove = require('gabbygrove')

exports.name = 'gabbygrove'
exports.version = '1.0.0'
exports.manifest = {
  binaryStream: 'source'
}
exports.permissions = {
    anonymous: {allow: ['binaryStream']},
}


exports.init = function (sbot, config) {
    return {
        verify: grove.verifyTransfer,
        make: grove.makeEvent,

        binaryStream: function(args) {
            console.warn("binStream called, crafting some messages")
            // console.warn(args)
            // console.warn(arguments)

            let evt1 = grove.makeEventSync(config.keys, 1, null, {'type':'test','message':'hello world', 'level':0})
            let evt2 = grove.makeEventSync(config.keys, 2, evt1.key, {'type':'test','message':'exciting', 'level':9000})
            let evt3 = grove.makeEventSync(config.keys, 3, evt2.key, {'type':'test','message':'last', 'level':9001})

            return pull.values([
                evt1.trBytes,
                evt2.trBytes,
                evt3.trBytes,
            ])
        }
    }
}
