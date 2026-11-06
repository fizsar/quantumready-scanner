// The story engine (three.js, WebGL2): ONE cloud of particles that morphs on
// the GPU from shape to shape as the visitor scrolls, with soft additive
// sprites for the glow, simulated depth of field (size and opacity by distance
// from the focus), a burst when the scene changes and a camera that travels
// with the scroll. Imported dynamically, and only in 3D mode: nothing here is
// in the initial bundle.
//
// Shapes come from model.ts: static ones (the orbitals) live in buffers, each
// particle with a hue and a light weight; the wave is computed in the vertex
// shader from a 512-texel field texture that the CPU refreshes from wave.ts
// (the tested physics) when t changes.
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
import { BACKGROUND, NEGATIVE, POSITIVE, RADIUS } from '../view'
import { fit, type Box, type Frame } from './camera'
import { DOF, fillBase, frameAt, POINT, shapeBox, shapeKey, STEPS, storyAlpha, storySize, WAVE, WAVE_GAIN, WAVE_SIZE, type Shape } from './model'
import type { StoryReply, StoryRequest } from './story.worker'
import { field, SAMPLES, X_RANGE } from './wave'

export interface Story {
  /** 0 … STEPS.length: integer part = step, fraction = how far into it. */
  setProgress(p: number): void
  /** The free part of the canvas (CSS pixels, canvas coordinates): the shape is framed inside it. */
  setFrame(free: { left: number; top: number; right: number; bottom: number }): void
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
in vec2 aLookA;
in vec2 aLookB;
in vec4 aSeed;
uniform int uKindA;
uniform int uKindB;
uniform float uMix;
uniform float uBurst;
uniform float uScaleA;
uniform float uScaleB;
uniform float uSpin;
uniform sampler2D uField;
uniform float uSize;
uniform float uFocus;
uniform float uBase;
uniform vec3 uPos;
uniform vec3 uNeg;
out vec3 vColor;
out float vAlpha;

// model.ts wavePoint(), in GLSL.
void wave(out vec3 p, out float hue, out float alpha) {
  // model.ts fieldAt(): linear interpolation between the two nearest samples.
  float col = aSeed.x;
  int i0 = min(int(floor(col)), ${SAMPLES - 2});
  vec2 f = mix(texelFetch(uField, ivec2(i0, 0), 0).rg, texelFetch(uField, ivec2(i0 + 1, 0), 0).rg, col - float(i0));
  float x = (${f(-X_RANGE)} + ${f(2 * X_RANGE)} * (col + 0.5) / ${f(SAMPLES)}) * ${f(WAVE.xScale)};
  if (aSeed.z < 0.5) {
    float amp = length(f);
    float r = 1.0 - 0.05 * aSeed.y;
    p = vec3(x, r * f.x * ${f(WAVE.amp)}, r * f.y * ${f(WAVE.amp)} + aSeed.w);
    hue = amp > 0.0 ? 0.5 + 0.5 * f.x / amp : 0.5;
    alpha = min(1.0, amp);
  } else {
    float d = dot(f, f);
    p = vec3(x + aSeed.w, uBase + aSeed.y * d * ${f(WAVE.dens)}, aSeed.w);
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

// model.ts burstDir(), in GLSL.
float h(float k) { return fract(sin(float(gl_VertexID + 1) * k) * 43758.5453); }
vec3 burstDir() {
  float z = h(12.9898) * 2.0 - 1.0;
  float a = h(78.233) * 6.2831853;
  float r = sqrt(1.0 - z * z);
  return vec3(r * cos(a), z, r * sin(a)) * (0.5 + h(37.719));
}

void main() {
  vec3 pa; float ha; float aa;
  vec3 pb; float hb; float ab;
  if (uKindA == 1) wave(pa, ha, aa); else { pa = orbital(position, uScaleA); ha = aLookA.x; aa = aLookA.y; }
  if (uKindB == 1) wave(pb, hb, ab); else { pb = orbital(aPosB, uScaleB); hb = aLookB.x; ab = aLookB.y; }
  // Fly apart and back together during a morph.
  vec3 p = mix(pa, pb, uMix) + uBurst * sin(3.14159265 * uMix) * burstDir();
  vec4 mv = modelViewMatrix * vec4(p, 1.0);
  float dist = -mv.z;
  // Orbitals: depth of field, normal points. The wave: crisp and fine.
  float isWave = mix(float(uKindA), float(uKindB), uMix);
  float blur = 1.0 + ${f(DOF)} * (1.0 - isWave) * abs(dist - uFocus);
  float size = mix(1.0, ${f(WAVE_SIZE)}, isWave);
  float gain = mix(1.0, ${f(WAVE_GAIN)}, isWave);
  gl_Position = projectionMatrix * mv;
  gl_PointSize = uSize * size * blur / dist;
  vAlpha = mix(aa, ab, uMix) * gain / (blur * blur);
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
  fragColor = vec4(vColor, min(1.0, g * vAlpha * uAlpha));
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
  const camera = new PerspectiveCamera(38, 1, 0.1, 80)
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
      uBurst: { value: 0 },
      uScaleA: { value: 1 },
      uScaleB: { value: 1 },
      uSpin: { value: 0 },
      uField: { value: fieldTex },
      uSize: { value: 1 },
      uFocus: { value: 5 },
      uBase: { value: -2 },
      uAlpha: { value: 0.05 },
      // Raw sRGB values: the shader writes them as they are (no colour management).
      uPos: { value: new Vector3(...unit(POSITIVE)) },
      uNeg: { value: new Vector3(...unit(NEGATIVE)) },
    },
  })
  const cloud = new Points(geometry, material)
  cloud.frustumCulled = false
  scene.add(cloud)

  const orbitals: { pos: BufferAttribute; look: BufferAttribute; scale: number }[] = []
  let seeds: BufferAttribute | null = null
  let drawn = opts.points
  canvas.dataset.points = String(opts.points)

  const worker = new Worker(new URL('./story.worker.ts', import.meta.url), { type: 'module' })
  worker.onmessage = (e: MessageEvent<StoryReply>) => {
    const m = e.data
    if (m.kind === 'orbital') orbitals[m.index] = { pos: new BufferAttribute(m.positions, 3), look: new BufferAttribute(m.looks, 2), scale: RADIUS / m.r95 }
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
    geometry.setAttribute(side === 'A' ? 'aLookA' : 'aLookB', o.look)
    material.uniforms[`uKind${side}`].value = s.kind === 'wave' ? 1 : 0
    material.uniforms[`uScale${side}`].value = o.scale
  }

  function apply(time: number) {
    if (!orbitals[0]) return false
    const fr = frameAt(progress)
    let { a, b, mix, burst } = fr
    // Data still on its way: stay on the last shape that has arrived.
    if (!ready(b)) [b, mix, burst] = [a, 0, 0]
    if (!ready(a)) [a, b, mix, burst] = [STEPS[0].shape, STEPS[0].shape, 0, 0]
    // Rebind only when the pair of shapes (or the data that arrived) changes.
    const key = `${shapeKey(a)}${shapeKey(b)}${seeds ? 's' : ''}${orbitals.length}`
    if (key !== bound) {
      geometry.setAttribute('aSeed', seeds ?? new BufferAttribute(new Float32Array(drawn * 4), 4))
      bind('A', a)
      bind('B', b)
      bound = key
    }
    material.uniforms.uMix.value = mix
    material.uniforms.uBurst.value = burst
    if ((a.kind === 'wave' || b.kind === 'wave') && fr.t !== lastT) {
      field(fr.t, fieldData)
      fieldTex.needsUpdate = true
      material.uniforms.uBase.value = fillBase(fieldData)
      lastT = fr.t
    }
    // The step's camera path, plus a slow sway; the atom spins slowly. The
    // camera frames the shape (between A and B during a morph) whole, centred
    // in the free part of the screen (camera.ts fit).
    const atom = STEPS[fr.step].scene === 1 ? 1 - mix : 0
    material.uniforms.uSpin.value = time * 0.00012 * atom
    const box = lerpBox(shapeBox(a, fieldData), shapeBox(b, fieldData), mix)
    const shot = fit(box, fr.cam.az + 0.08 * Math.sin(time * 0.00009), fr.cam.el, fr.cam.frame, fr.cam.fov, frameNow())
    camera.position.set(...shot.position)
    camera.lookAt(...shot.target)
    camera.fov = fr.cam.fov
    camera.setViewOffset(area.width, area.height, -shot.shift[0], -shot.shift[1], area.width, area.height)
    camera.updateProjectionMatrix()
    material.uniforms.uFocus.value = shot.distance
    return true
  }

  // The drawing area and its free part, in CSS pixels.
  const area: Frame = { width: 1, height: 1, left: 0, top: 0, right: 1, bottom: 1 }
  let free: { left: number; top: number; right: number; bottom: number } | null = null
  const frameNow = () => {
    const f = free ?? { left: 0, top: 0, right: area.width, bottom: area.height }
    area.left = Math.max(0, f.left)
    area.top = Math.max(0, f.top)
    area.right = Math.min(area.width, f.right)
    area.bottom = Math.min(area.height, f.bottom)
    return area
  }
  const lerpBox = (p: Box, q: Box, t: number): Box => ({
    min: [0, 1, 2].map((k) => p.min[k] + (q.min[k] - p.min[k]) * t) as Box['min'],
    max: [0, 1, 2].map((k) => p.max[k] + (q.max[k] - p.max[k]) * t) as Box['max'],
  })

  function resize() {
    const w = canvas.clientWidth
    const h = canvas.clientHeight
    if (!w || !h) return
    renderer.setSize(w, h, false)
    camera.aspect = w / h
    area.width = w
    area.height = h
    // Points have a size in the world (POINT), drawn in drawing-buffer pixels:
    // diameter = POINT × focal length (buffer px) / depth. The light per pixel
    // then depends neither on the canvas size, nor on the DPR, nor on the distance.
    const focal = (h * dpr) / 2 / Math.tan(((camera.fov / 2) * Math.PI) / 180)
    material.uniforms.uSize.value = POINT * focal * storySize(drawn)
  }
  const setDrawn = (n: number) => {
    drawn = n
    geometry.setDrawRange(0, n)
    material.uniforms.uAlpha.value = storyAlpha(n)
    resize()
    canvas.dataset.drawn = String(n)
  }
  const ro = new ResizeObserver(resize)
  ro.observe(canvas)
  setDrawn(drawn)

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
    setFrame(f) {
      free = f
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
