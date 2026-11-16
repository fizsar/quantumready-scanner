// Measures horizontal overflow of pages in a real Chromium, through the
// DevTools protocol (Node's built-in fetch and WebSocket; no dependencies).
// Run by overflow-check.sh inside a throwaway Docker network.
//
//   node overflow-check.mjs <chrome-host> <width> <url>...
//
// For each URL, with every "Under the hood" panel and every <details> open:
// document width vs viewport, and every element that sticks out on the right
// of the viewport, of a box that clips it (cut text), or of its container.
// Content inside a box that scrolls on its own (overflow-x auto/scroll: the
// wide tables) may be wider. Exits 1 if any page overflows.
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
  await send('Runtime.evaluate', {
    awaitPromise: true,
    expression: `(async () => {
      for (const b of document.querySelectorAll('button.uth-toggle[aria-expanded="false"]')) b.click()
      await new Promise((r) => setTimeout(r, 300))
      for (const d of document.querySelectorAll('details')) d.open = true
      await new Promise((r) => setTimeout(r, 300))
    })()`,
  })
  const { result } = await send('Runtime.evaluate', {
    returnByValue: true,
    expression: `(() => {
      const vw = document.documentElement.clientWidth
      const tag = (e) => e.tagName.toLowerCase() + (e.getAttribute('class') ? '.' + e.getAttribute('class').trim().split(/\\s+/).join('.') : '')
      const clipOf = (e) => {
        for (let a = e.parentElement; a && a !== document.body; a = a.parentElement) {
          const ox = getComputedStyle(a).overflowX
          if (ox === 'auto' || ox === 'scroll') return 'scroll'
          if (ox === 'hidden' || ox === 'clip') return a
        }
        return null
      }
      // Visually hidden on purpose (for screen readers), by class or by a media query: clip-path inset(50%).
      const srOnly = (e) => {
        for (let a = e; a && a !== document.body; a = a.parentElement) if (getComputedStyle(a).clipPath === 'inset(50%)') return true
        return false
      }
      const out = []
      for (const e of document.querySelectorAll('body *')) {
        if (!e.getClientRects().length || e.closest('.sr-only, marker, defs') || srOnly(e)) continue
        const b = e.getBoundingClientRect()
        if (b.width === 0) continue
        const clip = clipOf(e)
        if (clip === 'scroll') continue
        if (clip && b.right > clip.getBoundingClientRect().right + 1) { out.push(tag(e) + ' cut by ' + tag(clip)); continue }
        if (b.right > vw + 0.5) { out.push(tag(e) + ' right=' + Math.round(b.right)); continue }
        const p = e.parentElement
        if (!p || p === document.body || getComputedStyle(e).position !== 'static') continue
        const d = getComputedStyle(p).display
        if (d === 'inline' || d === 'contents') continue
        const pr = p.getBoundingClientRect().right
        if (b.right > pr + 1) out.push(tag(e) + ' out of ' + tag(p) + ' by ' + Math.round(b.right - pr) + 'px')
      }
      out.splice(5)
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
