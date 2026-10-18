// Measures horizontal overflow of pages in a real Chromium, through the
// DevTools protocol (Node's built-in fetch and WebSocket; no dependencies).
// Run by overflow-check.sh inside a throwaway Docker network.
//
//   node overflow-check.mjs <chrome-host> <width> <url>...
//
// For each URL: document width vs viewport, and the elements sticking out on
// the right. Exits 1 if any page overflows.
const [host, widthArg, ...urls] = process.argv.slice(2)
const width = Number(widthArg)

const target = await (await fetch(`http://${host}:9222/json/new?about:blank`, { method: 'PUT' })).json()
const ws = new WebSocket(target.webSocketDebuggerUrl.replace(/ws:\/\/[^/]+/, `ws://${host}:9222`))
await new Promise((ok, bad) => {
  ws.onopen = ok
  ws.onerror = bad
})
let seq = 0
const pending = new Map()
const events = []
ws.onmessage = (m) => {
  const msg = JSON.parse(m.data)
  if (msg.id && pending.has(msg.id)) {
    pending.get(msg.id)(msg)
    pending.delete(msg.id)
  } else if (msg.method) events.push(msg.method)
}
const send = (method, params = {}) =>
  new Promise((ok) => {
    const id = ++seq
    pending.set(id, ok)
    ws.send(JSON.stringify({ id, method, params }))
  })
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

await send('Page.enable')
await send('Emulation.setDeviceMetricsOverride', { width, height: 800, deviceScaleFactor: 1, mobile: true })
let failed = false
for (const url of urls) {
  events.length = 0
  await send('Page.navigate', { url })
  for (let i = 0; i < 100 && !events.includes('Page.loadEventFired'); i++) await sleep(50)
  await sleep(1000) // the SPA renders after load (lazy pages too)
  const { result } = await send('Runtime.evaluate', {
    returnByValue: true,
    expression: `(() => {
      const vw = document.documentElement.clientWidth
      const out = [...document.querySelectorAll('body *')]
        .filter((e) => e.getClientRects().length && e.getBoundingClientRect().right > vw + 0.5)
        .slice(0, 5)
        .map((e) => e.tagName.toLowerCase() + (e.getAttribute('class') ? '.' + e.getAttribute('class').split(' ').join('.') : '') + ' right=' + Math.round(e.getBoundingClientRect().right))
      const shown = [...document.querySelectorAll('svg.arch-diagram')].filter((s) => getComputedStyle(s).display !== 'none').map((s) => s.getAttribute('class'))
      return { scrollWidth: document.documentElement.scrollWidth, viewport: vw, h1: document.querySelector('h1')?.textContent ?? '', diagrams: shown, out }
    })()`,
  })
  const r = result.result.value
  const ok = r.scrollWidth <= r.viewport && r.out.length === 0
  failed ||= !ok
  console.log(`${ok ? 'ok  ' : 'FAIL'} ${url} at ${width}px: scrollWidth ${r.scrollWidth}, viewport ${r.viewport}; h1 "${r.h1}"` +
    (r.diagrams.length ? `; diagram shown: ${r.diagrams.join(', ')}` : '') + (r.out.length ? `; sticking out: ${r.out.join(' | ')}` : ''))
}
ws.close()
process.exit(failed ? 1 : 0)
