import fs from 'node:fs'
import path from 'node:path'
import { pathToFileURL } from 'node:url'

const [plugins, requests] = process.argv.slice(2)
if (!plugins || !requests) throw new Error('Usage: node check-video-gateway-protocols.mjs <plugins/tasks> <requests.json>')
const fixtures = JSON.parse(fs.readFileSync(requests, 'utf8'))
const failures = []
for (const fixture of fixtures) {
  const plugin = await import(pathToFileURL(path.resolve(plugins, fixture.key, 'plugin.js')).href)
  try {
    const result = await plugin.protocols.openai_video.decodeRequest({ model: fixture.model, upstreamModel: fixture.model, body: fixture.body })
    if (fixture.expected_error || result.kind !== 'submit' || result.model !== fixture.model || !result.requestBody) throw new Error(`${fixture.key}/${fixture.mode}: invalid decoded submission`)
    if (fixture.mode === 'json' && !JSON.stringify(result.requestBody).includes('https://fixtures.example/reference.png')) throw new Error('Reference image was lost during decoding')
  } catch (error) {
    if (!fixture.expected_error || !error.message.includes(fixture.expected_error)) failures.push(`${fixture.key}/${fixture.mode}: ${error.message}`)
  }
}
if (failures.length) throw new Error(failures.join('\n'))
console.log(`Verified ${fixtures.length} adapter requests across ${new Set(fixtures.map((f) => f.key)).size} gateway video plugins, including documented format restrictions.`)
