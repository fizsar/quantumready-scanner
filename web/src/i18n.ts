import { createContext, useContext } from 'react'
import type { Lang } from './api'

const STORAGE_KEY = 'qr.lang'

// UI strings. Finding texts, block labels and API error messages come from the
// API in the requested language; these are the interface's own words.
const dict = {
  es: {
    'meta.home': 'QuantumReady · Auditoría post-cuántica de tu web',
    'meta.scan': 'Resultado de {host} · QuantumReady',
    'meta.notFound': 'Página no encontrada · QuantumReady',
    'nav.lang': 'Idioma',
    'nav.home': 'Inicio',
    'nav.skip': 'Saltar al contenido',
    'home.title': '¿Está tu web preparada para la era cuántica?',
    'home.lead':
      'Comprobamos cómo cifra tu web las conexiones: si ya usa intercambio de claves post-cuántico (ML-KEM), qué versiones de TLS y cifrados acepta y qué certificado presenta. El resultado, en lenguaje claro.',
    'home.demo': 'Por ahora, el escáner solo está abierto a estos dominios de demostración:',
    'home.scanThis': 'Escanear {host}',
    'home.formLabel': 'Dominio',
    'home.placeholder': 'ejemplo.com',
    'home.submit': 'Escanear',
    'home.submitting': 'Enviando…',
    'home.loadingTargets': 'Cargando dominios…',
    'err.invalid_host': 'Ese dominio no parece válido. Escribe solo el nombre, por ejemplo fizsar.com.',
    'err.port_not_allowed': 'Por ahora solo se escanea el puerto 443 (HTTPS). Quita el puerto del dominio.',
    'err.host_not_allowed': 'Por ahora solo se pueden escanear los dominios de demostración de arriba.',
    'err.body_too_large': 'El texto es demasiado largo. Escribe solo el dominio.',
    'err.rate_limited': 'Has hecho muchos escaneos seguidos. Podrás volver a intentarlo en {seconds} segundos.',
    'err.rate_limited_nowait': 'Has hecho muchos escaneos seguidos. Espera un poco y vuelve a intentarlo.',
    'err.network': 'No hemos podido conectar con el servidor. Comprueba tu conexión e inténtalo de nuevo.',
    'err.not_found': 'No encontramos ese escaneo. Puede que el enlace esté mal copiado.',
    'err.internal': 'Algo ha fallado de nuestro lado. Inténtalo de nuevo en unos minutos.',
    'progress.queued': 'En cola: el escaneo empezará en unos segundos…',
    'progress.running': 'Escaneando {host}: probando versiones de TLS, cifrados y grupos post-cuánticos…',
    'progress.retrying': 'El servidor no respondió a tiempo. Lo volveremos a intentar enseguida…',
    'progress.reconnecting': 'Conexión perdida, reintentando…',
    'failed.title': 'No hemos podido completar el escaneo',
    'failed.again': 'Probar otra vez',
    'headline.broken': 'Tu web tiene un problema que hay que corregir ya: los navegadores pueden mostrar un aviso de seguridad.',
    'headline.pqcWeak': 'Tu web ya usa cifrado post-cuántico, pero acepta cifrados débiles que conviene retirar hoy.',
    'headline.pqc': 'Tu web ya usa cifrado post-cuántico híbrido: el tráfico de hoy seguirá protegido aunque lleguen los ordenadores cuánticos.',
    'headline.notPreferred': 'Tu servidor admite cifrado post-cuántico, pero los navegadores aún no lo usan: falta activarlo como preferido.',
    'headline.weak': 'Tu web tiene debilidades que conviene corregir hoy y todavía no está preparada para la era cuántica.',
    'headline.classical': 'Tu web es segura hoy, pero el tráfico que se capture ahora podría descifrarse en el futuro con un ordenador cuántico.',
    'grade.label': 'Nota',
    'grade.score': '{score} de 100 puntos',
    'grade.capped': 'La letra está limitada a {grade} por: {reasons}.',
    'pqc.yes': 'PQC: sí',
    'pqc.no': 'PQC: no',
    'pqc.group': 'Grupo negociado: {group}',
    'pqc.noGroup': 'Grupo negociado: clásico',
    'axis.quantum.title': 'Riesgo cuántico',
    'axis.quantum.help': 'Lo que un ordenador cuántico podría romper: el tráfico capturado hoy y, en el futuro, las firmas.',
    'axis.current.title': 'Riesgo actual',
    'axis.current.help': 'Debilidades que se pueden aprovechar hoy, sin ordenadores cuánticos.',
    'axis.level': 'Nivel: {level}',
    'finding.fix': 'Qué hacer',
    'finding.more': 'Ver detalles',
    'breakdown.title': 'Desglose de la nota',
    'breakdown.points': '{points} de {max}',
    'breakdown.floor': 'Ajuste: nada roto ni débil (mínimo D)',
    'breakdown.floorHelp': 'Una configuración clásica sin fallos nunca baja de 40 puntos.',
    'noteval.title': 'No evaluado',
    'noteval.lead':
      'Estas comprobaciones quedan fuera del alcance del escáner actual. No suman ni restan puntos; no significan que tu web falle en ellas.',
    'footer.scanned': 'Escaneado el {date}',
    'footer.model': 'Modelo de puntuación {version}',
    'footer.newScan': 'Hacer otro escaneo',
    'notFound.title': 'Esta página no existe',
  },
  en: {
    'meta.home': 'QuantumReady · Post-quantum audit of your website',
    'meta.scan': '{host} result · QuantumReady',
    'meta.notFound': 'Page not found · QuantumReady',
    'nav.lang': 'Language',
    'nav.home': 'Home',
    'nav.skip': 'Skip to content',
    'home.title': 'Is your website ready for the quantum era?',
    'home.lead':
      'We check how your website encrypts connections: whether it already uses post-quantum key exchange (ML-KEM), which TLS versions and ciphers it accepts and which certificate it presents. The result, in plain language.',
    'home.demo': 'For now, the scanner is only open to these demo domains:',
    'home.scanThis': 'Scan {host}',
    'home.formLabel': 'Domain',
    'home.placeholder': 'example.com',
    'home.submit': 'Scan',
    'home.submitting': 'Sending…',
    'home.loadingTargets': 'Loading domains…',
    'err.invalid_host': 'That domain does not look valid. Type just the name, for example fizsar.com.',
    'err.port_not_allowed': 'For now only port 443 (HTTPS) is scanned. Remove the port from the domain.',
    'err.host_not_allowed': 'For now only the demo domains above can be scanned.',
    'err.body_too_large': 'The text is too long. Type just the domain.',
    'err.rate_limited': 'You have run many scans in a row. You can try again in {seconds} seconds.',
    'err.rate_limited_nowait': 'You have run many scans in a row. Wait a little and try again.',
    'err.network': 'We could not reach the server. Check your connection and try again.',
    'err.not_found': 'We cannot find that scan. The link may be incomplete.',
    'err.internal': 'Something went wrong on our side. Try again in a few minutes.',
    'progress.queued': 'Queued: the scan will start in a few seconds…',
    'progress.running': 'Scanning {host}: testing TLS versions, ciphers and post-quantum groups…',
    'progress.retrying': 'The server did not answer in time. We will try again shortly…',
    'progress.reconnecting': 'Connection lost, retrying…',
    'failed.title': 'We could not complete the scan',
    'failed.again': 'Try again',
    'headline.broken': 'Your website has a problem to fix now: browsers may show a security warning.',
    'headline.pqcWeak': 'Your website already uses post-quantum encryption, but it accepts weak ciphers that should be removed today.',
    'headline.pqc': "Your website already uses hybrid post-quantum encryption: today's traffic stays protected even when quantum computers arrive.",
    'headline.notPreferred': 'Your server supports post-quantum encryption, but browsers do not use it yet: it has to be made the preferred option.',
    'headline.weak': 'Your website has weaknesses worth fixing today and is not ready for the quantum era yet.',
    'headline.classical': 'Your website is secure today, but traffic captured now could be decrypted in the future by a quantum computer.',
    'grade.label': 'Grade',
    'grade.score': '{score} out of 100 points',
    'grade.capped': 'The grade is capped at {grade} because of: {reasons}.',
    'pqc.yes': 'PQC: yes',
    'pqc.no': 'PQC: no',
    'pqc.group': 'Negotiated group: {group}',
    'pqc.noGroup': 'Negotiated group: classical',
    'axis.quantum.title': 'Quantum risk',
    'axis.quantum.help': 'What a quantum computer could break: traffic captured today and, in the future, signatures.',
    'axis.current.title': 'Current risk',
    'axis.current.help': 'Weaknesses that can be exploited today, without quantum computers.',
    'axis.level': 'Level: {level}',
    'finding.fix': 'What to do',
    'finding.more': 'Show details',
    'breakdown.title': 'Score breakdown',
    'breakdown.points': '{points} of {max}',
    'breakdown.floor': 'Adjustment: nothing broken or weak (minimum D)',
    'breakdown.floorHelp': 'A classical setup without flaws never drops below 40 points.',
    'noteval.title': 'Not evaluated',
    'noteval.lead':
      'These checks are outside the current scanner. They never add or remove points and do not mean your website fails them.',
    'footer.scanned': 'Scanned on {date}',
    'footer.model': 'Scoring model {version}',
    'footer.newScan': 'Run another scan',
    'notFound.title': 'This page does not exist',
  },
} as const

export type MessageKey = keyof (typeof dict)['es']

/** Translates key, filling {placeholders}. */
export function translate(lang: Lang, key: MessageKey, vars?: Record<string, string | number>): string {
  let s: string = dict[lang][key] ?? dict.es[key] ?? key
  for (const [k, v] of Object.entries(vars ?? {})) s = s.replaceAll(`{${k}}`, String(v))
  return s
}

function readStored(): Lang | null {
  try {
    const v = window.localStorage.getItem(STORAGE_KEY)
    return v === 'es' || v === 'en' ? v : null
  } catch {
    return null // storage disabled (private mode, blocked cookies…)
  }
}

export function storeLang(lang: Lang) {
  try {
    window.localStorage.setItem(STORAGE_KEY, lang)
  } catch {
    // not persisted; the choice still applies to this page
  }
}

/**
 * Initial language: ?lang= in the URL, then the saved choice, then the
 * browser's: Spanish for es-*, English otherwise.
 */
export function detectLang(): Lang {
  const q = new URLSearchParams(window.location.search).get('lang')
  if (q === 'es' || q === 'en') return q
  const saved = readStored()
  if (saved) return saved
  const langs = navigator.languages?.length ? navigator.languages : [navigator.language]
  for (const l of langs) {
    const p = l?.toLowerCase().split('-')[0]
    if (p === 'es') return 'es'
    if (p === 'en') return 'en'
  }
  return 'en'
}

export interface LangContextValue {
  lang: Lang
  setLang: (l: Lang) => void
  t: (key: MessageKey, vars?: Record<string, string | number>) => string
}

export const LangContext = createContext<LangContextValue | null>(null)

/** The UI language and translator; see LangProvider. */
export function useLang(): LangContextValue {
  const ctx = useContext(LangContext)
  if (!ctx) throw new Error('useLang outside LangProvider')
  return ctx
}
