// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

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

const { where, author, and, type, live, toPullStream } = require('ssb-db2/operators')
const bendyButt = require('ssb-ebt/formats/bendy-butt')

const createSbot = theStack({ caps: { shs: testAppkey } })
  .use(require('ssb-db2'))
  .use(require('ssb-db2/compat/ebt'))
  .use(require('ssb-ebt'))
  .use(require('ssb-meta-feeds'))

const testName = process.env.TEST_NAME
const testBob = process.env.TEST_BOB
const testPort = process.env.TEST_PORT

const scriptBefore = readFileSync(process.env.TEST_BEFORE).toString()
const scriptAfter = readFileSync(process.env.TEST_AFTER).toString()

tape.createStream().pipe(process.stderr)
tape(testName, function (t) {
  // t.timeoutAfter(30000) // doesn't exit the process
  //   const tapeTimeout = setTimeout(() => {
  //     t.comment("test timeout")
  //     process.exit(1)
  //   }, 50000)

  function exit () { // call this when you're done
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
    replicate: { legacy: false },
    ebt: { logging: false }
  })
  const alice = sbot.id

  // setup bendybutt format for ebt
  sbot.ebt.request(alice, true)

  sbot.ebt.registerFormat(bendyButt)
  sbot.ebt.request(testBob, true)

  t.comment('sbot spawned, running before')

  eval(scriptBefore)

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
