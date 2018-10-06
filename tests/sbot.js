const { readFileSync } = require('fs')
const { generate } = require('ssb-keys')
const pull = require('pull-stream')
const tape = require('tape')

const createSbot = require('scuttlebot')
  .use(require('scuttlebot/plugins/gossip'))
  .use(require('scuttlebot/plugins/replicate'))
  .use(require('scuttlebot/plugins/logging'))
  .use(require('ssb-blobs'))


const testName = process.env['TEST_NAME']
const testBob = process.env['TEST_BOB']
const testAddr = process.env['TEST_GOADDR']

const scriptBefore = readFileSync(process.env['TEST_BEFORE']).toString()
const scriptAfter = readFileSync(process.env['TEST_AFTER']).toString()

tape.createStream().pipe(process.stderr);
tape(testName, function (t) {
  function noErr(err) {
    t.error(err)
  }


  function logMe(name) {
    let logCnt = 0
    return function (err) {
      t.error(err)
      console.warn(name, logCnt, arguments)
      logCnt++
    }
  }

  const alice = generate()

  const sbot = createSbot({
    temp: testName,
    keys: alice,
    timeout: 1000
  })

  console.log(alice.id)

  eval(scriptBefore)

  function run() {
    const to = `net:${testAddr}~shs:${testBob.substr(1).replace('.ed25519', '')}`
    console.warn('dialing:', to)
    sbot.connect(to, (err) => {
      t.error(err)
      eval(scriptAfter)
    })
  }

  setTimeout(() => {
    // t.end()
    process.exit(0)
  }, 5000)
})
