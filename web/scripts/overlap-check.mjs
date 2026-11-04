// The story, scrolled from top to bottom in real engines (Playwright:
// chromium and webkit, phone profiles), an eighth of a screen at a time.
// Fails if, at any position:
// - visible text lies over the visible 3D stage ([data-backdrop], with its
//   clip-path) without an opaque background (alpha ≥ 0.85) of its own;
// - the whole viewport is empty: no visible text and no visible scene (a
//   stage at least 30 % opaque, showing a loaded still image or a drawn canvas);
// and, once per page, if a panel that stays on screen scrolls inside or cuts
// an equation, or the scene keeps less than 40 % of a phone's height.
// Run by overlap-check.sh.
//
//   node overlap-check.mjs <base-url> <engine:WxHxDPR,...> <path>...
import { chromium, devices, webkit } from 'playwright'

const [base, profilesArg, ...paths] = process.argv.slice(2)
const profiles = profilesArg.split(',').map((p) => {
  const [engine, size] = p.split(':')
  const [width, height, dpr] = size.split('x').map(Number)
  return { engine, width, height, dpr }
})

// Shared by the checks below, in the page.
const HELPERS = `
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
  // The visible part of an element: its box, minus a clip-path inset(), within the viewport.
  const visibleBox = (e) => {
    const r = e.getBoundingClientRect()
    let [t, rt, b, l] = [0, 0, 0, 0]
    const m = getComputedStyle(e).clipPath.match(/^inset\\(([^)]*)\\)/)
    if (m) {
      const v = m[1].split(/\\s+/).map((x) => parseFloat(x) || 0)
      ;[t, rt, b, l] = [v[0], v[1] ?? v[0], v[2] ?? v[0], v[3] ?? v[1] ?? v[0]]
    }
    const box = { l: Math.max(0, r.left + l), t: Math.max(0, r.top + t), r: Math.min(innerWidth, r.right - rt), b: Math.min(innerHeight, r.bottom - b) }
    return box.r > box.l && box.b > box.t ? box : null
  }
  const texts = () => {
    const out = []
    const walk = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT)
    for (let n = walk.nextNode(); n; n = walk.nextNode()) {
      const el = n.parentElement
      if (!n.textContent.trim() || !el || el.closest('[data-backdrop]') || !shown(el)) continue
      const range = document.createRange()
      range.selectNodeContents(n)
      for (const q of range.getClientRects())
        if (q.right > 0 && q.left < innerWidth && q.bottom > 0 && q.top < innerHeight && q.width * q.height > 4) { out.push({ n, el, q }); break }
    }
    return out
  }
  const tag = (el) => el.tagName.toLowerCase() + (typeof el.className === 'string' && el.className ? '.' + el.className.split(' ')[0] : '')
`

const AT_POSITION = `(() => {${HELPERS}
  const stages = [...document.querySelectorAll('[data-backdrop]')].filter(shown).map((e) => ({ e, box: visibleBox(e) })).filter((s) => s.box)
  const words = texts()
  const over = []
  for (const { n, el, q } of words) {
    if (opaque(el)) continue
    if (stages.some(({ box: b }) => Math.min(b.r, q.right) - Math.max(b.l, q.left) > 2 && Math.min(b.b, q.bottom) - Math.max(b.t, q.top) > 2))
      over.push(tag(el) + ' "' + n.textContent.trim().slice(0, 40) + '"')
  }
  const scene = stages.some(({ e, box }) => +getComputedStyle(e).opacity >= 0.3 && (box.r - box.l) * (box.b - box.t) > 0.1 * innerWidth * innerHeight &&
    ([...e.querySelectorAll('img')].some((i) => i.complete && i.naturalWidth > 0 && shown(i)) || !!e.querySelector('canvas[data-ready="true"]')))
  return { over, empty: words.length === 0 && !scene }
})()`

const FIT = `(() => {${HELPERS}
  const out = []
  for (const box of document.querySelectorAll('.story-sheet, .story-caption')) {
    if (getComputedStyle(box).display === 'none') continue
    if (box.scrollHeight > box.clientHeight + 1) out.push(box.className.split(' ')[0] + ' scrolls inside: ' + box.scrollHeight + ' > ' + box.clientHeight + 'px')
    const r = box.getBoundingClientRect()
    for (const m of box.querySelectorAll('math')) {
      const b = m.getBoundingClientRect()
      if (!b.width) continue
      const holder = m.parentElement
      if (b.right > r.right + 1 || b.left < r.left - 1 || holder.scrollWidth > holder.clientWidth + 1) { out.push(box.className.split(' ')[0] + ' cuts an equation'); break }
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

let failed = false
const browsers = {}
for (const { engine, width, height, dpr } of profiles) {
  browsers[engine] ??= await (engine === 'webkit' ? webkit : chromium).launch()
  const phone = width < 600
  const ctx = await browsers[engine].newContext({
    ...(phone ? devices['iPhone 13'] : {}),
    viewport: { width, height },
    deviceScaleFactor: dpr,
    isMobile: phone,
    hasTouch: phone,
  })
  for (const path of paths) {
    const page = await ctx.newPage()
    await page.goto(base + path)
    await page.waitForTimeout(2500)
    const max = await page.evaluate(() => document.documentElement.scrollHeight - innerHeight)
    const over = new Map()
    const empty = []
    let positions = 0
    for (let y = 0; y <= max + 1; y += Math.round(height / 8)) {
      await page.evaluate((y) => scrollTo(0, y), Math.min(y, max))
      await page.waitForTimeout(350)
      positions++
      const r = await page.evaluate(AT_POSITION)
      for (const p of r.over) if (!over.has(p)) over.set(p, Math.min(y, max))
      if (r.empty) empty.push(Math.min(y, max))
      // Safari on iOS: the toolbars appear and hide while scrolling, changing the
      // visible height. Shrink by a toolbar (90 px), check again, and restore.
      if (engine === 'webkit' && phone) {
        await page.setViewportSize({ width, height: height - 90 })
        await page.waitForTimeout(250)
        const t = await page.evaluate(AT_POSITION)
        for (const p of t.over) if (!over.has(p)) over.set(p + ' (toolbar)', Math.min(y, max))
        if (t.empty) empty.push(`${Math.min(y, max)} (toolbar)`)
        await page.setViewportSize({ width, height })
        await page.waitForTimeout(150)
      }
    }
    const fit = await page.evaluate(FIT)
    const ok = over.size === 0 && empty.length === 0 && fit.length === 0
    failed ||= !ok
    console.log(`${ok ? 'ok  ' : 'FAIL'} ${engine} ${width}×${height} DPR ${dpr} ${path}: ${positions} positions` +
      (over.size ? `; text over the stage: ${[...over].slice(0, 5).map(([p, y]) => `${p} @${y}px`).join(' | ')}${over.size > 5 ? ` (+${over.size - 5})` : ''}` : '') +
      (empty.length ? `; EMPTY screen at ${empty.length} positions (${empty.slice(0, 6).join(', ')}px…)` : '') +
      (fit.length ? `; panels: ${fit.join(' | ')}` : ''))
    await page.close()
  }
  await ctx.close()
}
for (const b of Object.values(browsers)) await b.close()
process.exit(failed ? 1 : 0)
