import { Link } from 'react-router'
import { useShowcaseT } from '../i18n.showcase'
import type { DemoSource } from '../metrics'
import './DemoButton.css'

/**
 * "Request a free demo": a link to /demo that tells the form where it was
 * pressed (router state, not the URL), for the M1 demo_requested count.
 */
export function DemoButton({ from }: { from: Exclude<DemoSource, 'direct'> }) {
  const { t } = useShowcaseT()
  return (
    <Link to="/demo" state={{ from }} className="btn btn-demo">
      {t('demo.button')}
    </Link>
  )
}
