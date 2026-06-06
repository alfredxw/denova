import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import zhCN from './locales/zh-CN'
import enUS from './locales/en-US'

export type LocaleCode = 'auto' | 'zh-CN' | 'en-US'
export type ResolvedLocale = Exclude<LocaleCode, 'auto'>

export const LOCALE_OPTIONS: Array<{ value: LocaleCode; labelKey: string }> = [
  { value: 'auto', labelKey: 'locale.auto' },
  { value: 'zh-CN', labelKey: 'locale.zh-CN' },
  { value: 'en-US', labelKey: 'locale.en-US' },
]

let configuredLocale: LocaleCode = 'auto'
let fetchHeadersInstalled = false

export function resolveLocale(locale: string | undefined | null, browserLanguage = browserLocaleSource()): ResolvedLocale {
  const value = (locale || 'auto').trim()
  if (value === 'zh-CN' || value.toLowerCase().startsWith('zh')) return 'zh-CN'
  if (value === 'en-US' || value.toLowerCase().startsWith('en')) return 'en-US'
  return browserLanguage.toLowerCase().startsWith('zh') ? 'zh-CN' : 'en-US'
}

export function setConfiguredLocale(locale: string | undefined | null) {
  configuredLocale = normalizeLocaleCode(locale)
  const resolved = resolveLocale(configuredLocale)
  if (i18next.language !== resolved) {
    void i18next.changeLanguage(resolved)
  }
  if (typeof document !== 'undefined') {
    document.documentElement.lang = resolved
  }
}

export function getConfiguredLocale(): LocaleCode {
  return configuredLocale
}

export function getResolvedLocale(): ResolvedLocale {
  return resolveLocale(configuredLocale)
}

export function localeRequestHeader(): string {
  return getResolvedLocale()
}

export function formatDateTime(value: string | Date | number | undefined | null): string {
  if (!value) return ''
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return new Intl.DateTimeFormat(getResolvedLocale(), {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

export function formatLocaleNumber(value: number): string {
  return new Intl.NumberFormat(getResolvedLocale()).format(value)
}

export function installLocaleFetchHeaders() {
  if (fetchHeadersInstalled || typeof window === 'undefined') return
  const nativeFetch = window.fetch.bind(window)
  window.fetch = (input: RequestInfo | URL, init?: RequestInit) => {
    if (!shouldAttachLocaleHeader(input)) {
      return nativeFetch(input, init)
    }
    const nextInit: RequestInit = { ...init }
    const headers = new Headers(init?.headers)
    if (!headers.has('X-Nova-Locale')) {
      headers.set('X-Nova-Locale', localeRequestHeader())
    }
    nextInit.headers = headers
    return nativeFetch(input, nextInit)
  }
  fetchHeadersInstalled = true
}

function normalizeLocaleCode(locale: string | undefined | null): LocaleCode {
  if (locale === 'zh-CN' || locale === 'en-US') return locale
  return 'auto'
}

function browserLocaleSource(): string {
  if (typeof navigator === 'undefined') return 'zh-CN'
  return navigator.languages?.[0] || navigator.language || 'zh-CN'
}

function shouldAttachLocaleHeader(input: RequestInfo | URL): boolean {
  if (typeof input === 'string') return input.startsWith('/') || input.startsWith(window.location.origin)
  if (input instanceof URL) return input.origin === window.location.origin
  return input.url.startsWith('/') || input.url.startsWith(window.location.origin)
}

i18next
  .use(initReactI18next)
  .init({
    resources: {
      'zh-CN': { translation: zhCN },
      'en-US': { translation: enUS },
    },
    lng: resolveLocale('auto'),
    fallbackLng: 'zh-CN',
    keySeparator: false,
    interpolation: { escapeValue: false },
  })
  .catch((error) => {
    console.error('[i18n] 初始化失败', error)
  })

setConfiguredLocale('auto')
installLocaleFetchHeaders()

export default i18next
