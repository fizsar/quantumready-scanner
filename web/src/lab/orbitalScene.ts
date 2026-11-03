// The 3D point cloud (three.js, WebGL2). Imported dynamically by the lab page
// only when chooseMode() says 3D: nothing here is in the initial bundle.
import { AdditiveBlending, BufferAttribute, BufferGeometry, Color, GLSL3, PerspectiveCamera, Points, Scene, ShaderMaterial, SRGBColorSpace, Vector3, WebGLRenderer } from 'three'
import { pixelRatio } from './capability'
import { ORBITALS } from './hydrogen'
import { alphaFor, BACKGROUND, CAMERA, NEGATIVE, POSITIVE, RADIUS, SIZE } from './view'
import type { SampleReply, SampleRequest } from './sampler.worker'

export interface OrbitalScene {
  /** 0 … ORBITALS.length − 1: integer = that orbital, in between = morphing to the next. */
  setProgress(p: number): void
  /** Render only while visible: the page calls these from an IntersectionObserver. */
  start(): void
  stop(): void
  dispose(): void
}

export interface SceneOptions {
  points: number
  seed: number
  /** The first orbital has been drawn. */
  onReady: () => void
  /** A shader did not compile (or WebGL failed): the page falls back to the still images. */
  onError: () => void
}

// The physics z axis (the axis of m = 0 orbitals) points up: three's y.
const vertex = /* glsl */ `
in vec3 aPosB;
in float aSignA;
in float aSignB;
uniform float uMix;
uniform float uScaleA;
uniform float uScaleB;
uniform float uSize;
out float vSign;
void main() {
  vec3 p = mix(position * uScaleA, aPosB * uScaleB, uMix);
  vec4 mv = modelViewMatrix * vec4(p.x, p.z, -p.y, 1.0);
  gl_Position = projectionMatrix * mv;
  gl_PointSize = uSize / -mv.z;
  vSign = mix(aSignA, aSignB, uMix);
}`

const fragment = /* glsl */ `
uniform vec3 uPos;
uniform vec3 uNeg;
uniform float uAlpha;
in float vSign;
// With GLSL3, three declares no fragment output: the shader does.
layout(location = 0) out highp vec4 fragColor;
void main() {
  vec2 c = gl_PointCoord * 2.0 - 1.0;
  float d = dot(c, c);
  if (d > 1.0) discard;
  fragColor = vec4(mix(uNeg, uPos, vSign * 0.5 + 0.5), (1.0 - d) * uAlpha);
}`

const unit = (c: [number, number, number]) => c.map((v) => v / 255) as [number, number, number]

export function createOrbitalScene(canvas: HTMLCanvasElement, opts: SceneOptions): OrbitalScene {
  const renderer = new WebGLRenderer({ canvas, antialias: false, alpha: false, powerPreference: 'high-performance' })
  const dpr = pixelRatio(window.devicePixelRatio)
  renderer.setPixelRatio(dpr)
  // three's Color is linear unless told otherwise: --bg is an sRGB colour.
  renderer.setClearColor(new Color().setRGB(...unit(BACKGROUND), SRGBColorSpace), 1)
  // A shader that does not compile renders nothing, silently: report it instead.
  let failed = false
  let drawnOnce = false
  renderer.debug.onShaderError = () => {
    if (failed) return
    failed = true
    stop()
    opts.onError()
  }

  const scene = new Scene()
  const camera = new PerspectiveCamera(CAMERA.fov, 1, 0.1, 50)
  camera.position.set(0, 0, CAMERA.z)

  const geometry = new BufferGeometry()
  const material = new ShaderMaterial({
    glslVersion: GLSL3,
    vertexShader: vertex,
    fragmentShader: fragment,
    transparent: true,
    depthWrite: false,
    blending: AdditiveBlending,
    uniforms: {
      uMix: { value: 0 },
      uScaleA: { value: 1 },
      uScaleB: { value: 1 },
      uSize: { value: 1 },
      uAlpha: { value: 0.3 },
      // Raw sRGB values: the shader writes them as they are (no colour management).
      uPos: { value: new Vector3(...unit(POSITIVE)) },
      uNeg: { value: new Vector3(...unit(NEGATIVE)) },
    },
  })
  const cloud = new Points(geometry, material)
  cloud.frustumCulled = false
  cloud.rotation.x = CAMERA.tilt
  scene.add(cloud)

  const loaded: { pos: BufferAttribute; sign: BufferAttribute; scale: number }[] = []
  let progress = 0
  let pair = -1
  let drawn = opts.points
  // Points are in random order, so the first k are a uniform subsample.
  const setDrawn = (n: number) => {
    drawn = n
    geometry.setDrawRange(0, n)
    material.uniforms.uAlpha.value = alphaFor(n)
    canvas.dataset.drawn = String(n)
  }
  canvas.dataset.points = String(opts.points)
  setDrawn(drawn)

  const worker = new Worker(new URL('./sampler.worker.ts', import.meta.url), { type: 'module' })
  worker.onmessage = (e: MessageEvent<SampleReply>) => {
    const { index, positions, signs, r95 } = e.data
    loaded[index] = { pos: new BufferAttribute(positions, 3), sign: new BufferAttribute(signs, 1), scale: RADIUS / r95 }
    if (index === 0) apply()
  }
  worker.postMessage({ count: opts.points, seed: opts.seed } satisfies SampleRequest)

  // Shows orbital i morphing into i+1: the first half of each step holds, the second morphs.
  function apply() {
    if (!loaded[0]) return
    const last = ORBITALS.length - 1
    let i = Math.min(Math.floor(progress), last)
    let f = progress - i
    while (i > 0 && !loaded[i]) [i, f] = [i - 1, 1]
    const j = Math.min(i + 1, last)
    const mix = loaded[j] && j !== i ? Math.min(1, Math.max(0, (f - 0.5) * 2)) : 0
    const eased = mix * mix * (3 - 2 * mix)
    if (pair !== i) {
      const a = loaded[i]
      const b = loaded[j] ?? a
      geometry.setAttribute('position', a.pos)
      geometry.setAttribute('aSignA', a.sign)
      geometry.setAttribute('aPosB', b.pos)
      geometry.setAttribute('aSignB', b.sign)
      material.uniforms.uScaleA.value = a.scale
      material.uniforms.uScaleB.value = b.scale
      pair = loaded[j] ? i : -1 // re-pair when the next orbital arrives
    }
    material.uniforms.uMix.value = eased
  }

  function resize() {
    const w = canvas.clientWidth
    const h = canvas.clientHeight
    if (!w || !h) return
    renderer.setSize(w, h, false)
    camera.aspect = w / h
    // Portrait screens: move back until the cloud (r95 plus a margin) fits the width;
    // points scale with the shorter side, so they keep their size relative to the cloud.
    const fit = (RADIUS * 1.2) / (Math.tan(((CAMERA.fov / 2) * Math.PI) / 180) * camera.aspect)
    camera.position.z = Math.max(CAMERA.z, fit)
    camera.updateProjectionMatrix()
    material.uniforms.uSize.value = SIZE * Math.min(w, h) * dpr * (camera.position.z / CAMERA.z)
  }
  const ro = new ResizeObserver(resize)
  ro.observe(canvas)
  resize()

  let raf = 0
  let running = false
  let last = 0
  // Adaptive load: if the first frames are slow, halve the points (twice at most).
  let frames = 0
  let slowSum = 0
  let halvings = 0
  const frame = (t: number) => {
    raf = requestAnimationFrame(frame)
    const dt = last ? Math.min(t - last, 100) : 16
    last = t
    cloud.rotation.y += dt * 0.00018
    apply()
    renderer.render(scene, camera)
    if (failed) return
    if (!drawnOnce && loaded[0]) {
      drawnOnce = true
      opts.onReady()
    }
    if (halvings < 2 && loaded[0]) {
      if (++frames > 15) slowSum += dt
      if (frames === 75) {
        if (slowSum / 60 > 24 && drawn > 10_000) {
          setDrawn(Math.floor(drawn / 2))
          halvings++
        }
        frames = 0
        slowSum = 0
      }
    }
  }
  const start = () => {
    if (running || failed || document.hidden) return
    running = true
    last = 0
    raf = requestAnimationFrame(frame)
  }
  const stop = () => {
    running = false
    cancelAnimationFrame(raf)
  }
  let wanted = false
  const onVisibility = () => (document.hidden ? stop() : wanted && start())
  document.addEventListener('visibilitychange', onVisibility)

  return {
    setProgress(p) {
      progress = p
    },
    start() {
      wanted = true
      start()
    },
    stop() {
      wanted = false
      stop()
    },
    dispose() {
      stop()
      ro.disconnect()
      document.removeEventListener('visibilitychange', onVisibility)
      worker.terminate()
      geometry.dispose()
      material.dispose()
      renderer.dispose()
    },
  }
}
