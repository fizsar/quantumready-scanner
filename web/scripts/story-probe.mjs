// Walks /lab/historia in a real browser engine (Playwright: webkit or
// chromium, phone profile) and prints, at each scroll position, what the
// visitor sees: the stage's state, the track's rectangle and the viewport
// heights (innerHeight, visualViewport, 100vh, 100svh, 100dvh). For finding
// why a screen is empty. Manual.
//
//   node story-probe.mjs <url> [webkit|chromium] [WxHxDPR]
import { chromium, devices, webkit } from 'playwright'

const [url, engine = 'webkit', size = '393x852x3'] = process.argv.slice(2)
const [width, height, dpr] = size.split('x').map(Number)
const browser = await (engine === 'webkit' ? webkit : chromium).launch()
const ctx = await browser.newContext({
  ...devices['iPhone 13'],
  viewport: { width, height },
  deviceScaleFactor: dpr,
  ...(engine === 'webkit' ? {} : { isMobile: true }),
})
const page = await ctx.newPage()
const errors = []
page.on('pageerror', (e) => errors.push(e.message))
page.on('console', (m) => m.type() === 'error' && errors.push(m.text()))
await page.goto(url)
await page.waitForTimeout(3000)
const units = await page.evaluate(() => {
  const probe = (h) => {
    const d = document.createElement('div')
    d.style.cssText = `position:absolute;top:0;height:${h}`
    document.body.appendChild(d)
    const v = d.getBoundingClientRect().height
    d.remove()
    return v
  }
  return { innerHeight, visual: visualViewport?.height, vh: probe('100vh'), svh: probe('100svh'), dvh: probe('100dvh') }
})
console.log(engine, size, JSON.stringify(units))
const max = await page.evaluate(() => document.documentElement.scrollHeight - innerHeight)
for (let y = 0; y <= max; y += Math.round(height / 3)) {
  await page.evaluate((y) => scrollTo(0, y), y)
  await page.waitForTimeout(600)
  const s = await page.evaluate(() => {
    const t = document.querySelector('.story-track')?.getBoundingClientRect()
    const st = document.querySelector('.story-stage')
    const cs = st && getComputedStyle(st)
    return {
      top: Math.round(t?.top ?? NaN),
      bottom: Math.round(t?.bottom ?? NaN),
      active: st?.dataset.active,
      opacity: cs?.opacity,
      mode: document.querySelector('.story')?.dataset.mode,
      ready: document.querySelector('canvas')?.dataset.ready,
      sheet: document.querySelector('.story-sheet')?.dataset.active,
    }
  })
  console.log(`y=${y}`, JSON.stringify(s))
}
console.log(errors.length ? 'errors: ' + errors.join(' | ') : 'no errors')
await browser.close()
