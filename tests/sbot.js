const { readFileSync } = require('fs')
const { generate } = require('ssb-keys')
const pull = require('pull-stream')
const tape = require('tape')
const parallel = require('run-parallel')

const createSbot = require('ssb-server')
  .use(require('ssb-gossip'))
  .use(require('ssb-replicate'))
  .use(require('ssb-private'))
  .use(require('ssb-friends'))
  .use(require('ssb-blobs'))
  .use(require('./ggdemo'))


const testName = process.env['TEST_NAME']
const testBob = process.env['TEST_BOB']
const testAddr = process.env['TEST_GOADDR']

const scriptBefore = readFileSync(process.env['TEST_BEFORE']).toString()
const scriptAfter = readFileSync(process.env['TEST_AFTER']).toString()

let testSHSappKey = bufFromEnv('TEST_APPKEY')
let testHMACkey = bufFromEnv('TEST_HMACKEY')

function bufFromEnv(evname) {
  const has = process.env[evname]
  if (has) {
    return Buffer.from(has, "base64")
  }
  return false
}

tape.createStream().pipe(process.stderr);
tape(testName, function (t) {
  t.timeoutAfter(30000) // doesn't exit the process
  const tapeTimeout = setTimeout(() => {
    t.comment("test timeout")
    process.exit(1)
  }, 50000)
  function run() { // needs to be called by the before block when it's done
    const to = `net:${testAddr}~shs:${testBob.substr(1).replace('.ed25519', '')}`
    t.comment("dialing")
    console.warn('dialing:', to)
    sbot.connect(to, (err) => {
      t.error(err, "connected")
      eval(scriptAfter)
    })
  }

  function exit() { // call this when you're done
    sbot.close()
    t.comment('closed sbot')
    clearTimeout(tapeTimeout)
    t.end()
  }
  let opts = {
    temp: testName,
    keys: generate(),
  }

  if (testSHSappKey !== false) {
    opts.caps = opts.caps ? opts.caps : {}
    opts.caps.shs = testSHSappKey
  }

  if (testHMACkey !== false) {
    opts.caps = opts.caps ? opts.caps : {}
    opts.caps.sign = testHMACkey
  }

  const sbot = createSbot(opts)
  const alice = sbot.whoami()

  t.comment("sbot spawned, running before")
  console.log(alice.id) // tell go process who's incoming
  eval(scriptBefore)
})
