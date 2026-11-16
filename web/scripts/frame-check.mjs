// Framing and light of every step of /lab/historia, in a real Chrome with its
// GPU (manual: Chrome started with --remote-debugging-port), through the
// DevTools protocol. For each viewport and each step: a screenshot, the free
// area (right of the text panel on wide screens, the stage above the sheet on
// phones) and, inside the stage:
// - cut: lit pixels (any channel ≥ 40) in a 4 px band along the free area's
//   edges, or outside it but inside the stage (behind the panel): the shape
//   is cut or overflows;
// - burnt: pixels burnt to white (every channel ≥ 235) among the lit ones.
// Fails if any position is cut (more than 0.02 % of the free area) or burnt
// (more than 2 % of its lit pixels). Positions: the top of the page as it
// opens (the stage entering; the engine may load after the page measured the
// free area), every step (progress i), half-way through every step (the
// camera between its cam and via), and the way out (the end of the story at
// 55 % of the height). The edges are those of the visible band too: entering
// and leaving, a lit pixel at the band's top or bottom edge is a straight cut
// (the stage must fade out there).
//
//   node frame-check.mjs <devtools-port> <url> [WxHxDPR,...]
import { writeFileSync } from 'node:fs'
import { inflateSync } from 'node:zlib'

const [port, url, sizesArg = '1280x800x1,1280x800x2,1440x900x1,1440x900x2,1920x1080x1,1920x1080x2,393x852x3'] = process.argv.slice(2)
const sizes = sizesArg.split(',').map((s) => s.split('x').map(Number))
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

function decodePNG(buf) {
  let p = 8
  let w = 0
  let h = 0
  let ct = 0
  const idat = []
  while (p < buf.length) {
    const len = buf.readUInt32BE(p)
    const type = buf.toString('ascii', p + 4, p + 8)
    const data = buf.subarray(p + 8, p + 8 + len)
    if (type === 'IHDR') [w, h, ct] = [data.readUInt32BE(0), data.readUInt32BE(4), data[9]]
    if (type === 'IDAT') idat.push(data)
    p += 12 + len
  }
  const bpp = ct === 6 ? 4 : 3
  const raw = inflateSync(Buffer.concat(idat))
  const out = Buffer.alloc(w * h * bpp)
  const stride = w * bpp
  for (let y = 0; y < h; y++) {
    const ft = raw[y * (stride + 1)]
    for (let x = 0; x < stride; x++) {
      const v = raw[y * (stride + 1) + 1 + x]
      const a = x >= bpp ? out[y * stride + x - bpp] : 0
      const b = y > 0 ? out[(y - 1) * stride + x] : 0
      const c = x >= bpp && y > 0 ? out[(y - 1) * stride + x - bpp] : 0
      let r = v
      if (ft === 1) r = v + a
      else if (ft === 2) r = v + b
      else if (ft === 3) r = v + ((a + b) >> 1)
      else if (ft === 4) {
        const pa = Math.abs(b - c)
        const pb = Math.abs(a - c)
        const pc = Math.abs(a + b - 2 * c)
        r = v + (pa <= pb && pa <= pc ? a : pb <= pc ? b : c)
      }
      out[y * stride + x] = r & 0xff
    }
  }
  return { w, h, bpp, px: out }
}

const t = await (
  await fetch(`http://127.0.0.1:${port}/json/new?about:blank`, {
    method: 'PUT',
  })
).json()
const ws = new WebSocket(t.webSocketDebuggerUrl)
await new Promise((ok) => (ws.onopen = ok))
let seq = 0
const pending = new Map()
ws.onmessage = (m) => {
  const msg = JSON.parse(m.data)
  if (msg.id) pending.get(msg.id)?.(msg.result)
}
const send = (method, params = {}) =>
  new Promise((ok) => {
    const id = ++seq
    pending.set(id, ok)
    ws.send(JSON.stringify({ id, method, params }))
  })
const ev = async (e) =>
  (
    await send('Runtime.evaluate', {
      expression: e,
      returnByValue: true,
      awaitPromise: true,
    })
  ).result?.value
await send('Page.enable')

let failed = false
for (const [width, height, dpr] of sizes) {
  const phone = width < 600
  await send('Emulation.setDeviceMetricsOverride', {
    width,
    height,
    deviceScaleFactor: dpr,
    mobile: phone,
  })
  await send('Emulation.setTouchEmulationEnabled', {
    enabled: phone,
    maxTouchPoints: 5,
  })
  await send('Page.navigate', { url })
  await sleep(6000)
  const steps = await ev(`document.querySelectorAll('.story-step').length`)
  const rows = []
  // progress = scrolled viewports + 0.35: progress i is the step's own shape and camera.
  const positions = [{ label: 'in' }]
  for (let i = 0; i < steps; i++) positions.push({ label: String(i).padStart(2), at: i - 0.35 }, { label: `${i}½`, at: i + 0.15 })
  positions.push({ label: 'out', out: true })
  for (const pos of positions) {
    const name = pos.label.trim()
    if (pos.out)
      await ev(`(() => { const t = document.querySelector('.story-track'); scrollTo(0, t.getBoundingClientRect().top + scrollY + t.offsetHeight - innerHeight * 0.55) })()`)
    else if (pos.at !== undefined)
      await ev(`(() => { const t = document.querySelector('.story-track'); scrollTo(0, t.getBoundingClientRect().top + scrollY + innerHeight * Math.max(0, ${pos.at})) })()`)
    await sleep(1800)
    const g = await ev(`(() => {
      const r = (e) => { if (!e || getComputedStyle(e).display === 'none') return null; const b = e.getBoundingClientRect(); return { l: b.left, t: b.top, r: b.right, b: b.bottom } }
      const stage = r(document.querySelector('.story-stage'))
      const sheet = r(document.querySelector('.story-sheet'))
      const panel = [...document.querySelectorAll('.story-panel')].map(r).find((p) => p && p.r > 1 && p.b > 0 && p.t < innerHeight)
      const cap = r(document.querySelector('.story-caption'))
      // The free area the page frames the shape in (--free-left/--free-right on the stage);
      // older versions do not set them: then right of the panel, to the stage's right edge.
      const st = document.querySelector('.story-stage')
      const fl = parseFloat(getComputedStyle(st).getPropertyValue('--free-left'))
      const fr = parseFloat(getComputedStyle(st).getPropertyValue('--free-right'))
      // Vertically, the part of the stage on screen: --vis-top/--vis-bottom, or the
      // clip-path inset() of older versions.
      const clip = getComputedStyle(st).clipPath.match(/^inset\(([^)]*)\)/)
      const ci = clip ? clip[1].split(/\s+/).map((x) => parseFloat(x) || 0) : [0, 0, 0, 0]
      const vt = parseFloat(getComputedStyle(st).getPropertyValue('--vis-top')) || ci[0]
      const vb0 = parseFloat(getComputedStyle(st).getPropertyValue('--vis-bottom'))
      const vb = Number.isFinite(vb0) ? vb0 : clip ? stage.b - stage.t - (ci[2] ?? ci[0]) : NaN
      const t = Math.max(0, stage.t + vt)
      const b = Math.min(innerHeight, Number.isFinite(vb) ? stage.t + vb : stage.b)
      const free = sheet ? { ...stage, t, b } : {
        l: Number.isFinite(fl) ? stage.l + fl : panel ? panel.r : 0,
        t,
        r: Number.isFinite(fr) ? stage.r - fr : stage.r,
        b,
      }
      return { stage, free, cap, ready: document.querySelector('canvas')?.dataset.ready, mode: document.querySelector('.story')?.dataset.mode }
    })()`)
    const { data } = await send('Page.captureScreenshot', { format: 'png' })
    const buf = Buffer.from(data, 'base64')
    if (process.env.SHOTS) writeFileSync(`frame-${width}x${height}x${dpr}-${name.padStart(2, '0')}.png`, buf)
    const img = decodePNG(buf)
    const s = img.w / width
    const box = (q) =>
      q && {
        l: Math.round(q.l * s),
        t: Math.round(q.t * s),
        r: Math.round(q.r * s),
        b: Math.round(q.b * s),
      }
    const free = box(g.free)
    const stage = box(g.stage)
    const cap = box(g.cap)
    const band = Math.round(4 * s)
    let lit = 0
    let burnt = 0
    let cut = 0
    const edges = { left: 0, right: 0, top: 0, bottom: 0 }
    for (let y = Math.max(0, stage.t); y < Math.min(img.h, stage.b); y++)
      for (let x = Math.max(0, stage.l); x < Math.min(img.w, stage.r); x++) {
        if (cap && x >= cap.l && x < cap.r && y >= cap.t && y < cap.b) continue // a label box (older versions)
        const o = (y * img.w + x) * img.bpp
        const mx = Math.max(img.px[o], img.px[o + 1], img.px[o + 2])
        if (mx < 40) continue
        const inside = x >= free.l && x < free.r && y >= free.t && y < free.b
        if (!inside) {
          // Behind the panel on wide screens: the panel's own text is lit too, so only
          // count what is outside the free area and not under the panel column.
          continue
        }
        lit++
        if (Math.min(img.px[o], img.px[o + 1], img.px[o + 2]) >= 235) burnt++
        if (x < free.l + band || x >= free.r - band || y < free.t + band || y >= free.b - band) {
          cut++
          if (process.env.DEBUG) edges[x < free.l + band ? 'left' : x >= free.r - band ? 'right' : y < free.t + band ? 'top' : 'bottom']++
        }
      }
    const area = (free.r - free.l) * (free.b - free.t)
    const cutPct = (100 * cut) / area
    const burntPct = (100 * burnt) / Math.max(1, lit)
    const bad = cutPct > 0.02 || burntPct > 2
    failed ||= bad
    rows.push(
      `${pos.label.padStart(3)} ${bad ? 'FAIL' : 'ok  '} cut ${cutPct.toFixed(3)} %  burnt ${burntPct.toFixed(1)} %` +
        (process.env.DEBUG ? ` ${JSON.stringify(edges)} free ${JSON.stringify(free)} stage ${JSON.stringify(stage)}` : ''),
    )
  }
  console.log(`== ${width}×${height} DPR ${dpr}\n` + rows.join('\n'))
}
ws.close()
await fetch(`http://127.0.0.1:${port}/json/close/${t.id}`)
process.exit(failed ? 1 : 0)
