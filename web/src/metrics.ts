// M1 usage metrics: tells the API (POST /api/e) that something happened, so
// it adds one to a daily counter. No cookies, no storage, no identifiers:
// each event is anonymous and stands alone. Nothing is sent when the browser
// asks for privacy (Global Privacy Control) or is driven by automation, and a
// failure never reaches the page.

export type Route = '/' | '/examples' | '/architecture' | '/privacy' | '/about-scans' | 'account' | 'scan_report' | 'not_found'
export type Page = 'scan_report' | 'examples'

export type UsageEvent =
  | { e: 'page_view'; p: Route; entry?: true; r?: string }
  | { e: 'scan_started'; d: string }
  | { e: 'under_the_hood_open' | 'json_download'; d: Page }
  | { e: 'lang_switch'; d: 'es' | 'en' }

/**
 * The route as counted: scan results are one bucket, and the account and
 * organization pages another ("account"), never with an id.
 */
export function routeOf(pathname: string): Route {
  if (pathname === '/' || pathname === '/examples' || pathname === '/architecture' || pathname === '/privacy' || pathname === '/about-scans')
    return pathname
  if (/^\/scans\/[^/]+\/?$/.test(pathname)) return 'scan_report'
  if (/^\/(login|register|verify|forgot|reset|account|invite|org)(\/|$)/.test(pathname)) return 'account'
  return 'not_found'
}

function optedOut(): boolean {
  const n = navigator as Navigator & { globalPrivacyControl?: boolean }
  return n.globalPrivacyControl === true || n.webdriver === true
}

export function track(ev: UsageEvent): void {
  try {
    if (optedOut()) return
    const body = JSON.stringify(ev)
    if (typeof navigator.sendBeacon === 'function' && navigator.sendBeacon('/api/e', new Blob([body], { type: 'application/json' }))) return
    void fetch('/api/e', { method: 'POST', body, keepalive: true, headers: { 'Content-Type': 'application/json' } }).catch(() => {})
  } catch {
    // metrics never break the page
  }
}

let entered = false

/**
 * A page view. The first one after the page loaded is the visit's entry and
 * carries where it came from: the referrer's host, only if another site
 * (the API reduces it to its registrable domain; never a URL).
 */
export function pageView(pathname: string): void {
  const ev: UsageEvent = { e: 'page_view', p: routeOf(pathname) }
  if (!entered) {
    entered = true
    ev.entry = true
    try {
      const host = document.referrer ? new URL(document.referrer).host : ''
      if (host && host !== window.location.host) ev.r = host
    } catch {
      // an unparsable referrer: no source
    }
  }
  track(ev)
}

/** Tests only: the next page view is an entry again. */
export function resetEntryForTests(): void {
  entered = false
}
