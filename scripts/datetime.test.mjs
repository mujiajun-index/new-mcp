import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import test from 'node:test'
import vm from 'node:vm'

const require = createRequire(new URL('../web/package.json', import.meta.url))
const ts = require('typescript')
const module = { exports: {} }
vm.runInNewContext(ts.transpileModule(
  readFileSync(new URL('../web/src/lib/datetime.ts', import.meta.url), 'utf8'),
  { compilerOptions: { module: ts.ModuleKind.CommonJS, esModuleInterop: true } },
).outputText, { module, exports: module.exports, require })
const { toDateTimeLocalValue } = module.exports

test('expiry inputs show browser local time and preserve the instant when saved', () => {
  const originalTZ = process.env.TZ
  try {
    for (const [zone, expected] of [
      ['Asia/Shanghai', '2026-10-19T14:14'],
      ['UTC', '2026-10-19T06:14'],
      ['Etc/GMT+8', '2026-10-18T22:14'],
    ]) {
      process.env.TZ = zone
      const value = toDateTimeLocalValue('2026-10-19T14:14:00+08:00')
      assert.equal(value, expected, zone)
      assert.equal(new Date(value).toISOString(), '2026-10-19T06:14:00.000Z', zone)
      assert.equal(toDateTimeLocalValue('2026-10-19T06:14:00Z'), expected, zone)
    }
  } finally {
    if (originalTZ === undefined) delete process.env.TZ
    else process.env.TZ = originalTZ
  }
})

test('browser timezone rules account for US summer and winter offsets', () => {
  const originalTZ = process.env.TZ
  try {
    process.env.TZ = 'America/Los_Angeles'
    assert.equal(toDateTimeLocalValue('2026-10-19T14:14:00+08:00'), '2026-10-18T23:14')
    assert.equal(toDateTimeLocalValue('2026-01-19T14:14:00+08:00'), '2026-01-18T22:14')
  } finally {
    if (originalTZ === undefined) delete process.env.TZ
    else process.env.TZ = originalTZ
  }
})

test('empty and invalid dates produce an empty input value', () => {
  assert.equal(toDateTimeLocalValue(''), '')
  assert.equal(toDateTimeLocalValue('invalid'), '')
})
