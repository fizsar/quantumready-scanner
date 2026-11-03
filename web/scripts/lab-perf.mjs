// Performance of the redesign's scenes in a real Chrome with its GPU, through
// the DevTools protocol (Node's built-in fetch and WebSocket; no dependencies).
// Manual: needs Chrome started with --remote-debugging-port and the builds
// served locally (e.g. main's and this branch's dist with the production
// nginx config).
//
//   node lab-perf.mjs <devtools-port> home <base-url>...     the home page: FCP, LCP, scanner ready, long tasks, bytes
//   node lab-perf.mjs <devtools-port> scene <base-url> <path> FPS while scrolling through a scene, points, GPU memory
//
// Profiles: desktop (1440×900, DPR 1, no throttling) and mobile (390×844,
// DPR 3, touch, 4 cores, CPU ×4 and a slow 4G link: 150 ms, 1.6 Mbit/s).
// CPU throttling does not slow the GPU: the mobile FPS is a CPU-side estimate.
const [port, what, ...args] = process.argv.slice(2)
const runs = Number(process.env.RUNS ?? 5)

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
      msg.error ? bad(new Error(JSON.stringify(msg.error))) : ok(msg.result)
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
window.__perf = { lcp: 0, long: [], ready: 0 };
new PerformanceObserver((l) => { for (const e of l.getEntries()) __perf.lcp = e.startTime }).observe({ type: 'largest-contentful-paint', buffered: true });
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
      return { fcp, lcp: __perf.lcp, ready, load: nav.loadEventEnd, tbt, longTasks: __perf.long.length }
    })()`)
    out.push({ ...r, kb: bytes / 1024 })
    await page.close()
  }
  const keys = Object.keys(out[0])
  return Object.fromEntries(keys.map((k) => [k, Math.round(median(out.map((o) => o[k])) * 10) / 10]))
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
  let info = null
  for (let k = 0; k < 400; k++) {
    await sleep(100)
    info = await evaluate(page, `(() => { const c = document.querySelector('canvas.lab-canvas'); return { mode: document.querySelector('.lab')?.dataset.mode, ready: c?.dataset.ready === 'true', firstFrame: __ready } })()`)
    if (info?.ready || info?.mode === 'still') break
  }
  // When each piece arrived (ms since navigation start).
  const timeline = await evaluate(page, `Object.fromEntries(performance.getEntriesByType('resource')
    .filter((e) => /index-|LabOrbital-|orbitalScene-|sampler\\.worker-|orbital-1s|jetbrains/.test(e.name))
    .map((e) => [e.name.split('/').pop().replace(/-[\\w-]{8}\\./, '.'), [Math.round(e.startTime), Math.round(e.responseEnd)]]))`)
  if (process.env.TIMELINE) console.log(JSON.stringify(timeline))
  const gpu = await evaluate(page, `(() => { const gl = document.createElement('canvas').getContext('webgl2'); const e = gl?.getExtension('WEBGL_debug_renderer_info'); return gl ? gl.getParameter(e ? e.UNMASKED_RENDERER_WEBGL : gl.RENDERER) : 'no webgl2' })()`)
  if (info?.mode !== '3d') {
    await page.close()
    return { mode: info?.mode, gpu, errors }
  }
  // Scroll through the track in 10 s and count frames (the scene renders in requestAnimationFrame too).
  const r = await evaluate(page, `(async () => {
    const t = document.querySelector('.lab-track')
    const top = t.getBoundingClientRect().top + scrollY
    const span = t.offsetHeight - innerHeight
    scrollTo(0, top)
    await new Promise((r) => setTimeout(r, 1500)) // the adaptive point count settles
    const dts = []
    let last = performance.now()
    const start = last
    await new Promise((done) => {
      const step = (now) => {
        dts.push(now - last); last = now
        const f = (now - start) / 10000
        scrollTo(0, top + span * Math.min(1, f))
        f < 1 ? requestAnimationFrame(step) : done()
      }
      requestAnimationFrame(step)
    })
    dts.sort((a, b) => a - b)
    const c = document.querySelector('canvas.lab-canvas')
    return { fps: dts.length / 10, p95: dts[Math.floor(dts.length * 0.95)], points: +c.dataset.points, drawn: +c.dataset.drawn, w: c.width, h: c.height }
  })()`)
  await page.close()
  // GPU memory: 4 orbitals × points × (3 floats position + 1 float sign), the
  // drawing buffer (RGBA8, double-buffered) and its depth buffer (4 bytes).
  const buffers = 4 * r.points * 16
  const framebuffer = r.w * r.h * (4 * 2 + 4)
  return { mode: '3d', errors, gpu, firstFrameMs: Math.round(info.firstFrame), fps: Math.round(r.fps * 10) / 10, p95FrameMs: Math.round(r.p95 * 10) / 10, points: r.points, drawn: r.drawn, canvas: `${r.w}×${r.h}`, gpuMB: Math.round(((buffers + framebuffer) / 2 ** 20) * 10) / 10 }
}

for (const [name, profile] of Object.entries(PROFILES)) {
  if (what === 'home') for (const base of args) console.log(name, base, JSON.stringify(await home(base, profile)))
  else if (what === 'scene') console.log(name, JSON.stringify(await scene(args[0], args[1], profile)))
}
