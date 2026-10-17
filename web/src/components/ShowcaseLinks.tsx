import { Link } from 'react-router'
import { useShowcaseT } from '../i18n.showcase'

/** Discreet links under the home form to the showcase pages. */
export function ShowcaseHomeLinks() {
  const { t } = useShowcaseT()
  return (
    <nav className="showcase-links" aria-label={t('links.label')}>
      <Link to="/examples">{t('links.examples')}</Link>
      <span aria-hidden="true">·</span>
      <Link to="/architecture">{t('links.architecture')}</Link>
    </nav>
  )
}
