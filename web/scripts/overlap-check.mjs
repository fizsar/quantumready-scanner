// The story, scrolled from top to bottom in real engines (Playwright:
// chromium and webkit, phone and desktop profiles), an eighth of a screen at
// a time. Fails if, at any position:
// - visible text lies over the visible 3D stage ([data-backdrop]: the part on
//   screen, --vis-top/--vis-bottom, or its clip-path in older versions)
//   without an opaque background (alpha ≥ 0.85) of its own;
// - on wide screens, a text box (even an opaque one) covers the free area the
//   shape is framed in (right of the panels: --free-left/--free-right, or
//   right of the panel column in older versions);
// - the whole viewport is empty: no visible text and no visible scene (a
//   stage at least 30 % opaque, showing a loaded still image or a drawn canvas);
// - a text panel (the phone sheet, a step panel) is visible while the visible
//   scene (above the sheet, if any) is shorter than 40 % of the height;
// and, once per page, if a panel scrolls inside, cuts an equation or breaks a
// title inside an expression (.story-nowrap on more than one line), or the
// scene keeps less than 40 % of a phone's height.
// On the home page (its hero card is a stage too) it also fails if the form
// is not on the first screen from the first paint, if the scene's card
// overlaps a control, if something covers a control where it would be
// tapped, or if the page scrolls sideways (HOME below).
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
  // The visible part of an element, within the viewport: its box, limited to the
  // band it shows (--vis-top/--vis-bottom, from its top; the edges fade there) or
  // minus a clip-path inset() (older versions).
  const visibleBox = (e) => {
    const r = e.getBoundingClientRect()
    const cs = getComputedStyle(e)
    let [t, rt, b, l] = [0, 0, 0, 0]
    const m = cs.clipPath.match(/^inset\\(([^)]*)\\)/)
    if (m) {
      const v = m[1].split(/\\s+/).map((x) => parseFloat(x) || 0)
      ;[t, rt, b, l] = [v[0], v[1] ?? v[0], v[2] ?? v[0], v[3] ?? v[1] ?? v[0]]
    }
    // The mask's stops are pixels from the element's top: the band stays put even
    // if the element's height changes (Safari's toolbars) before the page updates it.
    const vt = parseFloat(cs.getPropertyValue('--vis-top'))
    const vb = parseFloat(cs.getPropertyValue('--vis-bottom'))
    if (Number.isFinite(vt)) t = Math.max(t, vt)
    const bottom = Math.min(r.bottom - b, Number.isFinite(vb) ? r.top + vb : Infinity)
    const box = { l: Math.max(0, r.left + l), t: Math.max(0, r.top + t), r: Math.min(innerWidth, r.right - rt), b: Math.min(innerHeight, bottom) }
    return box.r > box.l && box.b > box.t ? box : null
  }
  // Wide screens: where the shape is framed, inside a visible stage box.
  const freeBox = (e, box) => {
    const sheet = document.querySelector('.story-sheet')
    if (sheet && getComputedStyle(sheet).display !== 'none') return null
    const r = e.getBoundingClientRect()
    const cs = getComputedStyle(e)
    const fl = parseFloat(cs.getPropertyValue('--free-left'))
    const fr = parseFloat(cs.getPropertyValue('--free-right'))
    const panels = [...document.querySelectorAll('.story-panel')].filter(shown).map((p) => p.getBoundingClientRect()).filter((p) => p.width > 1)
    const l = Number.isFinite(fl) ? r.left + fl : Math.max(box.l, ...panels.map((p) => p.right))
    const f = { l: Math.max(box.l, l), t: box.t, r: Math.min(box.r, Number.isFinite(fr) ? r.right - fr : box.r), b: box.b }
    return f.r > f.l && f.b > f.t ? f : null
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
  const hits = (b, q) => Math.min(b.r, q.right) - Math.max(b.l, q.left) > 2 && Math.min(b.b, q.bottom) - Math.max(b.t, q.top) > 2
  const frees = stages.map(({ e, box }) => freeBox(e, box)).filter(Boolean)
  for (const { n, el, q } of words) {
    const what = tag(el) + ' "' + n.textContent.trim().slice(0, 40) + '"'
    if (!opaque(el)) {
      if (stages.some(({ box }) => hits(box, q))) over.push(what)
    } else if (frees.some((f) => hits(f, q))) over.push('box over the shape: ' + what)
  }
  const scene = stages.some(({ e, box }) => +getComputedStyle(e).opacity >= 0.3 && (box.r - box.l) * (box.b - box.t) > 0.1 * innerWidth * innerHeight &&
    ([...e.querySelectorAll('img')].some((i) => i.complete && i.naturalWidth > 0 && shown(i)) || !!e.querySelector('canvas[data-ready="true"]')))
  // Panels only with the scene in place: the visible scene (above the sheet) at least 40 % of the height.
  const sheet = document.querySelector('.story-sheet')
  const sheetOn = sheet && getComputedStyle(sheet).display !== 'none' && shown(sheet)
  const panelOn = [...document.querySelectorAll('.story-sheet, .story-panel')].some((p) => {
    if (getComputedStyle(p).display === 'none' || !shown(p)) return false
    const b = p.getBoundingClientRect()
    return b.width > 1 && b.height > 1 && b.bottom > 0 && b.top < innerHeight
  })
  const sceneH = Math.max(0, ...stages.map(({ box }) => Math.min(box.b, sheetOn ? sheet.getBoundingClientRect().top : Infinity) - box.t))
  const early = panelOn && sceneH < 0.4 * innerHeight ? Math.round(sceneH) : null
  return { over, empty: words.length === 0 && !scene, early }
})()`

const FIT = `(() => {${HELPERS}
  const out = []
  for (const box of document.querySelectorAll('.story-sheet, .story-caption, .story-panel')) {
    if (getComputedStyle(box).display === 'none' || !shown(box)) continue
    // One rect per text node, so count the lines they sit on (their tops), not the rects.
    const lines = (n) => new Set([...n.getClientRects()].filter((r) => r.width > 0).map((r) => Math.round(r.top))).size
    for (const n of box.querySelectorAll('.story-nowrap')) if (lines(n) > 1) { out.push(box.className.split(' ')[0] + ' breaks "' + n.textContent.trim() + '"'); break }
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

// Read the page as it would be painted: after a scroll or a resize, two animation
// frames. The page updates in requestAnimationFrame, right before each paint, so a
// painted frame always went through its update; but headless WebKit can stop making
// frames for 200–500 ms after setViewportSize, and a fixed wait then read a DOM no
// frame had shown (the stage still on past the end of the story, never on screen).
const settle = (page) => page.evaluate(() => new Promise((ok) => requestAnimationFrame(() => requestAnimationFrame(() => ok()))))

// The home page's entry (a page with .hero): the form is on the first screen
// and nothing is ever over it. At the top of the page, as soon as the form
// exists (the first paint) and again once everything has loaded; then at every
// scroll position, for whatever controls are on screen.
const HOME = `((first) => {${HELPERS}
  const hero = document.querySelector('.hero')
  if (!hero) return null
  const out = []
  const card = document.querySelector('.hero-scene').getBoundingClientRect()
  const controls = [...hero.querySelectorAll('input, button, a[href]')].filter(shown)
  if (first) {
    for (const c of hero.querySelectorAll('.scan-form input, .scan-form button[type=submit]')) {
      const b = c.getBoundingClientRect()
      if (b.top < 0 || b.bottom > innerHeight) out.push(tag(c) + ' not on the first screen (' + Math.round(b.bottom) + ' of ' + innerHeight + 'px)')
    }
    if (!hero.querySelector('.hero-title')?.textContent.trim()) out.push('no title')
    if (!shown(document.querySelector('.hero-scene')) || card.width < 100) out.push('no scene card')
  }
  for (const c of controls) {
    const b = c.getBoundingClientRect()
    if (Math.min(b.right, card.right) - Math.max(b.left, card.left) > 0 && Math.min(b.bottom, card.bottom) - Math.max(b.top, card.top) > 0) out.push('the scene overlaps ' + tag(c))
    // What a tap in its middle would hit, if it is on screen (a link that wraps: the middle of its first line).
    const q = c.getClientRects()[0] ?? b
    const x = (q.left + q.right) / 2
    const y = (q.top + q.bottom) / 2
    if (x < 0 || x >= innerWidth || y < 0 || y >= innerHeight) continue
    const hit = document.elementFromPoint(x, y)
    if (hit && hit !== c && !c.contains(hit)) out.push(tag(c) + ' covered by ' + tag(hit))
  }
  if (document.documentElement.scrollWidth > innerWidth + 1) out.push('horizontal scroll: ' + document.documentElement.scrollWidth + ' > ' + innerWidth)
  const label = document.querySelector('.hero-label')
  if (label && shown(label) && !opaque(label)) out.push('the label has no opaque background')
  return out
})`

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
    const home = new Set()
    // The home's form must be usable from the first paint: check before anything else has loaded.
    await page.waitForSelector('.scan-form input, .story', { timeout: 15000 })
    for (const p of (await page.evaluate(`${HOME}(true)`)) ?? []) home.add(p + ' (first paint)')
    await page.waitForTimeout(2500)
    for (const p of (await page.evaluate(`${HOME}(true)`)) ?? []) home.add(p)
    const max = await page.evaluate(() => document.documentElement.scrollHeight - innerHeight)
    const over = new Map()
    const empty = []
    const early = []
    let positions = 0
    for (let y = 0; y <= max + 1; y += Math.round(height / 8)) {
      await page.evaluate((y) => scrollTo(0, y), Math.min(y, max))
      await page.waitForTimeout(350)
      await settle(page)
      positions++
      const r = await page.evaluate(AT_POSITION)
      for (const p of (await page.evaluate(`${HOME}(false)`)) ?? []) home.add(`${p} @${Math.min(y, max)}px`)
      for (const p of r.over) if (!over.has(p)) over.set(p, Math.min(y, max))
      if (r.empty) empty.push(Math.min(y, max))
      if (r.early !== null) early.push(`${Math.min(y, max)} (scene ${r.early}px)`)
      // Safari on iOS: the toolbars appear and hide while scrolling, changing the
      // visible height. Shrink by a toolbar (90 px), check again, and restore.
      if (engine === 'webkit' && phone) {
        await page.setViewportSize({ width, height: height - 90 })
        await page.waitForTimeout(250)
        await settle(page)
        const t = await page.evaluate(AT_POSITION)
        for (const p of t.over) if (!over.has(p)) over.set(p + ' (toolbar)', Math.min(y, max))
        if (t.empty) empty.push(`${Math.min(y, max)} (toolbar)`)
        if (t.early !== null) early.push(`${Math.min(y, max)} (toolbar, scene ${t.early}px)`)
        await page.setViewportSize({ width, height })
        await page.waitForTimeout(150)
        await settle(page)
      }
    }
    const fit = await page.evaluate(FIT)
    const ok = over.size === 0 && empty.length === 0 && early.length === 0 && fit.length === 0 && home.size === 0
    failed ||= !ok
    console.log(`${ok ? 'ok  ' : 'FAIL'} ${engine} ${width}×${height} DPR ${dpr} ${path}: ${positions} positions` +
      (over.size ? `; text over the stage: ${[...over].slice(0, 5).map(([p, y]) => `${p} @${y}px`).join(' | ')}${over.size > 5 ? ` (+${over.size - 5})` : ''}` : '') +
      (empty.length ? `; EMPTY screen at ${empty.length} positions (${empty.slice(0, 6).join(', ')}px…)` : '') +
      (early.length ? `; PANEL with the scene < 40 % at ${early.length} positions (${early.slice(0, 4).join(', ')}…)` : '') +
      (fit.length ? `; panels: ${fit.join(' | ')}` : '') +
      (home.size ? `; home: ${[...home].slice(0, 6).join(' | ')}` : ''))
    await page.close()
  }
  await ctx.close()
}
for (const b of Object.values(browsers)) await b.close()
process.exit(failed ? 1 : 0)
