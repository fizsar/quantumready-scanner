import { describe, expect, it } from 'vitest'
import type { Finding, Report } from './api'
import { reportA, reportD } from './test/fixtures'
import type { MessageKey } from './i18n'
import { headline, type Tone } from './verdict'

const finding = (code: string, severity: Finding['severity'] = 'critical'): Finding => ({
  code, axis: 'current', axis_label: '', severity, severity_label: '', category: '', title: code, detail: '',
})

function withFindings(base: Report, codes: string[], cap?: { grade: string; codes: string[] }): Report {
  return {
    ...base,
    findings: [...base.findings, ...codes.map((c) => finding(c))],
    current_risk: { code: 'critical', label: '' },
    cap: cap ? { grade: cap.grade, findings: cap.codes.map((c) => ({ code: c, label: c })) } : base.cap,
  }
}

const cases: [string, Report, MessageKey, Tone][] = [
  ['broken certificate wins over everything', withFindings(reportD, ['CERT_EXPIRED', 'PROTO_SSL2'], { grade: 'F', codes: ['CERT_EXPIRED', 'PROTO_SSL2'] }), 'headline.broken', 'bad'],
  ['SSLv2', withFindings(reportD, ['PROTO_SSL2'], { grade: 'F', codes: ['PROTO_SSL2'] }), 'headline.ssl', 'bad'],
  ['SSLv3', withFindings(reportD, ['PROTO_SSL3'], { grade: 'C', codes: ['PROTO_SSL3'] }), 'headline.ssl', 'bad'],
  ['NULL suites', withFindings(reportD, ['SUITE_NULL', 'SUITE_EXPORT'], { grade: 'F', codes: ['SUITE_NULL', 'SUITE_EXPORT'] }), 'headline.noEncryption', 'bad'],
  ['integrity-only TLS 1.3', withFindings(reportA, ['SUITE_INTEGRITY_ONLY'], { grade: 'F', codes: ['SUITE_INTEGRITY_ONLY'] }), 'headline.noEncryption', 'bad'],
  ['EXPORT, anonymous, DES', withFindings(reportD, ['SUITE_ANON'], { grade: 'F', codes: ['SUITE_ANON'] }), 'headline.brokenCiphers', 'bad'],
  ['Logjam, < 1024 bits', withFindings(reportD, ['DH_WEAK'], { grade: 'F', codes: ['DH_WEAK'] }), 'headline.logjam', 'bad'],
  ['Logjam, 1024 bits', withFindings(reportD, ['DH_SMALL'], { grade: 'C', codes: ['DH_SMALL'] }), 'headline.logjam', 'warn'],
  ['experimental PQC only', { ...reportD, findings: [...reportD.findings, finding('KEX_EXPERIMENTAL_PQC_ONLY', 'medium')] }, 'headline.pqcExperimental', 'warn'],
  ['PQC as before', reportA, 'headline.pqc', 'good'],
  ['classical as before', reportD, 'headline.classical', 'warn'],
]

describe('headline', () => {
  it.each(cases)('%s', (_, report, key, tone) => {
    expect(headline(report)).toEqual({ key, tone })
  })
})
