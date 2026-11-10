// Performance of the redesign's scenes in a real Chrome with its GPU, through
// the DevTools protocol (Node's built-in fetch and WebSocket; no dependencies).
// Manual: needs Chrome started with --remote-debugging-port and the builds
// served locally (e.g. main's and this branch's dist with the production
// nginx config).
//
//   node lab-perf.mjs <devtools-port> home <base-url>...     the home page: FCP, LCP, scanner ready, long tasks, bytes
//   node lab-perf.mjs <devtools-port> scene <base-url> <path> FPS while scrolling through a scene, points, GPU memory
//   node lab-perf.mjs <devtools-port> hero <base-url>         FPS of the home's hero: during the entry and at rest
//
// Profiles: desktop (1440×900, DPR 1, no throttling) and mobile (390×844,
// DPR 3, touch, 4 cores, CPU ×4 and a slow 4G link: 150 ms, 1.6 Mbit/s).
// CPU throttling does not slow the GPU: the mobile FPS is a CPU-side estimate.
const [port, what, ...args] = process.argv.slice(2)
const runs = Number(process.env.RUNS ?? 5)
const SECONDS = Number(process.env.SECONDS ?? 10)

const PROFILES = {
  desktop: { width: 1440, height: 900, dpr: 1, mobile: false, cpu: 1, net: null, cores: null },
  mobile: { width: 390, height: 844, dpr: 3, mobile: true, cpu: 4, net: { latency: 150, downloadThroughput: (1.6e6 / 8) | 0, uploadThroughput: (750e3 / 8) | 0 }, cores: 4 },
}

async function open() {
  const t = await (await fetch(`http://127.0.0.1:${port}/json/new?about:blank`, { method: 'PUT' })).json()
  const ws = new WebSocket(t.webSocketDebuggerUrl)
  await new Promise((ok, bad) => ((ws.onopen = ok), (ws.onerror = bad)))
  let seq = 0
  const pending = new Map()
  const listeners = []
  ws.onmessage = (m) => {
    const msg = JSON.parse(m.data)
    if (msg.id && pending.has(msg.id)) {
      const { ok, bad } = pending.get(msg.id)
      pending.delete(msg.id)
      if (msg.error) bad(new Error(JSON.stringify(msg.error)))
      else ok(msg.result)
    } else if (msg.method) for (const l of listeners) l(msg)
  }
  const send = (method, params = {}) => new Promise((ok, bad) => { const id = ++seq; pending.set(id, { ok, bad }); ws.send(JSON.stringify({ id, method, params })) })
  const close = async () => { ws.close(); await fetch(`http://127.0.0.1:${port}/json/close/${t.id}`) }
  return { send, on: (f) => listeners.push(f), close }
}

async function setup(page, p) {
  await page.send('Page.enable')
  await page.send('Network.enable')
  await page.send('Network.setCacheDisabled', { cacheDisabled: true })
  await page.send('Emulation.setDeviceMetricsOverride', { width: p.width, height: p.height, deviceScaleFactor: p.dpr, mobile: p.mobile })
  await page.send('Emulation.setCPUThrottlingRate', { rate: p.cpu })
  if (p.mobile) await page.send('Emulation.setTouchEmulationEnabled', { enabled: true, maxTouchPoints: 5 })
  if (p.cores) await page.send('Emulation.setHardwareConcurrencyOverride', { hardwareConcurrency: p.cores })
  await page.send('Network.emulateNetworkConditions', p.net ? { offline: false, ...p.net } : { offline: false, latency: 0, downloadThroughput: -1, uploadThroughput: -1 })
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
const median = (xs) => { const s = [...xs].sort((a, b) => a - b); return s[s.length >> 1] }
const evaluate = async (page, expression) => (await page.send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })).result.value

// Installed before any page script: paint, LCP, long tasks and when the scanner's input appears.
const OBSERVERS = `
window.__perf = { lcp: 0, lcpEl: '', cls: 0, long: [], ready: 0 };
new PerformanceObserver((l) => { for (const e of l.getEntries()) { __perf.lcp = e.startTime; __perf.lcpEl = e.element ? e.element.tagName.toLowerCase() + (e.element.className ? '.' + String(e.element.className).split(' ').join('.') : '') : '?' } }).observe({ type: 'largest-contentful-paint', buffered: true });
new PerformanceObserver((l) => { for (const e of l.getEntries()) if (!e.hadRecentInput) __perf.cls += e.value }).observe({ type: 'layout-shift', buffered: true });
new PerformanceObserver((l) => { for (const e of l.getEntries()) __perf.long.push([e.startTime, e.duration]) }).observe({ type: 'longtask', buffered: true });
new MutationObserver((_, o) => { const i = document.querySelector('main form input'); if (i && !i.disabled) { __perf.ready = performance.now(); o.disconnect() } })
  .observe(document, { childList: true, subtree: true });`

async function home(base, profile) {
  const out = []
  for (let i = 0; i < runs; i++) {
    const page = await open()
    await setup(page, profile)
    await page.send('Page.addScriptToEvaluateOnNewDocument', { source: OBSERVERS })
    let bytes = 0
    let loaded = false
    page.on((m) => {
      if (m.method === 'Network.loadingFinished') bytes += m.params.encodedDataLength
      if (m.method === 'Page.loadEventFired') loaded = true
    })
    await page.send('Page.navigate', { url: `${base}/?lang=es` })
    for (let k = 0; k < 600 && !loaded; k++) await sleep(50)
    await sleep(profile.mobile ? 5000 : 2500)
    const r = await evaluate(page, `(() => {
      const nav = performance.getEntriesByType('navigation')[0]
      const fcp = performance.getEntriesByName('first-contentful-paint')[0]?.startTime ?? 0
      const ready = __perf.ready
      const tbt = __perf.long.filter(([s]) => s >= fcp && s < ready + 5000).reduce((a, [, d]) => a + Math.max(0, d - 50), 0)
      return { fcp, lcp: __perf.lcp, cls: __perf.cls, ready, load: nav.loadEventEnd, tbt, longTasks: __perf.long.length, lcpEl: __perf.lcpEl }
    })()`)
    out.push({ ...r, kb: bytes / 1024 })
    await page.close()
  }
  const keys = Object.keys(out[0]).filter((k) => k !== 'lcpEl')
  // The LCP element of each run (it must be the same one for the median to mean anything).
  return { ...Object.fromEntries(keys.map((k) => [k, Math.round(median(out.map((o) => o[k])) * (k === 'cls' ? 10000 : 10)) / (k === 'cls' ? 10000 : 10)])), lcpEl: [...new Set(out.map((o) => o.lcpEl))].join(' | ') }
}

// The home's hero: frames per second while the entry plays (from the first 3D
// frame to the Q̂ at rest) and then at rest, with the card on screen.
async function hero(base, profile) {
  const page = await open()
  await setup(page, profile)
  const errors = []
  await page.send('Runtime.enable')
  page.on((m) => {
    if (m.method === 'Runtime.exceptionThrown') errors.push(m.params.exceptionDetails.text)
    if (m.method === 'Runtime.consoleAPICalled' && m.params.type === 'error') errors.push(String(m.params.args[0]?.value).split('\n')[0])
  })
  // Counts frames per phase of the entry from the moment the canvas exists.
  await page.send('Page.addScriptToEvaluateOnNewDocument', {
    source: `window.__hero = { ready: 0, phases: {} }; (() => { let last = 0; const tick = (now) => { const c = document.querySelector('.hero-canvas'); if (c?.dataset.ready === 'true') { if (!__hero.ready) __hero.ready = now; const p = c.dataset.phase; const s = (__hero.phases[p] ??= { n: 0, ms: 0, max: 0 }); if (last) { s.n++; s.ms += now - last; s.max = Math.max(s.max, now - last) } } last = now; requestAnimationFrame(tick) }; requestAnimationFrame(tick) })()`,
  })
  await page.send('Page.navigate', { url: `${base}/?lang=es` })
  let info = null
  for (let k = 0; k < 300; k++) {
    await sleep(100)
    // Phones: the card is below the text; keep it on screen (the entry waits for it, and the
    // page above it grows as it loads).
    await evaluate(page, `document.querySelector('.hero-scene')?.scrollIntoView({ block: 'center' })`)
    info = await evaluate(page, `(() => { const c = document.querySelector('.hero-canvas'); return { mode: document.querySelector('.hero-scene')?.dataset.mode, phase: c?.dataset.phase, ready: __hero.ready } })()`)
    // The card shows the still image until the page has decided (after the first paint): give it 20 s.
    if (info?.phase === 'rest' || (info?.mode === 'still' && k > 200)) break
  }
  await sleep(SECONDS * 1000) // at rest
  const r = await evaluate(page, `(() => { const c = document.querySelector('.hero-canvas'); return { phases: __hero.phases, ready: __hero.ready, points: c && +c.dataset.points, drawn: c && +c.dataset.drawn, w: c?.width, h: c?.height } })()`)
  await page.close()
  if (info?.mode !== '3d') return { mode: info?.mode, errors }
  const fps = (names) => {
    const s = names.map((n) => r.phases[n]).filter(Boolean)
    const n = s.reduce((a, x) => a + x.n, 0)
    return n ? { fps: Math.round((n / s.reduce((a, x) => a + x.ms, 0)) * 10000) / 10, worstMs: Math.round(Math.max(...s.map((x) => x.max)) * 10) / 10 } : null
  }
  return { mode: '3d', errors, firstFrameMs: Math.round(r.ready), entry: fps(['handover', 'inhale', 'condense', 'hold', 'collapse']), rest: fps(['rest']), points: r.points, drawn: r.drawn, canvas: `${r.w}×${r.h}` }
}

async function scene(base, path, profile) {
  const page = await open()
  await setup(page, profile)
  // A scene that renders nothing (a shader error) still runs at 60 FPS: count errors too.
  const errors = []
  await page.send('Runtime.enable')
  page.on((m) => {
    if (m.method === 'Runtime.exceptionThrown') errors.push(m.params.exceptionDetails.text)
    if (m.method === 'Runtime.consoleAPICalled' && m.params.type === 'error') errors.push(String(m.params.args[0]?.value).split('\n')[0])
  })
  await page.send('Page.addScriptToEvaluateOnNewDocument', {
    source: `window.__ready = 0; new MutationObserver((_, o) => { if (document.querySelector('canvas[data-ready="true"]')) { __ready = performance.now(); o.disconnect() } }).observe(document, { subtree: true, attributes: true, childList: true });`,
  })
  await page.send('Page.navigate', { url: `${base}${path}` })
  // The story starts drawing only once its track fills the screen: go there.
  await sleep(1000)
  await evaluate(page, `(() => { const t = document.querySelector('.story-track'); if (t) scrollTo(0, t.getBoundingClientRect().top + scrollY) })()`)
  let info = null
  for (let k = 0; k < 400; k++) {
    await sleep(100)
    info = await evaluate(page, `(() => { const c = document.querySelector('canvas[data-points]'); return { mode: document.querySelector('[data-mode]')?.dataset.mode, ready: c?.dataset.ready === 'true', firstFrame: __ready, story: !!document.querySelector('.story') } })()`)
    if (info?.ready || info?.mode === 'still') break
  }
  // When each piece arrived (ms since navigation start).
  const timeline = await evaluate(page, `Object.fromEntries(performance.getEntriesByType('resource')
    .filter((e) => /index-|Story-|engine-|three\\.module-|\\.worker-|story-1s|jetbrains/.test(e.name))
    .map((e) => [e.name.split('/').pop().replace(/-[\\w-]{8}\\./, '.'), [Math.round(e.startTime), Math.round(e.responseEnd)]]))`)
  if (process.env.TIMELINE) console.log(JSON.stringify(timeline))
  const gpu = await evaluate(page, `(() => { const gl = document.createElement('canvas').getContext('webgl2'); const e = gl?.getExtension('WEBGL_debug_renderer_info'); return gl ? gl.getParameter(e ? e.UNMASKED_RENDERER_WEBGL : gl.RENDERER) : 'no webgl2' })()`)
  if (info?.mode !== '3d') {
    await page.close()
    return { mode: info?.mode, gpu, errors }
  }
  // Main-thread time while scrolling (PROFILE=1): script, style, layout, tasks.
  await page.send('Performance.enable')
  const metrics = async () => Object.fromEntries((await page.send('Performance.getMetrics')).metrics.map((m) => [m.name, m.value]))
  const m0 = await metrics()
  // Scroll through the track in SECONDS (10 by default) and count frames (the scene renders in
  // requestAnimationFrame too). Longer stories: scale SECONDS to compare at the same speed per step.
  const r = await evaluate(page, `(async () => {
    const t = document.querySelector('.lab-track, .story-track')
    const top = t.getBoundingClientRect().top + scrollY
    const span = t.offsetHeight - innerHeight
    scrollTo(0, top)
    // HIDE=selector: take an element out (to find what costs frames).
    for (const e of document.querySelectorAll(${JSON.stringify(process.env.HIDE ?? '')} || 'x-none')) e.style.display = 'none'
    await new Promise((r) => setTimeout(r, ${Number(process.env.SETTLE ?? 1500)})) // the adaptive point count settles (SETTLE ms)
    const dts = []
    let last = performance.now()
    const start = last
    await new Promise((done) => {
      const step = (now) => {
        dts.push(now - last); last = now
        const f = (now - start) / ${SECONDS * 1000}
        scrollTo(0, top + span * Math.min(1, f))
        f < 1 ? requestAnimationFrame(step) : done()
      }
      requestAnimationFrame(step)
    })
    dts.sort((a, b) => a - b)
    const c = document.querySelector('canvas[data-points]')
    return { fps: dts.length / ${SECONDS}, p95: dts[Math.floor(dts.length * 0.95)], points: +c.dataset.points, drawn: +c.dataset.drawn, w: c.width, h: c.height }
  })()`)
  if (process.env.PROFILE) {
    const m1 = await metrics()
    const keys = ['TaskDuration', 'ScriptDuration', 'RecalcStyleDuration', 'LayoutDuration', 'LayoutCount', 'RecalcStyleCount']
    console.log(JSON.stringify(Object.fromEntries(keys.map((k) => [k, Math.round((m1[k] - m0[k]) * (k.endsWith('Count') ? 1 : 1000))]))))
  }
  await page.close()
  // GPU memory: 4 orbitals × points × (3 floats position + 1 float sign or hue),
  // the story's wave seeds (4 floats per point), the drawing buffer (RGBA8,
  // double-buffered) and its depth buffer (4 bytes).
  const buffers = 4 * r.points * 16 + (info.story ? r.points * 16 : 0)
  const framebuffer = r.w * r.h * (4 * 2 + 4)
  return { mode: '3d', errors, gpu, firstFrameMs: Math.round(info.firstFrame), fps: Math.round(r.fps * 10) / 10, p95FrameMs: Math.round(r.p95 * 10) / 10, points: r.points, drawn: r.drawn, canvas: `${r.w}×${r.h}`, gpuMB: Math.round(((buffers + framebuffer) / 2 ** 20) * 10) / 10 }
}

for (const [name, profile] of Object.entries(PROFILES)) {
  if (what === 'home') for (const base of args) console.log(name, base, JSON.stringify(await home(base, profile)))
  else if (what === 'scene') console.log(name, JSON.stringify(await scene(args[0], args[1], profile)))
  else if (what === 'hero') console.log(name, JSON.stringify(await hero(args[0], profile)))
}
