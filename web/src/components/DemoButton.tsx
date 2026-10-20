import { Link } from 'react-router'
import { useDemoEnabled } from '../features'
import { useShowcaseT } from '../i18n.showcase'
import type { DemoSource } from '../metrics'
import './DemoButton.css'

type From = Exclude<DemoSource, 'direct'>

/**
 * "Request a free demo": a link to /demo that tells the form where it was
 * pressed (router state, not the URL), for the M1 demo_requested count.
 */
export function DemoButton({ from }: { from: From }) {
  const { t } = useShowcaseT()
  return (
    <Link to="/demo" state={{ from }} className="btn btn-demo">
      {t('demo.button')}
    </Link>
  )
}

/** The line with the button, only when the server has the form on (DEMO_ENABLED). */
export function DemoCta({ from }: { from: From }) {
  const { t } = useShowcaseT()
  if (useDemoEnabled() !== true) return null
  return (
    <p className="demo-cta">
      <span className="muted">{t('demo.cta')}</span>
      <DemoButton from={from} />
    </p>
  )
}
