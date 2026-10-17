import { useId, useState } from 'react'
import type { ScanView } from '../api'
import { useShowcaseT, type ShowcaseKey } from '../i18n.showcase'
import { downloadJSON, hex, parseError, probeRows, resultOf, suitesByVersion, versionsTried, type Offer, type Reply, type ScanResult } from '../underTheHood'
import './UnderTheHood.css'

type T = (key: ShowcaseKey, vars?: Record<string, string | number>) => string

/** Anchor of the panel: a link to …/scans/<id>#under-the-hood opens it. */
const ANCHOR = 'under-the-hood'

/** Collapsible panel with the scanner facts of a finished scan; closed by default. */
export function UnderTheHood({ scan }: { scan: ScanView }) {
  const { lang, t } = useShowcaseT()
  const [open, setOpen] = useState(() => window.location.hash === '#' + ANCHOR)
  const id = useId()
  const r = scan.report && resultOf(scan.report)
  if (!r) return null
  const num = new Intl.NumberFormat(lang === 'es' ? 'es-ES' : 'en-GB', { maximumFractionDigits: 1 })

  return (
    <section id={ANCHOR} className="panel uth" aria-labelledby={`${id}-h`}>
      <h2 id={`${id}-h`} className="h-section uth-heading">
        <button type="button" className="uth-toggle" aria-expanded={open} aria-controls={`${id}-body`} onClick={() => setOpen(!open)}>
          {t('uth.title')}
        </button>
      </h2>
      <div id={`${id}-body`} className="uth-body" hidden={!open}>
        <p className="muted small">{t('uth.lead')}</p>
        <div className="uth-download">
          <button type="button" className="btn" onClick={() => downloadJSON(scan)}>
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
        <Suites r={r} t={t} />
        <Probes r={r} t={t} />
      </div>
    </section>
  )
}

function ScanFacts({ r, t, duration }: { r: ScanResult; t: T; duration: string }) {
  return (
    <section className="uth-section" aria-label={t('uth.scan.title')}>
      <h3 className="uth-h">{t('uth.scan.title')}</h3>
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
  const tried = versionsTried(r)
  const accepted = tried.filter((v) => v.accepted).map((v) => v.version)
  return (
    <section className="uth-section" aria-label={t('uth.versions.title')}>
      <h3 className="uth-h">{t('uth.versions.title')}</h3>
      <p>{t('uth.versions.tried', { list: tried.map((v) => v.version).join(', ') })}</p>
      <p>{t('uth.versions.accepted', { list: accepted.join(', ') || t('uth.none') })}</p>
    </section>
  )
}

function Groups({ r, t }: { r: ScanResult; t: T }) {
  const kx = r.key_exchange
  const names = (l: { name: string }[] | null | undefined) => (l ?? []).map((g) => g.name).join(', ')
  return (
    <section className="uth-section" aria-label={t('uth.groups.title')}>
      <h3 className="uth-h">{t('uth.groups.title')}</h3>
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
  const versions = suitesByVersion(r)
  return (
    <section className="uth-section" aria-label={t('uth.suites.title')}>
      <h3 className="uth-h">{t('uth.suites.title')}</h3>
      <p className="muted small">{t('uth.suites.lead')}</p>
      {versions.length === 0 && <p>{t('uth.suites.none')}</p>}
      {versions.map((v) => (
        <div key={v.version} className="uth-suites">
          <h4 className="uth-h4">{v.version}</h4>
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
  const rows = probeRows(r)
  return (
    <section className="uth-section" aria-label={t('uth.probes.title')}>
      <h3 className="uth-h">{t('uth.probes.title')}</h3>
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
      {r.raw && <p className="muted small">{t('uth.probes.rawNote', { n: r.raw.probes })}</p>}
    </section>
  )
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
        <span className="bad">
          {reply.code !== undefined ? t('uth.reply.alert', { code: reply.code, name: reply.name }) : t('uth.reply.alertNoCode', { name: reply.name })}
        </span>
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
