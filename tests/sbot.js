const { readFileSync } = require('fs')
const { generate } = require('ssb-keys')
const pull = require('pull-stream')

const createSbot = require('scuttlebot')
  .use(require('scuttlebot/plugins/gossip'))
  .use(require('scuttlebot/plugins/replicate'))
  .use(require('scuttlebot/plugins/logging'))
  .use(require('ssb-blobs'))


function logMe(err) {
  if (err) throw err
  console.warn(arguments)
}

const testName = process.env['TEST_NAME']
const testBob = process.env['TEST_BOB']
const testAddr = process.env['TEST_GOADDR']

const scriptBefore = readFileSync(process.env['TEST_BEFORE']).toString()
const scriptAfter = readFileSync(process.env['TEST_AFTER']).toString()

const alice = generate()

const sbot = createSbot({
  temp: testName,
  keys: alice,
  timeout: 1000
})

console.log(alice.id)

eval(scriptBefore)

const to =`net:${testAddr}~shs:${testBob.substr(1).replace('.ed25519','')}`
console.warn('dialing:', to)
sbot.connect(to, (err) => {
  if (err) throw err
  eval(scriptAfter)
})

setTimeout(() => {
  process.exit(0)
},2000)
