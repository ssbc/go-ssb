// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

const Path = require('path')
const tape = require('tape')
const { readFileSync } = require('fs')
const { loadOrCreateSync } = require('ssb-keys')
const theStack = require('secret-stack')
const ssbCaps = require('ssb-caps')
const ebtBendyButt = require('ssb-ebt/formats/bendy-butt')
const ebtIndexed = require('ssb-ebt/formats/indexed')

// eval deps
const { where, author, and, type, live, toPullStream } = require('ssb-db2/operators')
const pull = require('pull-stream')
const parallel = require('run-parallel')

const testSHSappKey = bufFromEnv('TEST_APPKEY')
const testHMACkey = bufFromEnv('TEST_HMACKEY')

let testAppkey = Buffer.from(ssbCaps.shs, 'base64')
if (testSHSappKey !== false) {
  testAppkey = testSHSappKey
}
const stackOpts = { caps: { shs: testAppkey } }

if (testHMACkey !== false) {
  stackOpts.caps.sign = testHMACkey
}

const createSbot = theStack(stackOpts)
  .use(require('ssb-db2'))
  .use(require('ssb-db2/compat/ebt'))
  .use(require('ssb-ebt'))
  .use(require('ssb-meta-feeds'))
  .use(require('ssb-index-feed-writer'))

const testName = process.env.TEST_NAME
const testBob = process.env.TEST_BOB
const testPort = process.env.TEST_PORT

const scriptBefore = readFileSync(process.env.TEST_BEFORE).toString()
const scriptAfter = readFileSync(process.env.TEST_AFTER).toString()

tape.createStream().pipe(process.stderr)
tape(testName, (t) => {
  // t.timeoutAfter(30000) // doesn't exit the process
  const tapeTimeout = setTimeout(() => {
    t.comment("test timeout")
    process.exit(1)
  }, 50000)

  // call this from the eval block when you're done
  function exit () {
    clearTimeout(tapeTimeout)
    sbot.close(() => {
      t.comment('closed jsbot')
      t.end()
    })
  }

  const tempRepo = Path.join('..', 'testrun', testName)
  const keys = loadOrCreateSync(Path.join(tempRepo, 'secret'))
  const sbot = createSbot({
    port: testPort,
    // temp: testName,
    path: tempRepo,
    keys: keys,
    replicate: { legacy: false },
    ebt: { logging: false }
  })
  const alice = sbot.id

  // setup bendybutt format for ebt
  sbot.ebt.registerFormat(ebtBendyButt)
  sbot.ebt.registerFormat(ebtIndexed)

  sbot.ebt.request(alice, true)
  sbot.ebt.request(testBob, true)

  t.comment('sbot spawned, running before')

  eval(scriptBefore)

  // call this from the eval block once the sbot is prepared
  function ready () {
    console.log(alice)
  }

  sbot.on('rpc:connect', (remote) => {
    // t.equal(testBob, remote.id, "correct ID") // not the right format
    // t.comment(JSON.stringify(remote))
    eval(scriptAfter)
  })
})

// util
function bufFromEnv (evname) {
  const has = process.env[evname]
  if (has) {
    return Buffer.from(has, 'base64')
  }
  return false
}
