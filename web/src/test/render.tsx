import { render } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import type { Lang } from '../api'
import App from '../App'
import { AuthProvider } from '../AuthProvider'
import { LangProvider } from '../LangProvider'

/** Renders the whole app at path, as main.tsx does (with a memory router). */
export function renderAt(path: string, lang: Lang = 'es') {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <LangProvider initial={lang}>
        <AuthProvider>
          <App />
        </AuthProvider>
      </LangProvider>
    </MemoryRouter>,
  )
}
