import type { Lang } from '../../api'

export interface StoryText {
  title: string
  lead: string
  scenes: Record<1 | 2, string>
  steps: Record<string, { title: string; body: string }>
  legendAtom: [string, string]
  legendWave: [string, string]
  energy: string
  scale: (r: string) => string
  units: string
  still: Record<'reduced-motion' | 'no-webgl2' | 'save-data' | 'low-end', string>
  stillAlt: (step: string) => string
  sources: string
  method: string
}

export const TEXT: Record<Lang, StoryText> = {
  es: {
    title: 'De la mecánica cuántica a tu web · laboratorio',
    lead: 'Una sola nube de partículas que cambia de forma mientras bajas. Todo lo que dibuja está calculado con las ecuaciones de verdad.',
    scenes: { 1: 'Escena 1 · El átomo', 2: 'Escena 2 · La ecuación de Schrödinger' },
    steps: {
      '1s': { title: '1s', body: 'El estado fundamental del hidrógeno. Cada punto es un sitio donde podría estar el electrón, muestreado de |ψ|². Con l = 0 la nube es esférica y de una sola fase.' },
      '2pz': { title: '2p', body: 'Con l = 1 aparece un plano nodal (z = 0), donde ψ vale cero. Separa dos lóbulos de fase opuesta: verde e índigo.' },
      '3dz2': { title: '3d', body: 'Con l = 2, dos conos nodales (cos²θ = 1/3) separan los lóbulos del eje z del anillo, de fase contraria.' },
      '4fxyz': { title: '4f', body: 'Con l = 3, tres planos nodales: ψ es proporcional a xyz. Ocho lóbulos, uno por octante, alternando la fase.' },
      psi: { title: 'La función de onda ψ(x, t)', body: 'Un electrón libre no tiene una posición fija: lo describe una onda compleja. Aquí está en superposición de dos paquetes. La cinta es ψ en el plano complejo; debajo, |ψ|², la probabilidad de encontrarlo en cada x.' },
      dt: { title: 'iℏ ∂ψ/∂t: cómo cambia con el tiempo', body: 'La ecuación de Schrödinger da la evolución de ψ. La fase gira —el color pasa de verde a índigo— y los dos paquetes avanzan uno hacia el otro.' },
      kin: { title: '−(ℏ²/2m) ∂²ψ/∂x²: la energía cinética', body: 'Depende de la curvatura de ψ: cuanto más rápido oscila, más energía. Por este término los paquetes se ensanchan al avanzar.' },
      pot: { title: 'V(x) ψ = 0: la partícula libre', body: 'Sin fuerzas, el potencial es cero. La ecuación es lineal: la suma de dos soluciones también es solución.' },
      sup: { title: 'Superposición e interferencia', body: 'Al solaparse, |ψ₁ + ψ₂|² no es |ψ₁|² + |ψ₂|²: aparecen franjas separadas π/2. No son dos partículas chocando: es una sola, y se suman sus amplitudes.' },
    },
    legendAtom: ['verde: ψ > 0', 'índigo: ψ < 0'],
    legendWave: ['color: la fase de ψ', 'abajo: |ψ|²'],
    energy: 'energía',
    scale: (r) => `el 95 % de la probabilidad está dentro de r = ${r} a₀`,
    units: 'Unidades con ℏ = m = 1; σ = 1,5, k₀ = ±2, x₀ = ∓12.',
    still: {
      'reduced-motion': 'Imágenes fijas: tu sistema pide reducir el movimiento.',
      'no-webgl2': 'Imágenes fijas: este navegador no ofrece WebGL2 acelerado.',
      'save-data': 'Imágenes fijas: el modo de ahorro de datos está activado.',
      'low-end': 'Imágenes fijas para este dispositivo.',
    },
    stillAlt: (s) => `Escena fija: ${s}`,
    sources: 'Fuentes y método',
    method:
      'Escena 1: orbitales muestreados de |ψ_nlm|² = |R_nl|²|Y_lm|² (unidades atómicas, núcleo fijo). Escena 2: ψ = (ψ₁ + ψ₂)/√2, con cada ψ_j la solución exacta de la ecuación de Schrödinger libre para un paquete gaussiano. Los tests comprueban la ecuación por diferencias finitas, que ∫|ψ|² dx = 1 en todo instante, la velocidad de grupo, el ensanchamiento σ(t) = σ√(1 + (ℏt/2mσ²)²) y la separación de las franjas. Las imágenes fijas se generan al compilar con el mismo código.',
  },
  en: {
    title: 'From quantum mechanics to your website · lab',
    lead: 'A single cloud of particles that changes shape as you scroll. Everything it draws is computed from the real equations.',
    scenes: { 1: 'Scene 1 · The atom', 2: 'Scene 2 · The Schrödinger equation' },
    steps: {
      '1s': { title: '1s', body: 'The ground state of hydrogen. Each point is a place where the electron could be, sampled from |ψ|². With l = 0 the cloud is spherical and has a single phase.' },
      '2pz': { title: '2p', body: 'With l = 1 a nodal plane appears (z = 0), where ψ is zero. It separates two lobes of opposite phase: green and indigo.' },
      '3dz2': { title: '3d', body: 'With l = 2, two nodal cones (cos²θ = 1/3) separate the lobes along z from the ring, which has the opposite phase.' },
      '4fxyz': { title: '4f', body: 'With l = 3, three nodal planes: ψ is proportional to xyz. Eight lobes, one per octant, alternating in phase.' },
      psi: { title: 'The wave function ψ(x, t)', body: 'A free electron has no fixed position: a complex wave describes it. Here it is in a superposition of two packets. The ribbon is ψ in the complex plane; below, |ψ|², the probability of finding it at each x.' },
      dt: { title: 'iℏ ∂ψ/∂t: how it changes in time', body: 'The Schrödinger equation gives how ψ evolves. The phase turns (the colour goes from green to indigo) and the two packets move towards each other.' },
      kin: { title: '−(ℏ²/2m) ∂²ψ/∂x²: the kinetic energy', body: 'It depends on the curvature of ψ: the faster it oscillates, the more energy. This term makes the packets spread as they move.' },
      pot: { title: 'V(x) ψ = 0: the free particle', body: 'With no forces the potential is zero. The equation is linear: the sum of two solutions is a solution too.' },
      sup: { title: 'Superposition and interference', body: 'Where they overlap, |ψ₁ + ψ₂|² is not |ψ₁|² + |ψ₂|²: fringes appear, π/2 apart. These are not two particles colliding: it is one, and its amplitudes add.' },
    },
    legendAtom: ['green: ψ > 0', 'indigo: ψ < 0'],
    legendWave: ['colour: the phase of ψ', 'below: |ψ|²'],
    energy: 'energy',
    scale: (r) => `95% of the probability lies within r = ${r} a₀`,
    units: 'Units with ℏ = m = 1; σ = 1.5, k₀ = ±2, x₀ = ∓12.',
    still: {
      'reduced-motion': 'Still images: your system asks for reduced motion.',
      'no-webgl2': 'Still images: this browser has no accelerated WebGL2.',
      'save-data': 'Still images: data saver is on.',
      'low-end': 'Still images for this device.',
    },
    stillAlt: (s) => `Still scene: ${s}`,
    sources: 'Sources and method',
    method:
      'Scene 1: orbitals sampled from |ψ_nlm|² = |R_nl|²|Y_lm|² (atomic units, fixed nucleus). Scene 2: ψ = (ψ₁ + ψ₂)/√2, each ψ_j the exact solution of the free Schrödinger equation for a Gaussian packet. The tests check the equation by finite differences, that ∫|ψ|² dx = 1 at every time, the group velocity, the spreading σ(t) = σ√(1 + (ℏt/2mσ²)²) and the fringe spacing. The still images are generated at build time with the same code.',
  },
}
