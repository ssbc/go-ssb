const ssbKeys = require('ssb-keys')

const createSbot = require('scuttlebot')
  .use(require('scuttlebot/plugins/gossip'))
  .use(require('scuttlebot/plugins/replicate'))
  .use(require('scuttlebot/plugins/logging'))
  .use(require('ssb-blobs'))

const testName = process.env['TEST_NAME']
const testBob = process.env['TEST_BOB']
const testAddr = process.env['TEST_GOADDR']

const alice = ssbKeys.generate()

const sbot = createSbot({
  temp: testName,
  keys: alice,
  timeout: 1000
})

console.log(alice.id)

const to =`net:${testAddr}~shs:${testBob.substr(1).replace('.ed25519','')}`
console.warn('dialing:', to)
sbot.connect(to, (err) => {
  if (err) throw err

})

setTimeout(() => {
  process.exit(0)
},2000)
