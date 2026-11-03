// After `npm run build`: three.js must not be in what the home page loads.
// Follows dist/index.html's scripts, modulepreloads and stylesheets, then every
// static import of those chunks (not dynamic imports), and fails if any of
// them contains three.js. Prints the gzip sizes of the initial load and of
// each lazy chunk. No dependencies.
//
//   node scripts/check-bundle.mjs [dist]
import { readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'
import { gzipSync } from 'node:zlib'

const dist = process.argv[2] ?? 'dist'
const html = readFileSync(join(dist, 'index.html'), 'utf8')
const refs = [...html.matchAll(/(?:src|href)="\/(assets\/[^"]+\.(?:js|css))"/g)].map((m) => m[1])
const initial = new Set()
const queue = [...refs]
while (queue.length) {
  const f = queue.shift()
  if (initial.has(f)) continue
  initial.add(f)
  if (!f.endsWith('.js')) continue
  // Static imports only: `import"./x.js"` and `from"./x.js"`; never `import("./x.js")`.
  for (const m of readFileSync(join(dist, f), 'utf8').matchAll(/(?:\bimport|\bfrom)\s*["']\.\/([\w.-]+\.js)["']/g)) queue.push(`assets/${m[1]}`)
}

// Property names three.js keeps after minification.
const isThree = (src) => src.includes('isWebGLRenderer') || src.includes('isBufferGeometry')
const kb = (n) => `${(n / 1024).toFixed(1)} KB`
const gz = (f) => gzipSync(readFileSync(join(dist, f))).length

let failed = false
let total = 0
console.log('initial load (gzip):')
for (const f of initial) {
  const size = gz(f)
  total += size
  console.log(`  ${f}  ${kb(size)}`)
  if (f.endsWith('.js') && isThree(readFileSync(join(dist, f), 'utf8'))) {
    console.error(`  ✗ ${f} contains three.js`)
    failed = true
  }
}
console.log(`  total ${kb(total)}`)

const lazy = readdirSync(join(dist, 'assets')).filter((f) => f.endsWith('.js') && !initial.has(`assets/${f}`))
console.log('lazy chunks (gzip):')
let threeChunks = 0
for (const f of lazy) {
  const three = isThree(readFileSync(join(dist, 'assets', f), 'utf8'))
  if (three) threeChunks++
  console.log(`  assets/${f}  ${kb(gz(`assets/${f}`))}${three ? '  (three.js)' : ''}`)
}
if (threeChunks !== 1) {
  console.error(`✗ expected three.js in exactly one lazy chunk, found ${threeChunks}`)
  failed = true
}
process.exit(failed ? 1 : 0)
