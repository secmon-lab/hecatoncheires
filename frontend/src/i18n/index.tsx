import { createContext, useContext, useState, useCallback, type ReactNode } from 'react'
import type { MsgKey, Messages } from './keys'
import { en } from './en'
import { ja } from './ja'

export type Lang = 'en' | 'ja'

const STORAGE_KEY = 'hecatoncheires-lang'

const messagesMap: Record<Lang, Messages> = { en, ja }

function detectBrowserLang(fallback: Lang): Lang {
  const nav = navigator.language || ''
  if (nav.startsWith('ja')) return 'ja'
  if (nav.startsWith('en')) return 'en'
  return fallback
}

function loadStoredLang(): Lang | null {
  const stored = localStorage.getItem(STORAGE_KEY)
  if (stored === 'en' || stored === 'ja') return stored
  return null
}

function interpolate(template: string, params: Record<string, string | number>): string {
  return template.replace(/\{(\w+)\}/g, (_, key) => {
    return key in params ? String(params[key]) : `{${key}}`
  })
}

interface I18nContextValue {
  lang: Lang
  setLang: (lang: Lang) => void
  t: (key: MsgKey, params?: Record<string, string | number>) => string
}

const I18nContext = createContext<I18nContextValue | null>(null)

interface I18nProviderProps {
  defaultLang?: Lang
  children: ReactNode
}

export function I18nProvider({ defaultLang = 'en', children }: I18nProviderProps) {
  const [lang, setLangState] = useState<Lang>(() => {
    return loadStoredLang() ?? detectBrowserLang(defaultLang)
  })

  const setLang = useCallback((newLang: Lang) => {
    setLangState(newLang)
    localStorage.setItem(STORAGE_KEY, newLang)
  }, [])

  const t = useCallback(
    (key: MsgKey, params?: Record<string, string | number>): string => {
      const msg = messagesMap[lang]?.[key] ?? messagesMap[defaultLang]?.[key] ?? key
      return params ? interpolate(msg, params) : msg
    },
    [lang, defaultLang],
  )

  return (
    <I18nContext.Provider value={{ lang, setLang, t }}>
      {children}
    </I18nContext.Provider>
  )
}

export function useTranslation(): I18nContextValue {
  const ctx = useContext(I18nContext)
  if (!ctx) {
    throw new Error('useTranslation must be used within I18nProvider')
  }
  return ctx
}

export type { MsgKey, Messages }
