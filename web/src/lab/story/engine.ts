// The story engine (three.js, WebGL2): ONE cloud of particles that morphs on
// the GPU from shape to shape as the visitor scrolls, with soft additive
// sprites for the glow, simulated depth of field (size and opacity by distance
// from the focus) and a camera that glides with the scroll. Imported
// dynamically, and only in 3D mode: nothing here is in the initial bundle.
//
// Shapes come from model.ts: static ones (the orbitals) live in buffers;
// the wave is computed in the vertex shader from a 512-texel field texture
// that the CPU refreshes from wave.ts (the tested physics) when t changes.
import {
  AdditiveBlending,
  BufferAttribute,
  BufferGeometry,
  Color,
  DataTexture,
  FloatType,
  GLSL3,
  NearestFilter,
  PerspectiveCamera,
  Points,
  RGFormat,
  Scene,
  ShaderMaterial,
  SRGBColorSpace,
  Vector3,
  WebGLRenderer,
} from 'three'
import { pixelRatio } from '../capability'
import { BACKGROUND, NEGATIVE, POSITIVE, RADIUS, SIZE } from '../view'
import { camPos, DOF, frameAt, shapeKey, STEPS, storyAlpha, WAVE, type Shape } from './model'
import type { StoryReply, StoryRequest } from './story.worker'
import { field, SAMPLES, X_RANGE } from './wave'

export interface Story {
  /** 0 … STEPS.length: integer part = step, fraction = how far into it. */
  setProgress(p: number): void
  start(): void
  stop(): void
  dispose(): void
}

export interface StoryOptions {
  points: number
  seed: number
  onReady: () => void
  onError: () => void
}

const f = (v: number) => v.toFixed(6)
const vertex = /* glsl */ `
in vec3 aPosB;
in float aHueA;
in float aHueB;
in vec4 aSeed;
uniform int uKindA;
uniform int uKindB;
uniform float uMix;
uniform float uScaleA;
uniform float uScaleB;
uniform float uSpin;
uniform sampler2D uField;
uniform float uSize;
uniform float uFocus;
uniform vec3 uPos;
uniform vec3 uNeg;
out vec3 vColor;
out float vAlpha;

// model.ts wavePoint(), in GLSL.
void wave(out vec3 p, out float hue, out float alpha) {
  float col = aSeed.x;
  vec2 f = texelFetch(uField, ivec2(int(col), 0), 0).rg;
  float x = (${f(-X_RANGE)} + ${f(2 * X_RANGE)} * (col + 0.5) / ${f(SAMPLES)}) * ${f(WAVE.xScale)};
  if (aSeed.z < 0.5) {
    float amp = length(f);
    p = vec3(x, aSeed.y * f.x * ${f(WAVE.amp)}, aSeed.y * f.y * ${f(WAVE.amp)} + aSeed.w);
    hue = amp > 0.0 ? 0.5 + 0.5 * f.x / amp : 0.5;
    alpha = min(1.0, amp);
  } else {
    float d = dot(f, f);
    p = vec3(x + aSeed.w, ${f(WAVE.base)} + aSeed.y * d * ${f(WAVE.dens)}, aSeed.w);
    hue = 0.85;
    alpha = min(1.0, d * 1.2);
  }
}

// An orbital: physics z up (three's y), spinning about it.
vec3 orbital(vec3 q, float scale) {
  vec3 p = vec3(q.x, q.z, -q.y) * scale;
  float c = cos(uSpin), s = sin(uSpin);
  return vec3(c * p.x + s * p.z, p.y, -s * p.x + c * p.z);
}

void main() {
  vec3 pa; float ha; float aa;
  vec3 pb; float hb; float ab;
  if (uKindA == 1) wave(pa, ha, aa); else { pa = orbital(position, uScaleA); ha = aHueA; aa = 1.0; }
  if (uKindB == 1) wave(pb, hb, ab); else { pb = orbital(aPosB, uScaleB); hb = aHueB; ab = 1.0; }
  vec3 p = mix(pa, pb, uMix);
  vec4 mv = modelViewMatrix * vec4(p, 1.0);
  float dist = -mv.z;
  float blur = 1.0 + ${f(DOF)} * abs(dist - uFocus);
  gl_Position = projectionMatrix * mv;
  gl_PointSize = uSize * blur / dist;
  vAlpha = mix(aa, ab, uMix) / (blur * blur);
  vColor = mix(uNeg, uPos, mix(ha, hb, uMix));
}`

const fragment = /* glsl */ `
uniform float uAlpha;
in vec3 vColor;
in float vAlpha;
// With GLSL3, three declares no fragment output: the shader does.
layout(location = 0) out highp vec4 fragColor;
void main() {
  vec2 c = gl_PointCoord * 2.0 - 1.0;
  float d = dot(c, c);
  if (d > 1.0) discard;
  // A soft sprite: bright core, gaussian halo.
  float g = exp(-d * 3.5);
  fragColor = vec4(vColor, g * vAlpha * uAlpha);
}`

const unit = (c: [number, number, number]) => c.map((v) => v / 255) as [number, number, number]

export function createStory(canvas: HTMLCanvasElement, opts: StoryOptions): Story {
  const renderer = new WebGLRenderer({ canvas, antialias: false, alpha: false, powerPreference: 'high-performance' })
  const dpr = pixelRatio(window.devicePixelRatio)
  renderer.setPixelRatio(dpr)
  renderer.setClearColor(new Color().setRGB(...unit(BACKGROUND), SRGBColorSpace), 1)
  let failed = false
  let drawnOnce = false
  renderer.debug.onShaderError = () => {
    if (failed) return
    failed = true
    stop()
    opts.onError()
  }

  const scene = new Scene()
  const camera = new PerspectiveCamera(38, 1, 0.1, 60)
  const fieldData = new Float32Array(SAMPLES * 2)
  const fieldTex = new DataTexture(fieldData, SAMPLES, 1, RGFormat, FloatType)
  fieldTex.minFilter = NearestFilter
  fieldTex.magFilter = NearestFilter

  const geometry = new BufferGeometry()
  const material = new ShaderMaterial({
    glslVersion: GLSL3,
    vertexShader: vertex,
    fragmentShader: fragment,
    transparent: true,
    depthWrite: false,
    blending: AdditiveBlending,
    uniforms: {
      uKindA: { value: 0 },
      uKindB: { value: 0 },
      uMix: { value: 0 },
      uScaleA: { value: 1 },
      uScaleB: { value: 1 },
      uSpin: { value: 0 },
      uField: { value: fieldTex },
      uSize: { value: 1 },
      uFocus: { value: 5 },
      uAlpha: { value: 0.3 },
      // Raw sRGB values: the shader writes them as they are (no colour management).
      uPos: { value: new Vector3(...unit(POSITIVE)) },
      uNeg: { value: new Vector3(...unit(NEGATIVE)) },
    },
  })
  const cloud = new Points(geometry, material)
  cloud.frustumCulled = false
  scene.add(cloud)

  const orbitals: { pos: BufferAttribute; hue: BufferAttribute; scale: number }[] = []
  let seeds: BufferAttribute | null = null
  let drawn = opts.points
  const setDrawn = (n: number) => {
    drawn = n
    geometry.setDrawRange(0, n)
    material.uniforms.uAlpha.value = storyAlpha(n)
    canvas.dataset.drawn = String(n)
  }
  canvas.dataset.points = String(opts.points)
  setDrawn(drawn)

  const worker = new Worker(new URL('./story.worker.ts', import.meta.url), { type: 'module' })
  worker.onmessage = (e: MessageEvent<StoryReply>) => {
    const m = e.data
    if (m.kind === 'orbital') orbitals[m.index] = { pos: new BufferAttribute(m.positions, 3), hue: new BufferAttribute(m.hues, 1), scale: RADIUS / m.r95 }
    else seeds = new BufferAttribute(m.seeds, 4)
    bound = ''
  }
  worker.postMessage({ count: opts.points, seed: opts.seed } satisfies StoryRequest)

  let progress = 0
  let bound = ''
  let lastT = Number.NaN
  /** Whether a shape's data has arrived from the worker. */
  const ready = (s: Shape) => (s.kind === 'orbital' ? !!orbitals[s.index] : !!seeds)
  const bind = (side: 'A' | 'B', s: Shape) => {
    const o = s.kind === 'orbital' ? orbitals[s.index] : orbitals[0]
    geometry.setAttribute(side === 'A' ? 'position' : 'aPosB', o.pos)
    geometry.setAttribute(side === 'A' ? 'aHueA' : 'aHueB', o.hue)
    material.uniforms[`uKind${side}`].value = s.kind === 'wave' ? 1 : 0
    material.uniforms[`uScale${side}`].value = o.scale
  }

  function apply(time: number) {
    if (!orbitals[0]) return false
    const fr = frameAt(progress)
    let { a, b, mix } = fr
    // Data still on its way: stay on the last shape that has arrived.
    if (!ready(b)) [b, mix] = [a, 0]
    if (!ready(a)) [a, b, mix] = [STEPS[0].shape, STEPS[0].shape, 0]
    // Rebind only when the pair of shapes (or the data that arrived) changes.
    const key = `${shapeKey(a)}${shapeKey(b)}${seeds ? 's' : ''}${orbitals.length}`
    if (key !== bound) {
      geometry.setAttribute('aSeed', seeds ?? new BufferAttribute(new Float32Array(drawn * 4), 4))
      bind('A', a)
      bind('B', b)
      bound = key
    }
    material.uniforms.uMix.value = mix
    if ((a.kind === 'wave' || b.kind === 'wave') && fr.t !== lastT) {
      field(fr.t, fieldData)
      fieldTex.needsUpdate = true
      lastT = fr.t
    }
    // Cinematic camera: the step's path, plus a slow orbit while on the atom.
    const atom = STEPS[fr.step].scene === 1 ? 1 - mix : 0
    material.uniforms.uSpin.value = time * 0.00012 * atom
    const cam = { ...fr.cam, az: fr.cam.az + 0.12 * Math.sin(time * 0.00009) }
    camera.position.set(...camPos(cam))
    camera.lookAt(...cam.target)
    if (camera.fov !== cam.fov) {
      camera.fov = cam.fov
      camera.updateProjectionMatrix()
    }
    material.uniforms.uFocus.value = cam.dist
    return true
  }

  let wide = false
  function resize() {
    const w = canvas.clientWidth
    const h = canvas.clientHeight
    if (!w || !h) return
    renderer.setSize(w, h, false)
    camera.aspect = w / h
    // Portrait: zoom out until the scene (the wave is the widest) fits the width.
    camera.zoom = Math.min(1, (w / h) * 0.9)
    // Wide screens: the text panels are on the left, so the scene moves right.
    wide = w > 900 && w / h > 1.25
    if (wide) camera.setViewOffset(w, h, -0.2 * w, 0, w, h)
    else camera.clearViewOffset()
    camera.updateProjectionMatrix()
    // Pixels at distance 5 = SIZE × the shorter side (as the prototype), scaled with the zoom.
    material.uniforms.uSize.value = SIZE * Math.min(w, h) * dpr * 5 * camera.zoom
  }
  const ro = new ResizeObserver(resize)
  ro.observe(canvas)
  resize()

  let raf = 0
  let running = false
  let last = 0
  let frames = 0
  let slowSum = 0
  let halvings = 0
  const frame = (t: number) => {
    raf = requestAnimationFrame(frame)
    const dt = last ? Math.min(t - last, 100) : 16
    last = t
    const ok = apply(t)
    renderer.render(scene, camera)
    if (failed || !ok) return
    if (!drawnOnce) {
      drawnOnce = true
      opts.onReady()
    }
    // Adaptive load: if the first frames are slow, halve the particles (twice at most).
    if (halvings < 2) {
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
      fieldTex.dispose()
      renderer.dispose()
    },
  }
}
