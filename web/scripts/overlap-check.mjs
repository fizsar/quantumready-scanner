// Text over the 3D stage: scrolls each page from top to bottom (a quarter of
// a screen at a time) in a real Chromium, through the DevTools protocol, and
// reports every piece of visible text that lies over a visible backdrop
// ([data-backdrop]: the lab stages) without an opaque background (alpha ≥
// 0.85) of its own between them. Run by overlap-check.sh.
//
//   node overlap-check.mjs <chrome-host> <WxHxDPR,...> <url>...
const [host, sizesArg, ...urls] = process.argv.slice(2)
const sizes = sizesArg.split(',').map((s) => s.split('x').map(Number))

const target = await (await fetch(`http://${host}:9222/json/new?about:blank`, { method: 'PUT' })).json()
const ws = new WebSocket(target.webSocketDebuggerUrl.replace(/ws:\/\/[^/]+/, `ws://${host}:9222`))
await new Promise((ok, bad) => ((ws.onopen = ok), (ws.onerror = bad)))
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
const send = (method, params = {}) => new Promise((ok) => { const id = ++seq; pending.set(id, ok); ws.send(JSON.stringify({ id, method, params })) })
const ev = async (expression) => (await send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true })).result.result?.value
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

// In the page: visible text rectangles that intersect a visible backdrop, with nothing opaque in between.
const CHECK = `(() => {
  const shown = (e) => {
    for (let a = e; a && a !== document.documentElement; a = a.parentElement) {
      const cs = getComputedStyle(a)
      if (cs.display === 'none' || cs.visibility === 'hidden' || +cs.opacity < 0.05 || cs.clipPath === 'inset(50%)') return false
    }
    return true
  }
  const opaque = (e) => {
    for (let a = e; a && a !== document.body; a = a.parentElement) {
      const m = getComputedStyle(a).backgroundColor.match(/rgba?\\(([^)]+)\\)/)
      if (!m) continue
      const p = m[1].split(',').map(Number)
      if ((p.length === 4 ? p[3] : 1) >= 0.85) return true
    }
    return false
  }
  const vw = innerWidth, vh = innerHeight
  const backs = [...document.querySelectorAll('[data-backdrop]')].filter(shown).map((e) => e.getBoundingClientRect())
    .map((r) => ({ l: Math.max(0, r.left), t: Math.max(0, r.top), r: Math.min(vw, r.right), b: Math.min(vh, r.bottom) })).filter((r) => r.r > r.l && r.b > r.t)
  if (!backs.length) return []
  const out = []
  const walk = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT)
  for (let n = walk.nextNode(); n; n = walk.nextNode()) {
    const el = n.parentElement
    if (!n.textContent.trim() || !el || el.closest('[data-backdrop]') || !shown(el) || opaque(el)) continue
    const range = document.createRange()
    range.selectNodeContents(n)
    for (const q of range.getClientRects()) {
      const hit = backs.some((b) => Math.min(b.r, q.right) - Math.max(b.l, q.left) > 2 && Math.min(b.b, q.bottom) - Math.max(b.t, q.top) > 2)
      if (hit) { out.push(el.tagName.toLowerCase() + (el.className && typeof el.className === 'string' ? '.' + el.className.split(' ')[0] : '') + ' "' + n.textContent.trim().slice(0, 40) + '"'); break }
    }
  }
  return out
})()`

// In the page: the text panels that stay on screen (the phone sheet, the
// label) hold every step without scrolling inside and without cutting an
// equation; on phones the scene keeps at least 40 % of the height. Hidden
// layers are laid out too, so one look covers every step.
const FIT = `(() => {
  const out = []
  for (const box of document.querySelectorAll('.story-sheet, .story-caption')) {
    const cs = getComputedStyle(box)
    if (cs.display === 'none') continue
    if (box.scrollHeight > box.clientHeight + 1) out.push(box.className.split(' ')[0] + ' scrolls inside: ' + box.scrollHeight + ' > ' + box.clientHeight + 'px')
    const r = box.getBoundingClientRect()
    for (const m of box.querySelectorAll('math')) {
      const b = m.getBoundingClientRect()
      if (!b.width) continue // not rendered (a scene's group that is not shown)
      const holder = m.parentElement
      if (b.right > r.right + 1 || b.left < r.left - 1 || holder.scrollWidth > holder.clientWidth + 1) { out.push(box.className.split(' ')[0] + ' cuts an equation (' + Math.round(b.width) + 'px wide)'); break }
    }
  }
  const stage = document.querySelector('.story-stage')
  const sheet = document.querySelector('.story-sheet')
  if (stage && sheet && getComputedStyle(sheet).display !== 'none') {
    const h = stage.getBoundingClientRect().height
    if (h < 0.4 * innerHeight) out.push('scene only ' + Math.round(h) + 'px of ' + innerHeight + 'px')
  }
  return out
})()`

await send('Page.enable')
let failed = false
for (const [width, height, dpr] of sizes) {
  await send('Emulation.setDeviceMetricsOverride', { width, height, deviceScaleFactor: dpr, mobile: width < 600 })
  await send('Emulation.setTouchEmulationEnabled', { enabled: width < 600, maxTouchPoints: 5 })
  for (const url of urls) {
    events.length = 0
    await send('Page.navigate', { url })
    for (let i = 0; i < 100 && !events.includes('Page.loadEventFired'); i++) await sleep(50)
    await sleep(2500)
    const max = await ev('document.documentElement.scrollHeight - innerHeight')
    const problems = new Map()
    let positions = 0
    for (let y = 0; y <= max + 1; y += Math.round(height / 4)) {
      await ev(`scrollTo(0, ${Math.min(y, max)})`)
      await sleep(700) // fades and the scroll handler's frame
      positions++
      for (const p of await ev(CHECK)) if (!problems.has(p)) problems.set(p, Math.min(y, max))
    }
    const fit = await ev(FIT)
    const ok = problems.size === 0 && fit.length === 0
    failed ||= !ok
    console.log(`${ok ? 'ok  ' : 'FAIL'} ${url} at ${width}×${height} DPR ${dpr}: ${positions} scroll positions` +
      (problems.size ? `; text over the stage: ${[...problems].slice(0, 6).map(([p, y]) => `${p} @${y}px`).join(' | ')}${problems.size > 6 ? ` (+${problems.size - 6})` : ''}` : '') +
      (fit.length ? `; panels: ${fit.join(' | ')}` : ''))
  }
}
ws.close()
process.exit(failed ? 1 : 0)
