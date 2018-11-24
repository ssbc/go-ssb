const { readFileSync } = require('fs')
const { generate } = require('ssb-keys')
const pull = require('pull-stream')
const tape = require('tape')
const series = require('run-series')

const createSbot = require('scuttlebot')
  .use(require('scuttlebot/plugins/gossip'))
  .use(require('scuttlebot/plugins/replicate'))
  .use(require('scuttlebot/plugins/logging'))
  .use(require('ssb-friends'))
  .use(require('ssb-blobs'))


const testName = process.env['TEST_NAME']
const testBob = process.env['TEST_BOB']
const testAddr = process.env['TEST_GOADDR']

const scriptBefore = readFileSync(process.env['TEST_BEFORE']).toString()
const scriptAfter = readFileSync(process.env['TEST_AFTER']).toString()

tape.createStream().pipe(process.stderr);
tape(testName, function (t) {
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
    t.end()
  }

  function logMe(name) {
    let logCnt = 0
    return function (err) {
      t.error(err, name+"/loggerd")
      console.warn(name, logCnt, arguments)
      logCnt++
    }
  }

  const alice = generate()
  const sbot = createSbot({
    temp: testName,
    keys: alice,
  })

  t.comment("sbot spawned, running before")
  console.log(alice.id) // tell go process who's incoming
  eval(scriptBefore)
})
