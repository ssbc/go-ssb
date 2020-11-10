const Path = require('path')
const tape = require('tape')
const { readFileSync } = require('fs')
const { loadOrCreateSync } = require('ssb-keys')
const theStack = require('secret-stack')
const ssbCaps = require('ssb-caps')

// eval deps
const pull = require('pull-stream')
const parallel = require('run-parallel')

const testSHSappKey = bufFromEnv('TEST_APPKEY')
const testHMACkey = bufFromEnv('TEST_HMACKEY')

let testAppkey = Buffer.from(ssbCaps.shs, 'base64')
if (testSHSappKey !== false) {
  testAppkey = testSHSappKey
}

const createSbot = theStack({caps: {shs: testAppkey } })
  .use(require('ssb-db'))
  .use(require('ssb-gossip'))
  .use(require('ssb-replicate'))
  .use(require('ssb-private'))
  .use(require('ssb-friends'))
  .use(require('ssb-blobs'))
  .use(require('ssb-identities'))
  .use(require('ssb-ebt'))

const testName = process.env['TEST_NAME']
const testBob = process.env['TEST_BOB']
const testPort = process.env['TEST_PORT']

const scriptBefore = readFileSync(process.env['TEST_BEFORE']).toString()
const scriptAfter = readFileSync(process.env['TEST_AFTER']).toString()

tape.createStream().pipe(process.stderr);
tape(testName, function (t) {
  // t.timeoutAfter(30000) // doesn't exit the process
//   const tapeTimeout = setTimeout(() => {
//     t.comment("test timeout")
//     process.exit(1)
//   }, 50000)
  
  

  function exit() { // call this when you're done
    sbot.close()
    t.comment('closed jsbot')
    // clearTimeout(tapeTimeout)
    t.end()
  }

  const tempRepo = Path.join('testrun', testName)
  const keys = loadOrCreateSync(Path.join(tempRepo, 'secret'))
  const sbot = createSbot({
    port: testPort,
    // temp: testName,
    path: tempRepo,
    keys: keys,
    replicate: {"legacy":false},
  })
  const alice = sbot.whoami()

//   const replicate_changes = sbot.replicate.changes()

  t.comment("sbot spawned, running before")
  eval(scriptBefore)
  
  function ready() {
    console.log(alice.id) // tell go process who our pubkey
  }
  
  sbot.on("rpc:connect", (remote, isClient) => {
    t.equal(testBob, remote.id, "correct ID")
    // t.true(isClient, "connected remote is client") ????
    // t.comment(JSON.stringify(remote))
    eval(scriptAfter)
    // return true
  })
})

// util
function bufFromEnv(evname) {
  const has = process.env[evname]
  if (has) {
    return Buffer.from(has, 'base64')
  }
  return false
}