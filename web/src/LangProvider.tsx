import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import type { Lang } from './api'
import { detectLang, LangContext, storeLang, translate, type MessageKey } from './i18n'

/** Provides the UI language (see detectLang), persists changes and sets <html lang>. */
export function LangProvider({ children, initial }: { children: ReactNode; initial?: Lang }) {
  const [lang, setLangState] = useState<Lang>(() => initial ?? detectLang())
  const setLang = useCallback((l: Lang) => {
    storeLang(l)
    setLangState(l)
  }, [])
  useEffect(() => {
    document.documentElement.lang = lang
  }, [lang])
  const value = useMemo(
    () => ({ lang, setLang, t: (k: MessageKey, v?: Record<string, string | number>) => translate(lang, k, v) }),
    [lang, setLang],
  )
  return <LangContext.Provider value={value}>{children}</LangContext.Provider>
}
