'use strict';
var PacketStream = require('packet-stream')
var pull         = require('pull-stream')
var pullWeird    = require('./pull-weird')
var goodbye      = require('pull-goodbye')
var u            = require('./util')
var explain      = require('explain-error')

function isFunction (f) {
  return 'function' === typeof f
}

function isString (s) {
  return 'string' === typeof s
}

function isObject (o) {
  return o && 'object' === typeof o
}

function isSource    (t) { return 'source' === t }
function isSink      (t) { return 'sink'   === t }
function isDuplex    (t) { return 'duplex' === t }
function isSync      (t) { return 'sync'  === t }
function isAsync     (t) { return 'async'  === t }
function isRequest   (t) { return isSync(t) || isAsync(t) }
function isStream    (t) { return isSource(t) || isSink(t) || isDuplex(t) }

module.exports = function initStream (localCall, codec, onClose) {

  var ps = PacketStream({
    message: function (msg) {
//      if(isString(msg)) return
//      if(msg.length > 0 && isString(msg[0]))
//        localCall('msg', 'emit', msg)
    },
    request: function (opts, cb) {
      var name = opts.name, args = opts.args
      var inCB = false, called = false, async = false, value

      if (typeof args === 'undefined') {
        console.trace('ouch! args undefined!')
      if (typeof opts === 'boolean') { // closed??
        return
      } else {
        console.warn("opts:",typeof opts)
      }
    }
      args.push(function (err, value) {
        called = true
        inCB = true; cb(err, value)
      })
      try {
        value = localCall('async', name, args)
      } catch (err) {
        if(inCB || called) throw explain(err, 'no callback provided to muxrpc async funtion')
        return cb(err)
      }

    },
    stream: function (stream) {
      stream.read = function (data, end) {
        var name = data.name
        var type = data.type
        var err, value

        stream.read = null

        if(!isStream(type))
          return stream.write(null, new Error('unsupported stream type:'+type))

        //how would this actually happen?
        if(end) return stream.write(null, end)

        try { value = localCall(type, name, data.args) }
        catch (_err) { err = _err }

        var _stream = pullWeird[
          {source: 'sink', sink: 'source'}[type] || 'duplex'
        ](stream)

        return u.pipeToStream(
          type, _stream,
          err ? u.errorAsStream(type, err) : value
        )

//        if(isSource(type))
//          _stream(err ? pull.error(err) : value)
//        else if (isSink(type))
//          (err ? abortSink(err) : value)(_stream)
//        else if (isDuplex(type))
//          pull(_stream, err ? abortDuplex(err) : value, _stream)
      }
    },

    close: function (err) {
        ps = null // deallocate
        ws.ended = true
        if(ws.closed) return
        ws.closed = true
        if(onClose) {
          var close = onClose; onClose = null; close(err)
        }
      }
  })

  var ws = goodbye(pullWeird(ps, function (_) {
    //this error will be handled in PacketStream.close
  }))

  ws = codec ? codec(ws) : ws

  ws.remoteCall = function (type, name, args, cb) {
    if(name === 'emit') return ps.message(args)

    if(!(isRequest(type) || isStream(type)))
      throw new Error('unsupported type:' + JSON.stringify(type))

    if(isRequest(type))
      return ps.request({name: name, args: args}, cb)

    var ws = ps.stream(), s = pullWeird[type](ws, cb)
    ws.write({name: name, args: args, type: type})
    return s
  }


  //hack to work around ordering in setting ps.ended.
  //Question: if an object has subobjects, which
  //all have close events, should the subobjects fire close
  //before the parent? or should parents close after?
  //should there be a preclose event on the parent
  //that fires when it's about to close all the children?
  ws.isOpen = function () {
    return !ps.ended
  }

  ws.close = function (err, cb) {
    if(isFunction(err))
      cb = err, err = false
    if(!ps) return (cb && cb())
    if(err) return ps.destroy(err), (cb && cb())

    ps.close(function (err) {
      if(cb) cb(err)
      else if(err) throw explain(err, 'no callback provided for muxrpc close')
    })

    return this
  }
  ws.closed = false

  return ws
}



