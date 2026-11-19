// The story engine (three.js, WebGL2): ONE cloud of particles that morphs on
// the GPU from shape to shape as the visitor scrolls, with soft additive
// sprites for the glow, simulated depth of field (size and opacity by distance
// from the focus), a burst when the scene changes and a camera that travels
// with the scroll. Imported dynamically, and only in 3D mode: nothing here is
// in the initial bundle.
//
// Shapes come from model.ts: static ones (the orbitals, the figures of scenes
// 3–5) live in buffers, each particle with a hue and a light weight; the wave
// is computed in the vertex shader from a 512-texel field texture that the CPU
// refreshes from wave.ts (the tested physics) when t changes. On the circuit's
// wires some particles flow (the shader moves them along x with the time).
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
  Matrix3,
  Matrix4,
  Quaternion,
  Vector2,
  Vector3,
  WebGLRenderer,
} from 'three'
import { pixelRatio } from '../capability'
import { drew, findProbe, note, ran } from '../probe'
import { BACKGROUND, NEGATIVE, POSITIVE, RADIUS } from '../view'
import { fit, type Box, type Frame } from './camera'
import { figureFamily, WIRE, type FigureId } from './figures'
import { DOF, fillBase, frameAt, MAX_BLUR, MAX_POINT, POINT, shapeBox, shapeKey, STEPS, storyAlpha, storySize, WAVE, WAVE_GAIN, WAVE_SIZE, type FrameState, type Shape } from './model'
import type { StoryReply, StoryRequest } from './story.worker'
import { field, SAMPLES, X_RANGE } from './wave'

export interface Story {
  /** 0 … STEPS.length: integer part = step, fraction = how far into it. */
  setProgress(p: number): void
  /** The free part of the canvas (CSS pixels, canvas coordinates): the shape is framed inside it. */
  setFrame(free: { left: number; top: number; right: number; bottom: number }): void
  /** Scene 6's sliders: x and y, in years. */
  setMosca(x: number, y: number): void
  start(): void
  stop(): void
  dispose(): void
}

export interface StoryOptions {
  points: number
  seed: number
  onReady: () => void
  onError: () => void
  /** What to draw at a progress (default: the story, model.ts frameAt). */
  script?: (p: number) => FrameState
  /** Build only these shapes (default: the whole story). */
  only?: StoryRequest['only']
  /** Ease the progress towards what was set (a damped scroll, with a speed streak) instead of following it exactly. */
  smooth?: boolean
  /** Strength of the electron-wave ripple (default 1; the brand glyph at rest uses little). */
  waveGain?: number
  /** Each dot quivers about its site by this much (world units; default 0). */
  jitter?: number
  /** Point size factor for figures (default 1). */
  pointScale?: number
  /** Called before each frame is drawn, with the time since the last one (ms; never more than 100). */
  onFrame?: (dt: number) => void
}

const f = (v: number) => v.toFixed(6)
/** One pass of the flowing particles along the circuit's wires (ms). */
const FLOW_MS = 5000
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
uniform float uTime;
uniform sampler2D uField;
uniform float uSize;
uniform float uMaxPx;
uniform float uFocus;
uniform float uBase;
uniform vec3 uPos;
uniform vec3 uNeg;
uniform vec2 uCur;      // pointer, in NDC
uniform vec2 uCurVel;   // its smoothed motion, NDC per frame
uniform float uCurAmt;  // 0 = pointer away … 1 = over the canvas
uniform float uKick;    // a click: 1, then fades out
uniform float uAspect;
uniform float uVelocity; // scroll speed, about -1.5 … 1.5
uniform float uSwirl;
uniform float uStagger;
uniform float uWaveGain;
uniform float uJitter;  // the brand glyph's quiver (world units; 0 in the story)
uniform float uFigScale; // point size factor for figures (the brand glyph's dots)
uniform mat3 uOrbRot;   // Zeeman: the orbital's axis turned towards B, precessing
uniform vec3 uB;        // direction of the pointer's magnetic field (world)
uniform float uLorentz; // 0 … 1 while the pointer is pressed
uniform vec2 uMosca;
uniform float uMoscaA;
uniform float uMoscaB;
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

// An orbital: physics z up (three's y), spinning about it; then turned by the pointer's
// magnetic field (Zeeman: its axis aligns with B and precesses about it, uOrbRot).
vec3 orbital(vec3 q, float scale) {
  vec3 p = vec3(q.x, q.z, -q.y) * scale;
  float c = cos(uSpin), s = sin(uSpin);
  return uOrbRot * vec3(c * p.x + s * p.z, p.y, -s * p.x + c * p.z);
}

// A figure (figures.ts): as built, in world units. A negative weight flows
// along the wire, from x0 to x1 and round again (one pass every FLOW_MS).
vec3 figure(vec3 q, vec2 look, float mosca, out float hue, out float alpha) {
  hue = look.x;
  alpha = abs(look.y);
  if (mosca > 0.5) {
    // Scene 6, driven by the sliders (years): x-bar and y-bar stretch, the marker of x + y
    // slides, and the survey's bars already reached by x + y light up.
    float x = uMosca.x;
    float y = uMosca.y;
    float tot = x + y;
    vec3 p = q;
    bool rowX = abs(q.y + 0.5) < 0.08;
    bool rowY = abs(q.y + 0.9) < 0.08;
    bool rowS = abs(q.y + 1.3) < 0.08;
    bool isY = abs(hue - 0.15) < 0.02;
    if ((rowX || rowS) && hue > 0.9) {
      // x: from the start, x years long.
      p.x = -3.0 + (q.x + 3.0) * x / 10.0;
      if (x < 0.4) alpha = 0.0;
    } else if (rowY && isY) {
      // y alone: from the start, y years long.
      p.x = -3.0 + (q.x + 3.0) * y / 5.0;
      if (y < 0.4) alpha = 0.0;
    } else if (rowS && isY) {
      // the total: y continues where x ends.
      p.x = -3.0 + 6.0 * x / 30.0 + (q.x + 1.0) * y / 5.0;
      if (y < 0.4) alpha = 0.0;
    } else if (abs(hue - 0.62) < 0.02) {
      p.x = q.x + 6.0 * (min(tot, 30.0) - 15.0) / 30.0;
      alpha *= 1.6;
    } else if (q.y > 0.3) {
      float years = (q.x + 3.0) * 5.0;
      alpha *= years <= tot + 0.01 ? 1.5 : 0.22;
    }
    return p;
  }
  if (look.y >= 0.0) return q;
  float u = fract((q.x - ${f(WIRE.x0)}) / ${f(WIRE.x1 - WIRE.x0)} + uTime / ${f(FLOW_MS)});
  return vec3(${f(WIRE.x0)} + u * ${f(WIRE.x1 - WIRE.x0)}, q.y, q.z);
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
  if (uKindA == 1) wave(pa, ha, aa); else if (uKindA == 2) pa = figure(position, aLookA, uMoscaA, ha, aa); else { pa = orbital(position, uScaleA); ha = aLookA.x; aa = aLookA.y; }
  if (uKindB == 1) wave(pb, hb, ab); else if (uKindB == 2) pb = figure(aPosB, aLookB, uMoscaB, hb, ab); else { pb = orbital(aPosB, uScaleB); hb = aLookB.x; ab = aLookB.y; }
  // Each particle leaves and arrives on its own clock (a wave through the cloud),
  // spiralling around the shape's axis while it travels, flying apart and back together.
  float rnd = h(91.7);
  float tl = clamp((uMix - rnd * uStagger) / (1.0 - uStagger), 0.0, 1.0);
  float t = tl * tl * (3.0 - 2.0 * tl);
  float mid = sin(3.14159265 * t);
  vec3 p = mix(pa, pb, t) + uBurst * mid * burstDir();
  // Quantum quiver (the brand glyph): each dot trembles softly about its site on its own
  // phase, and flickers a little. The phase comes from the site, so the particles stacked on
  // one dot move together and the dot stays round and sharp.
  vec3 site = t < 0.5 ? pa : pb;
  vec3 hq = fract(sin(vec3(dot(site, vec3(12.9898, 78.233, 37.719)), dot(site, vec3(39.346, 11.135, 83.155)), dot(site, vec3(73.156, 52.235, 9.151)))) * 43758.5453);
  float qphase = uTime * 6.2831853 * 2.0 / 5000.0 + hq.x * 6.2831853;
  p += (hq - 0.5) * 2.0 * uJitter * vec3(sin(qphase), sin(qphase * 1.5 + hq.y * 6.2831853), sin(qphase + hq.z * 6.2831853));
  float flick = uJitter > 0.0 ? 0.78 + 0.22 * sin(uTime * 6.2831853 * 3.0 / 5000.0 + hq.y * 6.2831853) : 1.0;
  float sw = mid * uSwirl * (0.4 + rnd);
  float cs = cos(sw), sn = sin(sw);
  p = vec3(cs * p.x + sn * p.z, p.y, -sn * p.x + cs * p.z);
  // A wave of electrons: a radial ripple that crosses the whole figure, stronger the faster you scroll
  // and mid-morph; every particle also shimmers on its own phase. (Phases are whole turns per FLOW_MS: no jump when the clock wraps.)
  // Full on the orbitals; the figures and the wave only breathe (they read as shapes),
  // and scrolling does not shake them.
  float orb = mix(uKindA == 0 ? 1.0 : 0.0, uKindB == 0 ? 1.0 : 0.0, t);
  float calm = mix(0.1, 1.0, orb);
  float motion = min(1.0, abs(uVelocity) * orb + mid * 0.8);
  float r0 = length(p) + 0.0001;
  float ph = r0 * 3.2 - uTime * 6.2831853 * 3.0 / 5000.0;
  float amp = (0.035 + 0.16 * motion) * uWaveGain * calm;
  p += (p / r0) * amp * sin(ph) + burstDir() * 0.03 * uWaveGain * calm * sin(uTime * 6.2831853 * 4.0 / 5000.0 + rnd * 40.0) * (0.4 + motion);
  p += vec3(0.0, 0.5 * amp * sin(ph * 0.7 + rnd * 6.2831853), 0.0);
  // Pressed: the Lorentz force curls each electron into a small helix about the field line.
  float lz = uLorentz * orb;
  if (lz > 0.001) {
    vec3 e1 = normalize(cross(uB, abs(uB.y) < 0.9 ? vec3(0.0, 1.0, 0.0) : vec3(1.0, 0.0, 0.0)));
    vec3 e2 = cross(uB, e1);
    float wc = uTime * 6.2831853 * 6.0 / 5000.0 + rnd * 6.2831853;
    p += (e1 * cos(wc) + e2 * sin(wc)) * 0.32 * (0.35 + rnd) * lz + uB * sin(wc * 0.5 + rnd * 3.0) * 0.22 * lz;
  }
  vec4 mv = modelViewMatrix * vec4(p, 1.0);
  // Scroll streak (orbitals only: on a thin curve or a figure it reads as shaking).
  mv.y += uVelocity * (0.12 + rnd * 0.9) * orb;
  float dist = -mv.z;
  // Orbitals: depth of field, normal points. The wave: crisp and fine.
  float isWave = mix(uKindA == 1 ? 1.0 : 0.0, uKindB == 1 ? 1.0 : 0.0, t);
  float blur = 1.0 + ${f(DOF)} * (1.0 - isWave) * abs(dist - uFocus);
  float isFig = mix(uKindA == 2 ? 1.0 : 0.0, uKindB == 2 ? 1.0 : 0.0, t);
  float size = mix(1.0, ${f(WAVE_SIZE)}, isWave) * mix(1.0, uFigScale, isFig);
  float gain = mix(1.0, ${f(WAVE_GAIN)}, isWave);
  gl_Position = projectionMatrix * mv;
  // Pointer field (the whole scene): the points part around the pointer like a
  // force of repulsion, are swept along with its motion, and a click blows them away.
  vec2 ndc = gl_Position.xy / gl_Position.w;
  vec2 dd = ndc - uCur;
  dd.x *= uAspect;
  float dl = length(dd);
  // While scrolling the pointer lets go (the shape changes under it: twisting it then reads as shaking).
  float curK = uCurAmt * (1.0 - min(1.0, abs(uVelocity) * 3.0));
  float field = exp(-dl * dl / 0.03) * curK;
  vec2 dirA = dd / (dl + 0.0001);
  vec2 dir = vec2(dirA.x / uAspect, dirA.y);
  float speed = min(1.0, length(uCurVel) * 25.0);
  // The pointer is a charge that twists the cloud around it: a rotation whose angle fades
  // with distance. A rotation keeps the density (no empty hole), and it is zero far away.
  // Gentle while the pointer rests, a swirl while it moves; softer on thin curves and figures.
  float ang = (0.25 + 1.0 * speed) * field * mix(0.4, 1.0, orb);
  float ca = cos(ang), sa = sin(ang);
  vec2 tw = vec2(ca * dd.x - sa * dd.y, sa * dd.x + ca * dd.y);
  ndc = uCur + vec2(tw.x / uAspect, tw.y);
  // A click sends a ring of repulsion outwards (zero at the centre: no hole either).
  float ring = (1.0 - uKick) * 1.5;
  float rk = exp(-pow((dl - ring) / 0.09, 2.0)) * uKick * (1.0 - orb); // orbitals: the Lorentz helices instead
  // The wake: rings that leave the pointer while it moves and fade with distance.
  float wake = exp(-dl * 3.0) * speed * sin(dl * 30.0 - uTime * 6.2831853 * 4.0 / 5000.0) * curK;
  ndc += uCurVel * field * 0.5 + dir * (rk * 0.07 + wake * 0.012);
  gl_Position.xy = ndc * gl_Position.w;
  // model.ts pointPx(): the blur grows a point up to MAX_BLUR, and no point is wider than uMaxPx.
  gl_PointSize = max(min(uSize * size * min(blur, ${f(MAX_BLUR)}) / dist, uMaxPx), 2.0);
  vAlpha = mix(aa, ab, t) * gain / (blur * blur) * (1.0 + field * 0.8 + 0.5 * motion * (0.5 + 0.5 * sin(ph)) + 0.6 * lz) * flick;
  vColor = mix(uNeg, uPos, mix(ha, hb, t));
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
  float g = min(1.0, exp(-d * 7.0) * 1.5);
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
  // Only when a check has installed it (lab/probe.ts): the frames drawn and whether the loop runs.
  const probe = findProbe()
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
      uTime: { value: 0 },
      uField: { value: fieldTex },
      uSize: { value: 1 },
      uMaxPx: { value: 64 },
      uFocus: { value: 5 },
      uBase: { value: -2 },
      uAlpha: { value: 0.05 },
      // Raw sRGB values: the shader writes them as they are (no colour management).
      uPos: { value: new Vector3(...unit(POSITIVE)) },
      uNeg: { value: new Vector3(...unit(NEGATIVE)) },
      uCur: { value: new Vector2(2, 2) },
      uCurVel: { value: new Vector2(0, 0) },
      uCurAmt: { value: 0 },
      uKick: { value: 0 },
      uAspect: { value: 1 },
      uVelocity: { value: 0 },
      uSwirl: { value: 1.4 },
      uStagger: { value: 0.35 },
      uOrbRot: { value: new Matrix3() },
      uB: { value: new Vector3(0, 1, 0) },
      uLorentz: { value: 0 },
      uWaveGain: { value: opts.waveGain ?? 1 },
      uJitter: { value: opts.jitter ?? 0 },
      uFigScale: { value: opts.pointScale ?? 1 },
      uMosca: { value: new Vector2(10, 5) },
      uMoscaA: { value: 0 },
      uMoscaB: { value: 0 },
    },
  })
  const cloud = new Points(geometry, material)
  cloud.frustumCulled = false
  scene.add(cloud)

  const orbitals: { pos: BufferAttribute; look: BufferAttribute; scale: number }[] = []
  const figures = new Map<FigureId, { pos: BufferAttribute; look: BufferAttribute; scale: number }>()
  let seeds: BufferAttribute | null = null
  let drawn = opts.points
  canvas.dataset.points = String(opts.points)

  const worker = new Worker(new URL('./story.worker.ts', import.meta.url), { type: 'module' })
  worker.onmessage = (e: MessageEvent<StoryReply>) => {
    const m = e.data
    if (m.kind === 'orbital') orbitals[m.index] = { pos: new BufferAttribute(m.positions, 3), look: new BufferAttribute(m.looks, 2), scale: RADIUS / m.r95 }
    else if (m.kind === 'figure') figures.set(m.id, { pos: new BufferAttribute(m.positions, 3), look: new BufferAttribute(m.looks, 2), scale: 1 })
    else seeds = new BufferAttribute(m.seeds, 4)
    bound = ''
  }
  worker.postMessage({ count: opts.points, seed: opts.seed, only: opts.only } satisfies StoryRequest)
  const script = opts.script ?? frameAt

  let progress = 0
  let shown = 0
  let firstProgress = true
  // The scene's own clock (ms): it only runs while frames are drawn, so the
  // spin, the sway and the flow pick up where they were after a pause (the
  // tab hidden, the scene off screen) instead of jumping ahead.
  let clock = 0
  let bound = ''
  let lastT = Number.NaN
  /** Whether a shape's data has arrived from the worker. */
  const ready = (s: Shape) => (s.kind === 'orbital' ? !!orbitals[s.index] : s.kind === 'figure' ? figures.has(s.id) : !!seeds)
  const bind = (side: 'A' | 'B', s: Shape) => {
    const o = s.kind === 'orbital' ? orbitals[s.index] : s.kind === 'figure' ? figures.get(s.id)! : orbitals[0]
    geometry.setAttribute(side === 'A' ? 'position' : 'aPosB', o.pos)
    geometry.setAttribute(side === 'A' ? 'aLookA' : 'aLookB', o.look)
    material.uniforms[`uKind${side}`].value = s.kind === 'wave' ? 1 : s.kind === 'figure' ? 2 : 0
    material.uniforms[`uScale${side}`].value = o.scale
    material.uniforms[`uMosca${side}`].value = s.kind === 'figure' && s.id === 'mosca' ? 1 : 0
  }
  // Scene 6's sliders (years): the drawing eases towards them.
  const moscaTarget = new Vector2(10, 5)

  // The pointer, over the whole canvas (the stage ignores pointer events: the window listens).
  const curTarget = new Vector2(2, 2)
  let curAmtTarget = 0
  let pressed = false
  // The pointer as a magnet (orbitals): field direction, Zeeman strength and precession phase.
  const bDir = new Vector3(0, 1, 0)
  let zeeman = 0
  let larmor = 0
  let lastApply = Number.NaN
  const tmp = { c: new Vector3(), r: new Vector3(), u: new Vector3(), b: new Vector3(), qa: new Quaternion(), qp: new Quaternion(), qi: new Quaternion(), m: new Matrix4() }
  const Y_AXIS = new Vector3(0, 1, 0)
  const onPointer = (e: PointerEvent) => {
    const r = canvas.getBoundingClientRect()
    if (!r.width || !r.height) return
    const x = ((e.clientX - r.left) / r.width) * 2 - 1
    const y = -(((e.clientY - r.top) / r.height) * 2 - 1)
    curTarget.set(x, y)
    curAmtTarget = x >= -1 && x <= 1 && y >= -1 && y <= 1 ? 1 : 0
    if (e.type === 'pointerdown' && curAmtTarget) {
      material.uniforms.uKick.value = 1
      pressed = true
    }
    if (e.type === 'pointerup') {
      pressed = false
      if (e.pointerType !== 'mouse') curAmtTarget = 0
    }
  }
  const onLeave = () => {
    curAmtTarget = 0
  }
  window.addEventListener('pointermove', onPointer, { passive: true })
  window.addEventListener('pointerdown', onPointer, { passive: true })
  window.addEventListener('pointerup', onPointer, { passive: true })
  document.documentElement.addEventListener('mouseleave', onLeave)

  function apply(time: number) {
    if (!orbitals[0]) return false
    const fr = script(opts.smooth ? shown : progress)
    let { a, b, mix, burst } = fr
    // Data still on its way: stay on the last shape that has arrived.
    if (!ready(b)) [b, mix, burst] = [a, 0, 0]
    if (!ready(a)) [a, b, mix, burst] = [STEPS[0].shape, STEPS[0].shape, 0, 0]
    // Rebind only when the pair of shapes (or the data that arrived) changes.
    const key = `${shapeKey(a)}${shapeKey(b)}${seeds ? 's' : ''}${orbitals.length}.${figures.size}`
    if (key !== bound) {
      geometry.setAttribute('aSeed', seeds ?? new BufferAttribute(new Float32Array(drawn * 4), 4))
      bind('A', a)
      bind('B', b)
      bound = key
    }
    // Within one family (the three Bloch spheres, or a shape with itself) nothing flies
    // apart or spirals: only what differs moves (the vector).
    const family = (s: Shape) => (s.kind === 'figure' ? `f${figureFamily(s.id)}` : shapeKey(s))
    const same = family(a) === family(b)
    material.uniforms.uMix.value = mix
    material.uniforms.uBurst.value = same ? 0 : burst
    material.uniforms.uSwirl.value = same ? 0 : 1.4
    material.uniforms.uStagger.value = same ? 0 : 0.35
    // At rest the wave keeps evolving a little (its own time breathes around the scrolled one).
    const tw = fr.t + 0.05 * Math.sin(time * 0.0009)
    if ((a.kind === 'wave' || b.kind === 'wave') && tw !== lastT) {
      field(tw, fieldData)
      fieldTex.needsUpdate = true
      material.uniforms.uBase.value = fillBase(fieldData)
      lastT = tw
    }
    // The step's camera path, plus a slow sway; the atom spins slowly. The
    // camera frames the shape (between A and B during a morph) whole, centred
    // in the free part of the screen (camera.ts fit).
    const atom = fr.spin ?? (STEPS[fr.step].scene === 1 ? 1 - mix : 0)
    material.uniforms.uSpin.value = time * 0.00012 * atom
    material.uniforms.uAlpha.value = storyAlpha(drawn) * (fr.gain ?? 1)
    material.uniforms.uTime.value = time % FLOW_MS // small: float32 keeps its precision
    const box = lerpBox(shapeBox(a, fieldData), shapeBox(b, fieldData), mix)
    const shot = fit(box, fr.cam.az + 0.08 * Math.sin(time * 0.00009), fr.cam.el, fr.cam.frame, fr.cam.fov, frameNow())
    camera.position.set(...shot.position)
    camera.lookAt(...shot.target)
    camera.fov = fr.cam.fov
    camera.setViewOffset(area.width, area.height, -shot.shift[0], -shot.shift[1], area.width, area.height)
    camera.updateProjectionMatrix()
    material.uniforms.uFocus.value = shot.distance

    // The pointer as a magnet. B points from the orbital's centre (the origin) towards the
    // pointer on screen, tilted a little towards the camera. Hovering near it, the orbital's
    // axis aligns with B and precesses about it (Zeeman, Larmor precession, faster when closer);
    // away, it eases back. The 1s is spherical: turning it changes nothing, as in nature.
    const dtA = Number.isNaN(lastApply) ? 16 : Math.min(100, Math.max(0, time - lastApply))
    lastApply = time
    const u = material.uniforms
    const cur = u.uCur.value as Vector2
    const c0 = tmp.c.set(0, 0, 0).project(camera)
    const dx = (cur.x - c0.x) * camera.aspect
    const dy = cur.y - c0.y
    camera.updateMatrixWorld()
    tmp.r.setFromMatrixColumn(camera.matrixWorld, 0)
    tmp.u.setFromMatrixColumn(camera.matrixWorld, 1)
    tmp.b.setFromMatrixColumn(camera.matrixWorld, 2)
    bDir.copy(tmp.r).multiplyScalar(dx).addScaledVector(tmp.u, dy).addScaledVector(tmp.b, 0.35).normalize()
    u.uB.value.copy(bDir)
    const near = Math.max(0, 1 - Math.hypot(dx, dy) / 1.4)
    const scrolling = Math.min(1, Math.abs(u.uVelocity.value) * 3)
    // How much of what is on screen is an orbital (during a step the next shape is already
    // bound as B with mix 0: the 4f, followed by the wave, must still count fully).
    const isOrb = (a.kind === 'orbital' ? 1 - mix : 0) + (b.kind === 'orbital' ? mix : 0)
    const main = mix < 0.5 ? a : b
    zeeman += (u.uCurAmt.value * near * (1 - scrolling) * isOrb - zeeman) * (1 - Math.exp(-dtA / 450))
    // Precession only shows (and only reads well) on orbitals symmetric about their axis
    // (2pz, 3dz2): the 4fxyz has no such axis, and spinning it looked like it ran off on its own.
    const axial = main.kind === 'orbital' && main.index !== 3 ? 1 : 0
    larmor = (larmor + (dtA / 1000) * (0.6 + 2.4 * near) * zeeman * axial) % (Math.PI * 2)
    tmp.qa.setFromUnitVectors(Y_AXIS, bDir)
    tmp.qp.setFromAxisAngle(bDir, larmor).multiply(tmp.qa)
    tmp.qi.identity().slerp(tmp.qp, zeeman)
    u.uOrbRot.value.setFromMatrix4(tmp.m.makeRotationFromQuaternion(tmp.qi))
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
    material.uniforms.uMaxPx.value = MAX_POINT * h * dpr
    // Resizing clears the canvas: while the loop is stopped (a scene at rest,
    // or paused), draw the same moment again so it never goes blank.
    if (drawnOnce && !running && !failed && apply(clock)) {
      renderer.render(scene, camera)
      drew(probe)
    }
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
    clock += dt
    material.uniforms.uMosca.value.lerp(moscaTarget, 1 - Math.exp(-dt / 120))
    if (opts.smooth) {
      const before = shown
      shown += (progress - shown) * (1 - Math.exp(-dt / 150))
      if (Math.abs(progress - shown) < 1e-4) shown = progress
      // Steps per second, smoothed: the streak and the wave follow how fast you scroll.
      const v = ((shown - before) / Math.max(dt, 1)) * 1000
      const u = material.uniforms.uVelocity
      u.value += (Math.max(-1.5, Math.min(1.5, v * 0.5)) - u.value) * 0.2
    }
    {
      const u = material.uniforms
      const cur: Vector2 = u.uCur.value
      if (u.uCurAmt.value < 0.02 && curAmtTarget) cur.copy(curTarget)
      const px = cur.x
      const py = cur.y
      cur.lerp(curTarget, 1 - Math.exp(-dt / 60))
      const vel: Vector2 = u.uCurVel.value
      vel.x += ((cur.x - px) * 1.2 - vel.x) * 0.25
      vel.y += ((cur.y - py) * 1.2 - vel.y) * 0.25
      u.uCurAmt.value += (curAmtTarget - u.uCurAmt.value) * (1 - Math.exp(-dt / 140))
      u.uKick.value *= Math.exp(-dt / 420)
      u.uLorentz.value += ((pressed && curAmtTarget ? 1 : 0) - u.uLorentz.value) * (1 - Math.exp(-dt / 250))
      u.uAspect.value = area.width / Math.max(1, area.height)
    }
    opts.onFrame?.(dt)
    const ok = apply(clock)
    renderer.render(scene, camera)
    if (failed || !ok) return
    drew(probe)
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
    ran(probe, true)
    last = 0
    raf = requestAnimationFrame(frame)
  }
  const stop = () => {
    if (running) ran(probe, false)
    running = false
    cancelAnimationFrame(raf)
  }
  let wanted = false
  const onVisibility = () => {
    note(probe, `visibilitychange: ${document.visibilityState}`)
    if (document.hidden) stop()
    else if (wanted) start()
  }
  document.addEventListener('visibilitychange', onVisibility)

  return {
    setProgress(p) {
      progress = p
      if (firstProgress) {
        firstProgress = false
        shown = p
      }
    },
    setFrame(f) {
      free = f
    },
    setMosca(x, y) {
      moscaTarget.set(x, y)
    },
    start() {
      if (!wanted) note(probe, document.hidden ? 'asked to run (tab hidden: waiting)' : 'asked to run')
      wanted = true
      start()
    },
    stop() {
      if (wanted) note(probe, 'asked to stop')
      wanted = false
      stop()
    },
    dispose() {
      stop()
      ro.disconnect()
      document.removeEventListener('visibilitychange', onVisibility)
      window.removeEventListener('pointermove', onPointer)
      window.removeEventListener('pointerdown', onPointer)
      window.removeEventListener('pointerup', onPointer)
      document.documentElement.removeEventListener('mouseleave', onLeave)
      worker.terminate()
      geometry.dispose()
      material.dispose()
      fieldTex.dispose()
      renderer.dispose()
    },
  }
}
