import { createContext, useContext, useEffect, useId, useState } from 'react'
import { useLocation } from 'react-router'
import type { ScanView } from '../api'
import { useShowcaseT, type ShowcaseKey } from '../i18n.showcase'
import { track } from '../metrics'
import {
  downloadJSON,
  expandOffer,
  groupName,
  hex,
  isGrease,
  offeredVersions,
  otherRawProbes,
  parseError,
  probeRows,
  resultOf,
  signatureName,
  suitesByVersion,
  versionName,
  versionsTried,
  type LogOffer,
  type LogProbe,
  type PresentedCert,
  type Offer,
  type ProbeLog,
  type Reply,
  type ScanResult,
} from '../underTheHood'
import './UnderTheHood.css'

type T = (key: ShowcaseKey, vars?: Record<string, string | number>) => string

// Heading levels inside the panel, one and two below its title.
const Levels = createContext({ Sub: 'h3' as 'h3' | 'h4', Sub2: 'h4' as 'h4' | 'h5' })

/**
 * Collapsible panel with the scanner facts of a finished scan; closed by
 * default. A link to #<anchor> (…/scans/<id>#under-the-hood) opens it and
 * scrolls to it. level 2 when the report is nested under a page heading.
 */
export function UnderTheHood({ scan, anchor = 'under-the-hood', level = 1 }: { scan: ScanView; anchor?: string; level?: 1 | 2 }) {
  const { lang, t } = useShowcaseT()
  const { hash } = useLocation()
  const linked = hash === '#' + anchor
  const [open, setOpen] = useState(linked)
  // A link to the anchor followed while the page is shown opens it too.
  const [wasLinked, setWasLinked] = useState(linked)
  if (linked !== wasLinked) {
    setWasLinked(linked)
    if (linked) setOpen(true)
  }
  const id = useId()
  useEffect(() => {
    if (linked) document.getElementById(anchor)?.scrollIntoView?.()
  }, [linked, anchor])
  // M1: each time it opens, by a click or a link.
  const page = anchor === 'under-the-hood' ? 'scan_report' : 'examples'
  useEffect(() => {
    if (open) track({ e: 'under_the_hood_open', d: page })
  }, [open, page])
  const r = scan.report && resultOf(scan.report)
  if (!r) return null
  const num = new Intl.NumberFormat(lang === 'es' ? 'es-ES' : 'en-GB', { maximumFractionDigits: 1 })
  const H = level === 1 ? 'h2' : 'h3'

  return (
    <section id={anchor} className="panel uth" aria-labelledby={`${id}-h`}>
      <H id={`${id}-h`} className="h-section uth-heading">
        <button type="button" className="uth-toggle" aria-expanded={open} aria-controls={`${id}-body`} onClick={() => setOpen(!open)}>
          {t('uth.title')}
        </button>
      </H>
      <Levels.Provider value={level === 1 ? { Sub: 'h3', Sub2: 'h4' } : { Sub: 'h4', Sub2: 'h5' }}>
      <div id={`${id}-body`} className="uth-body" hidden={!open}>
        <p className="muted small">{t('uth.lead')}</p>
        <div className="uth-download">
          <button type="button" className="btn" onClick={() => {
              downloadJSON(scan)
              track({ e: 'json_download', d: page })
            }}>
            {t('uth.download')}
          </button>
          <span className="muted small">{t('uth.downloadHelp')}</span>
        </div>

        <ScanFacts
          r={r}
          t={t}
          duration={r.duration_ms < 1000 ? t('uth.ms', { ms: r.duration_ms }) : t('uth.seconds', { s: num.format(r.duration_ms / 1000) })}
        />
        <Versions r={r} t={t} />
        <Groups r={r} t={t} />
        {r.certificate_selection && <Certificates sel={r.certificate_selection} t={t} />}
        <Suites r={r} t={t} />
        {r.probe_log ? <LogProbes log={r.probe_log} t={t} /> : <Probes r={r} t={t} />}
      </div>
      </Levels.Provider>
    </section>
  )
}

function ScanFacts({ r, t, duration }: { r: ScanResult; t: T; duration: string }) {
  const { Sub } = useContext(Levels)
  return (
    <section className="uth-section" aria-label={t('uth.scan.title')}>
      <Sub className="uth-h">{t('uth.scan.title')}</Sub>
      <dl className="uth-facts">
        <dt>{t('uth.scan.duration')}</dt>
        <dd>{duration}</dd>
        <dt>{t('uth.scan.ip')}</dt>
        <dd className="mono">{r.ip}</dd>
        <dt>{t('uth.scan.resolved')}</dt>
        <dd className="mono">{(r.resolved_ips ?? []).join(', ') || '—'}</dd>
        {r.connections !== undefined && (
          <>
            <dt>{t('uth.scan.connections')}</dt>
            <dd>{r.connections}</dd>
          </>
        )}
        {r.intensity && (
          <>
            <dt>{t('uth.scan.intensity')}</dt>
            <dd>{t(`uth.intensity.${r.intensity}`)}</dd>
          </>
        )}
        {r.raw && (
          <>
            <dt>{t('uth.scan.rawProbes')}</dt>
            <dd>{r.raw.budget_exhausted ? t('uth.scan.budget', { n: r.raw.probes }) : r.raw.probes}</dd>
          </>
        )}
      </dl>
    </section>
  )
}

function Versions({ r, t }: { r: ScanResult; t: T }) {
  const { Sub } = useContext(Levels)
  const tried = versionsTried(r)
  const accepted = tried.filter((v) => v.accepted).map((v) => v.version)
  return (
    <section className="uth-section" aria-label={t('uth.versions.title')}>
      <Sub className="uth-h">{t('uth.versions.title')}</Sub>
      <p>{t('uth.versions.tried', { list: tried.map((v) => v.version).join(', ') })}</p>
      <p>{t('uth.versions.accepted', { list: accepted.join(', ') || t('uth.none') })}</p>
    </section>
  )
}

function Groups({ r, t }: { r: ScanResult; t: T }) {
  const { Sub } = useContext(Levels)
  const kx = r.key_exchange
  const names = (l: { name: string }[] | null | undefined) => (l ?? []).map((g) => g.name).join(', ')
  return (
    <section className="uth-section" aria-label={t('uth.groups.title')}>
      <Sub className="uth-h">{t('uth.groups.title')}</Sub>
      <p>{t('uth.groups.offered', { list: (kx.offered ?? []).join(', ') })}</p>
      <p>{kx.selected ? t('uth.groups.selected', { group: kx.selected }) : t('uth.groups.selectedNone')}</p>
      <div className="uth-scroll">
        <table className="uth-table">
          <thead>
            <tr>
              <th scope="col">{t('uth.col.group')}</th>
              <th scope="col">{t('uth.col.id')}</th>
              <th scope="col">{t('uth.col.hybrid')}</th>
              <th scope="col">{t('uth.col.reply')}</th>
            </tr>
          </thead>
          <tbody>
            {(kx.groups ?? []).map((g) => (
              <tr key={g.id}>
                <td className="mono">{g.name}</td>
                <td className="mono">{hex(g.id)}</td>
                <td>{g.hybrid ? t('uth.yes') : t('uth.no')}</td>
                <td>
                  <ReplyText reply={g.status === 'supported' ? { kind: 'hello', group: g.name } : parseError(g.error)} t={t} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {!!r.raw?.tls13_groups?.length && <p>{t('uth.groups.raw13', { list: names(r.raw.tls13_groups) })}</p>}
      {!!r.raw?.tls12_curves?.length && <p>{t('uth.groups.raw12', { list: names(r.raw.tls12_curves) })}</p>}
    </section>
  )
}

function Suites({ r, t }: { r: ScanResult; t: T }) {
  const { Sub, Sub2 } = useContext(Levels)
  const versions = suitesByVersion(r)
  return (
    <section className="uth-section" aria-label={t('uth.suites.title')}>
      <Sub className="uth-h">{t('uth.suites.title')}</Sub>
      <p className="muted small">{t('uth.suites.lead')}</p>
      {versions.length === 0 && <p>{t('uth.suites.none')}</p>}
      {versions.map((v) => (
        <div key={v.version} className="uth-suites">
          <Sub2 className="uth-h4">{v.version}</Sub2>
          <div className="uth-scroll">
            <table className="uth-table">
              <thead>
                <tr>
                  <th scope="col">{t('uth.col.suite')}</th>
                  <th scope="col">{t('uth.col.id')}</th>
                  <th scope="col">{t('uth.col.fs')}</th>
                  <th scope="col">{t('uth.col.aead')}</th>
                  <th scope="col">{t('uth.col.notes')}</th>
                  <th scope="col">{t('uth.col.source')}</th>
                </tr>
              </thead>
              <tbody>
                {v.suites.map((s) => (
                  <tr key={s.name}>
                    <td className="mono">{s.name}</td>
                    <td className="mono">{s.id ? hex(s.id) : '—'}</td>
                    <td>{s.forward_secrecy ? t('uth.yes') : t('uth.no')}</td>
                    <td>{s.aead ? t('uth.yes') : t('uth.no')}</td>
                    <td>
                      {[
                        s.export && t('uth.note.export'),
                        s.anonymous && t('uth.note.anonymous'),
                        s.bits === 0 ? t('uth.note.null') : s.bits !== undefined && t('uth.note.bits', { bits: s.bits }),
                      ]
                        .filter(Boolean)
                        .join(' · ')}
                    </td>
                    <td>{t(`uth.source.${s.source}`)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {!v.complete && <p className="muted small">{t('uth.suites.incomplete')}</p>}
        </div>
      ))}
      {(r.raw?.dh ?? []).map((d) => (
        <p key={d.version + d.suite} className="small">
          {t('uth.dh', { suite: d.suite, version: d.version, bits: d.prime_bits })}
        </p>
      ))}
    </section>
  )
}

function Probes({ r, t }: { r: ScanResult; t: T }) {
  const { Sub } = useContext(Levels)
  const rows = probeRows(r)
  return (
    <section className="uth-section" aria-label={t('uth.probes.title')}>
      <Sub className="uth-h">{t('uth.probes.title')}</Sub>
      <p className="muted small">{t('uth.probes.lead')}</p>
      <ol className="uth-probes">
        {rows.map((p, i) => (
          <li key={i} className={`uth-probe reply-${p.reply.kind}`}>
            <span className="uth-src">{t(`uth.source.${p.source}`)}</span>
            <span className="uth-offer">
              <span className="sr-only">{t('uth.col.offered')}: </span>
              <OfferText offer={p.offer} t={t} />
            </span>
            <span className="uth-arrow" aria-hidden="true">
              →
            </span>
            <span className="uth-reply">
              <span className="sr-only">{t('uth.col.reply')}: </span>
              <ReplyText reply={p.reply} t={t} />
            </span>
          </li>
        ))}
      </ol>
      {r.raw && <p className="muted small">{rawNote(otherRawProbes(r), t)}</p>}
    </section>
  )
}

// Schema 2: the probe log, every connection as it went on the wire.
function LogProbes({ log, t }: { log: ProbeLog; t: T }) {
  const { Sub } = useContext(Levels)
  return (
    <section className="uth-section" aria-label={t('uth.probes.title')}>
      <Sub className="uth-h">{t('uth.probes.title')}</Sub>
      <p className="muted small">{t('uth.log.lead', { n: log.probes.length })}</p>
      <ol className="uth-probes uth-log">
        {log.probes.map((p, i) => {
          const offer = p.offer !== undefined ? expandOffer(log, p.offer) : undefined
          const retry = p.retry_offer !== undefined ? expandOffer(log, p.retry_offer) : undefined
          return (
            <li key={i} className={`uth-probe reply-${logReplyKind(p)}`}>
              <span className="uth-src">{t(p.client === 'raw' ? 'uth.source.raw' : 'uth.source.cryptotls')}</span>
              <span className="uth-offer">
                <span className="sr-only">{t('uth.col.offered')}: </span>
                <strong>{purposeText(p, t)}</strong>
                {offer && <span className="mono"> · {offerSummary(offer, t)}</span>}
              </span>
              <span className="uth-arrow" aria-hidden="true">
                →
              </span>
              <span className="uth-reply">
                <span className="sr-only">{t('uth.col.reply')}: </span>
                <LogReply p={p} t={t} />
                {retry && (
                  <span className="mono small">
                    {' · '}
                    {t('uth.log.retry', { list: (retry.key_shares ?? []).map(groupName).join(', ') || t('uth.none') })}
                  </span>
                )}
              </span>
              {offer && <OfferDetails offer={offer} title={t('uth.offer.d.title')} t={t} />}
              {retry && <OfferDetails offer={retry} title={t('uth.offer.d.retryTitle')} t={t} />}
            </li>
          )
        })}
      </ol>
    </section>
  )
}

function logReplyKind(p: LogProbe) {
  switch (p.response.type) {
    case 'server_hello':
    case 'hello_retry_request':
    case 'sslv2_server_hello':
      return p.response.alert ? 'alert' : 'hello'
    case 'alert':
    case 'sslv2_error':
      return 'alert'
    case 'timeout':
      return 'timeout'
    case 'closed':
      return 'closed'
  }
  return 'other'
}

const PURPOSES = ['default', 'group', 'version', 'suites', 'sslv2', 'sslv3', 'tls13_suites', 'tls13_groups', 'tls12_curves', 'certificate'] as const

function purposeText(p: LogProbe, t: T) {
  const known = PURPOSES.find((k) => k === p.purpose)
  return known ? t(`uth.purpose.${known}`, { subject: p.subject ?? '' }) : [p.purpose, p.subject].filter(Boolean).join(' ')
}

const shortList = (names: string[], max = 4) => (names.length > max ? [...names.slice(0, max), '…'] : names).join(', ')

function offerSummary(o: LogOffer, t: T) {
  const parts = [offeredVersions(o).map(versionName).join('/')]
  if (o.sslv2) parts.push(t('uth.offer.ciphers', { n: o.sslv2_ciphers?.length ?? 0 }))
  else parts.push(t('uth.offer.suites', { n: (o.suites ?? []).filter((s) => !isGrease(s)).length }))
  if (o.groups?.length) parts.push(t('uth.offer.groups', { list: shortList(o.groups.map(groupName)) }))
  if (o.key_shares !== undefined) parts.push(t('uth.offer.keyShares', { list: o.key_shares.map(groupName).join(', ') || t('uth.none') }))
  if (o.signature_schemes?.length) parts.push(t('uth.offer.signatures', { n: o.signature_schemes.filter((s) => !isGrease(s)).length }))
  if (o.alpn?.length) parts.push('ALPN ' + o.alpn.join(', '))
  return parts.filter(Boolean).join(' · ')
}

function LogReply({ p, t }: { p: LogProbe; t: T }) {
  const r = p.response
  const known = ['server_hello', 'hello_retry_request', 'alert', 'closed', 'timeout', 'not_tls', 'sslv2_server_hello', 'sslv2_error', 'no_connection', 'not_sent'] as const
  const type = known.find((k) => k === r.type)
  const facts = [
    r.retry_group_name && t('uth.resp.retry', { group: r.retry_group_name }),
    r.version && versionName(r.version),
    r.cipher_suite_name,
    r.group_name,
    r.dh_bits && t('uth.resp.dh', { bits: r.dh_bits }),
    r.sslv2_ciphers && t('uth.reply.ciphers', { n: r.sslv2_ciphers }),
    r.certificate && t('uth.resp.certificate', { key: certName(r.certificate) }),
    r.handshake_signature && t('uth.resp.signature', { scheme: signatureName(r.handshake_signature) }),
  ].filter(Boolean)
  const ok = type === 'server_hello' || type === 'hello_retry_request' || type === 'sslv2_server_hello'
  return (
    <>
      <span className={ok ? 'ok' : type === 'timeout' ? 'warn' : 'bad'}>{type ? t(`uth.resp.${type}`) : r.type}</span>
      {facts.length > 0 && <span className="mono"> · {facts.join(' · ')}</span>}
      {r.alert && (
        <span className="bad">
          {' · '}
          {t('uth.reply.alert', { code: r.alert.code, name: r.alert.name })}{' '}
          <span className="muted">
            ({t(!r.alert.level ? 'uth.resp.levelUnknown' : r.alert.level === 1 ? 'uth.resp.warning' : 'uth.resp.fatal')}
            {r.alert.level && r.alert.encrypted ? ', ' + t('uth.resp.decrypted') : ''})
          </span>
        </span>
      )}
      <span className="muted small">
        {' · '}
        {t('uth.ms', { ms: p.duration_ms })}
        {p.error_code && <> · <span className="mono">{p.error_code}</span></>}
      </span>
    </>
  )
}

const certName = (c: PresentedCert) => c.key_algorithm || c.key_oid

// Milestone 2.8.5: which certificate the server presents with and without ML-DSA in the offer.
function Certificates({ sel, t }: { sel: NonNullable<ScanResult['certificate_selection']>; t: T }) {
  const { Sub } = useContext(Levels)
  const cert = (c: PresentedCert) => t('uth.certs.cert', { key: certName(c), oid: c.key_oid, pq: c.post_quantum ? t('uth.certs.pq') : t('uth.certs.classical') })
  let without: string
  if (sel.without_mldsa) without = cert(sel.without_mldsa)
  else if (sel.without_status === 'not_supported') without = t('uth.certs.refused')
  else if (sel.without_status === 'error') without = t('uth.certs.unknown')
  else without = t('uth.certs.notProbed')
  return (
    <section className="uth-section" aria-label={t('uth.certs.title')}>
      <Sub className="uth-h">{t('uth.certs.title')}</Sub>
      <p>{t('uth.certs.with', { cert: sel.with_mldsa ? cert(sel.with_mldsa) : t('uth.certs.none') })}</p>
      <p>{t('uth.certs.without', { cert: without })}</p>
    </section>
  )
}

function OfferDetails({ offer: o, title, t }: { offer: LogOffer; title: string; t: T }) {
  const rows: [string, string][] = [
    [t('uth.offer.d.versions'), offeredVersions(o).map(versionName).join(', ')],
    [o.sslv2 ? t('uth.offer.d.ciphers') : t('uth.offer.d.suites'), o.sslv2 ? (o.sslv2_ciphers ?? []).map((c) => '0x' + c.toString(16).padStart(6, '0')).join(' ') : (o.suites ?? []).map((s) => (isGrease(s) ? 'GREASE' : hex(s))).join(' ')],
    [t('uth.offer.d.groups'), (o.groups ?? []).map(groupName).join(', ')],
    [t('uth.offer.d.keyShares'), o.key_shares === undefined ? '' : o.key_shares.map(groupName).join(', ') || t('uth.offer.d.emptyKeyShare')],
    [t('uth.offer.d.signatures'), (o.signature_schemes ?? []).map(signatureName).join(', ')],
    [t('uth.offer.d.signaturesCert'), (o.signature_schemes_cert ?? []).map(signatureName).join(', ')],
    ['ALPN', (o.alpn ?? []).join(', ')],
    [t('uth.offer.d.extensions'), (o.extensions ?? []).map((e) => (isGrease(e) ? 'GREASE' : String(e))).join(', ')],
  ]
  return (
    <details className="uth-offer-details">
      <summary>{title}</summary>
      <dl className="uth-facts">
        {rows
          .filter(([, v]) => v)
          .map(([k, v]) => (
            <div key={k} className="uth-fact">
              <dt>{k}</dt>
              <dd className="mono">{v}</dd>
            </div>
          ))}
      </dl>
    </details>
  )
}

function rawNote(n: number | undefined, t: T) {
  if (n === undefined) return t('uth.probes.rawNoteSome')
  if (n === 0) return t('uth.probes.rawNoteNone')
  if (n === 1) return t('uth.probes.rawNoteOne')
  return t('uth.probes.rawNote', { n })
}

function OfferText({ offer, t }: { offer: Offer; t: T }) {
  switch (offer.kind) {
    case 'default':
      return <>{t('uth.offer.default', { groups: offer.groups.join(', ') })}</>
    case 'version':
      return <>{t('uth.offer.version', { version: offer.version })}</>
    case 'group':
      return <>{t('uth.offer.group', { group: offer.group, id: hex(offer.id) })}</>
  }
}

export function ReplyText({ reply, t }: { reply: Reply; t: T }) {
  switch (reply.kind) {
    case 'hello': {
      const parts = [reply.version, reply.suite, reply.group, reply.ciphers !== undefined && t('uth.reply.ciphers', { n: reply.ciphers })].filter(Boolean)
      return (
        <>
          <span className="ok">{t('uth.reply.hello')}</span>
          {parts.length > 0 && <span className="mono"> · {parts.join(' · ')}</span>}
        </>
      )
    }
    case 'alert':
      return (
        <>
          <span className="bad">
            {reply.code !== undefined ? t('uth.reply.alert', { code: reply.code, name: reply.name }) : t('uth.reply.alertNoCode', { name: reply.name })}
          </span>
          {reply.byName && <span className="muted"> ({t('uth.reply.ianaCode')})</span>}
        </>
      )
    case 'closed':
      return <span className="bad">{t('uth.reply.closed')}</span>
    case 'timeout':
      return <span className="warn">{t('uth.reply.timeout')}</span>
    case 'refused':
      return <span className="bad">{t('uth.reply.refused')}</span>
    case 'other':
      return <span className="bad mono">{reply.text}</span>
  }
}
