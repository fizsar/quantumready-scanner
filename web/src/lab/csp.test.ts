import { describe, expect, it } from 'vitest'
import defaultConf from '../../nginx/default.conf?raw'
import headersConf from '../../nginx/security-headers.conf?raw'
import sitemap from '../../public/sitemap.xml?raw'

// The redesign keeps the CSP exactly as it is: three.js, the worker, the still
// images and the fonts are all served from our own origin. If a scene ever
// needs more, this test must be changed on purpose.
const CSP =
  "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

describe('CSP', () => {
  it('is unchanged', () => {
    expect(headersConf).toContain(`add_header Content-Security-Policy "${CSP}" always;`)
    expect(headersConf.match(/Content-Security-Policy/g)).toHaveLength(1)
  })

  it('is sent by every location that serves the app, /lab/ included', () => {
    const blocks = defaultConf.split(/\n\s*location\s+/).slice(1)
    const served = blocks.filter((b) => !b.startsWith('= /healthz'))
    expect(served.some((b) => b.startsWith('/lab/'))).toBe(true)
    for (const b of served) expect(b, b.split('\n')[0]).toContain('include /etc/nginx/snippets/security-headers.conf;')
    const lab = served.find((b) => b.startsWith('/lab/'))
    expect(lab).toContain('X-Robots-Tag "noindex, nofollow"')
    expect(lab).toContain('try_files $uri /index.html =404;') // the still images are real files, not the SPA
  })

  it('/lab is not in the sitemap', () => {
    expect(sitemap).not.toContain('/lab')
  })
})
