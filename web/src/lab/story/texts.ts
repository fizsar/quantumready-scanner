import type { Lang } from '../../api'
import type { Scene } from './model'

export interface StoryText {
  title: string
  lead: string
  scenes: Record<Scene, string>
  steps: Record<string, { title: string; body: string }>
  /** Per scene: what green and indigo mean. */
  legend: Record<Scene, [string, string]>
  energy: string
  scale: (r: string) => string
  units: string
  still: Record<'reduced-motion' | 'no-webgl2' | 'save-data' | 'low-end', string>
  stillAlt: (step: string) => string
  sources: string
  method: string
  /** Scenes 3–5. */
  methodQuantum: string
}

export const TEXT: Record<Lang, StoryText> = {
  es: {
    title: 'De la mecánica cuántica a tu web · laboratorio',
    lead: 'Una sola nube de partículas que cambia de forma mientras bajas. Todo lo que dibuja está calculado con las ecuaciones de verdad.',
    scenes: {
      1: 'Escena 1 · El átomo',
      2: 'Escena 2 · La ecuación de Schrödinger',
      3: 'Escena 3 · El cúbit',
      4: 'Escena 4 · El ordenador cuántico',
      5: 'Escena 5 · El algoritmo de Shor',
    },
    steps: {
      '1s': { title: '1s', body: 'El estado fundamental. Cada punto es un lugar donde podría estar el electrón. Con l = 0, la nube es esférica.' },
      '2pz': { title: '2p', body: 'Con l = 1 aparece un plano nodal, z = 0, donde ψ es cero. Separa dos lóbulos de fase opuesta.' },
      '3dz2': { title: '3d', body: 'Con l = 2, dos conos nodales (cos²θ = 1/3) separan los lóbulos del eje z del anillo, de fase contraria.' },
      '4fxyz': { title: '4f', body: 'Con l = 3, tres planos nodales: ψ ∝ xyz. Ocho lóbulos, uno por octante, alternando la fase.' },
      psi: { title: 'La función de onda ψ(x, t)', body: 'Un electrón libre no tiene posición fija: lo describe una onda compleja, aquí en superposición de dos paquetes.' },
      dt: { title: 'iℏ ∂ψ/∂t: cómo cambia', body: 'La ecuación de Schrödinger da la evolución de ψ: la fase gira (el color) y los paquetes avanzan uno hacia el otro.' },
      kin: { title: '−(ℏ²/2m) ∂²ψ/∂x²: cinética', body: 'Depende de la curvatura de ψ: cuanto más rápido oscila, más energía. Por eso los paquetes se ensanchan.' },
      pot: { title: 'V(x) ψ = 0: partícula libre', body: 'Sin fuerzas, el potencial es cero. La ecuación es lineal: la suma de soluciones también es solución.' },
      sup: { title: 'Superposición e interferencia', body: 'Al solaparse, |ψ₁ + ψ₂|² ≠ |ψ₁|² + |ψ₂|²: franjas separadas π/2. Es una sola partícula; se suman sus amplitudes.' },
      q0: { title: 'Un cúbit: |0⟩', body: 'Un cúbit es un vector en la esfera de Bloch: |0⟩ apunta al polo norte y |1⟩ al sur. Entre ellos, cualquier superposición.' },
      qH: { title: 'La puerta H: superposición', body: 'La puerta de Hadamard lleva |0⟩ al ecuador, (|0⟩ + |1⟩)/√2. Al medir, 0 o 1 con la misma probabilidad.' },
      qM: { title: 'Medir: el colapso', body: 'Al medir, el estado salta a un polo: aquí salió 1. La superposición se pierde; repetirlo daría 0 la mitad de las veces.' },
      bell: { title: 'Entrelazamiento', body: 'Dos cúbits en (|00⟩ + |11⟩)/√2. Cada uno por separado no tiene dirección: su vector encoge al centro. Juntos, siempre coinciden.' },
      circ: { title: 'Un circuito', body: 'H pone el primer cúbit en superposición y CNOT invierte el segundo si el primero es 1. Sale el estado de Bell: 00 u 11.' },
      reg3: { title: '3 cúbits, 8 amplitudes', body: 'Una H en cada cúbit: el registro está en los 2³ = 8 estados a la vez. Cada barra es una amplitud.' },
      reg6: { title: '6 cúbits, 64 amplitudes', body: 'Cada cúbit más duplica los estados: 2⁶ = 64. Las barras bajan, porque la suma de sus cuadrados sigue siendo 1.' },
      reg10: { title: '10 cúbits, 1024 amplitudes', body: 'Medir da un solo resultado al azar. La ventaja no es probarlo todo a la vez: hace falta un algoritmo que haga interferir las amplitudes.' },
      shorF: { title: 'Shor: N = 15, a = 7', body: 'Factorizar N se reduce a hallar el periodo r de f(x) = 7ˣ mod 15: los valores se repiten cada r pasos.' },
      qft: { title: 'La transformada de Fourier cuántica', body: 'La QFT convierte el periodo en picos en los múltiplos de 2⁸/r. Medir da uno de ellos, y de él sale r.' },
      key: { title: 'Una clave RSA: N = p · q', body: 'RSA confía en que multiplicar es fácil y factorizar, difícil. Aquí N = 15; en RSA-2048, N tiene 617 cifras.' },
      factors: { title: '15 = 3 × 5', body: 'Como r = 4 es par, mcd(7² ± 1, 15) da los factores. Con ellos, la clave privada se calcula al instante: la clave está rota.' },
      curve: { title: 'X25519: una curva elíptica', body: 'ECDH (X25519) usa los puntos de esta curva sobre un cuerpo finito. Su problema, el logaritmo discreto, también lo resuelve Shor.' },
      broken: { title: 'Lo que haría falta', body: 'Para RSA-2048, Gidney (2025) estima menos de un millón de cúbits ruidosos durante menos de una semana. Por eso se migra ya a ML-KEM.' },
    },
    legend: {
      1: ['verde: ψ > 0', 'índigo: ψ < 0'],
      2: ['hélice = ψ, color = fase', 'debajo: |ψ|²'],
      3: ['verde: el estado', 'índigo: la esfera de Bloch'],
      4: ['verde: cúbits y amplitudes', 'índigo: hilos'],
      5: ['verde: valores y puntos', 'índigo: ejes'],
    },
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
    methodQuantum:
      'Escenas 3 a 5: vectores de estado exactos (sin atajos) con las puertas H y CNOT; el vector de Bloch de cada cúbit sale de su matriz densidad reducida (traza parcial), que en el estado de Bell es I/2: vector nulo y pureza ½. Shor con N = 15, a = 7 y un registro de n = 8 cúbits (N² ≤ 2⁸ < 2N²): la distribución tras la QFT se calcula exacta, r sale por fracciones continuas y los factores, con el mcd. La curva es la ecuación de X25519 (RFC 7748) sobre el cuerpo de 101 elementos en lugar de 2²⁵⁵ − 19, para que sus puntos se vean. Los tests comprueban cada número que se dibuja o se muestra.',
  },
  en: {
    title: 'From quantum mechanics to your website · lab',
    lead: 'A single cloud of particles that changes shape as you scroll. Everything it draws is computed from the real equations.',
    scenes: {
      1: 'Scene 1 · The atom',
      2: 'Scene 2 · The Schrödinger equation',
      3: 'Scene 3 · The qubit',
      4: 'Scene 4 · The quantum computer',
      5: 'Scene 5 · Shor’s algorithm',
    },
    steps: {
      '1s': { title: '1s', body: 'The ground state. Each point is a place the electron could be. With l = 0, the cloud is spherical.' },
      '2pz': { title: '2p', body: 'With l = 1 a nodal plane appears, z = 0, where ψ is zero. It splits two lobes of opposite phase.' },
      '3dz2': { title: '3d', body: 'With l = 2, two nodal cones (cos²θ = 1/3) part the lobes along z from the ring, of opposite phase.' },
      '4fxyz': { title: '4f', body: 'With l = 3, three nodal planes: ψ ∝ xyz. Eight lobes, one per octant, alternating in phase.' },
      psi: { title: 'The wave function ψ(x, t)', body: 'A free electron has no fixed position: a complex wave describes it, here a superposition of two packets.' },
      dt: { title: 'iℏ ∂ψ/∂t: change in time', body: 'The Schrödinger equation gives how ψ evolves: the phase turns (the colour) and the packets move towards each other.' },
      kin: { title: '−(ℏ²/2m) ∂²ψ/∂x²: kinetic', body: 'It depends on the curvature of ψ: the faster it oscillates, the more energy. It makes the packets spread.' },
      pot: { title: 'V(x) ψ = 0: free particle', body: 'No forces, so the potential is zero. The equation is linear: a sum of solutions is a solution too.' },
      sup: { title: 'Superposition, interference', body: 'Where they overlap, |ψ₁ + ψ₂|² ≠ |ψ₁|² + |ψ₂|²: fringes π/2 apart. It is one particle; its amplitudes add.' },
      q0: { title: 'A qubit: |0⟩', body: 'A qubit is a vector on the Bloch sphere: |0⟩ points to the north pole and |1⟩ to the south. Any superposition lies between.' },
      qH: { title: 'The H gate: superposition', body: 'The Hadamard gate takes |0⟩ to the equator, (|0⟩ + |1⟩)/√2. Measured, it gives 0 or 1 with equal probability.' },
      qM: { title: 'Measuring: the collapse', body: 'A measurement sends the state to a pole: here it gave 1. The superposition is gone; repeating it would give 0 half the time.' },
      bell: { title: 'Entanglement', body: 'Two qubits in (|00⟩ + |11⟩)/√2. Neither has a direction on its own: its vector shrinks to the centre. Together, they always agree.' },
      circ: { title: 'A circuit', body: 'H puts the first qubit in superposition and CNOT flips the second if the first is 1. Out comes the Bell state: 00 or 11.' },
      reg3: { title: '3 qubits, 8 amplitudes', body: 'An H on each qubit: the register is in all 2³ = 8 states at once. Each bar is an amplitude.' },
      reg6: { title: '6 qubits, 64 amplitudes', body: 'Each extra qubit doubles the states: 2⁶ = 64. The bars shrink, because the sum of their squares is still 1.' },
      reg10: { title: '10 qubits, 1024 amplitudes', body: 'A measurement gives one random result. The edge is not trying everything at once: it takes an algorithm that makes the amplitudes interfere.' },
      shorF: { title: 'Shor: N = 15, a = 7', body: 'Factoring N comes down to finding the period r of f(x) = 7ˣ mod 15: the values repeat every r steps.' },
      qft: { title: 'The quantum Fourier transform', body: 'The QFT turns the period into peaks at the multiples of 2⁸/r. A measurement gives one of them, and r follows from it.' },
      key: { title: 'An RSA key: N = p · q', body: 'RSA relies on multiplying being easy and factoring hard. Here N = 15; in RSA-2048, N has 617 digits.' },
      factors: { title: '15 = 3 × 5', body: 'As r = 4 is even, gcd(7² ± 1, 15) gives the factors. From them the private key follows at once: the key is broken.' },
      curve: { title: 'X25519: an elliptic curve', body: 'ECDH (X25519) uses the points of this curve over a finite field. Its hard problem, the discrete logarithm, falls to Shor too.' },
      broken: { title: 'What it would take', body: 'For RSA-2048, Gidney (2025) estimates under a million noisy qubits for under a week. That is why the move to ML-KEM starts now.' },
    },
    legend: {
      1: ['green: ψ > 0', 'indigo: ψ < 0'],
      2: ['helix = ψ, colour = phase', 'below: |ψ|²'],
      3: ['green: the state', 'indigo: the Bloch sphere'],
      4: ['green: qubits and amplitudes', 'indigo: wires'],
      5: ['green: values and points', 'indigo: axes'],
    },
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
    methodQuantum:
      'Scenes 3 to 5: exact state vectors (no shortcuts) with the H and CNOT gates; each qubit’s Bloch vector comes from its reduced density matrix (partial trace), which in the Bell state is I/2: zero vector, purity ½. Shor with N = 15, a = 7 and a register of n = 8 qubits (N² ≤ 2⁸ < 2N²): the distribution after the QFT is computed exactly, r follows from continued fractions and the factors from the gcd. The curve is X25519’s equation (RFC 7748) over the field of 101 elements instead of 2²⁵⁵ − 19, so that its points can be seen. The tests check every number that is drawn or shown.',
  },
}
