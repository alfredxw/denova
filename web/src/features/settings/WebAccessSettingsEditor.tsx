import { useContext, useEffect, useId, useRef, useState } from 'react'
import { ExternalLink, Loader2, Plus, Search, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { APIError } from '@/lib/api-client'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { ApiKeyInput } from './ApiKeyInput'
import { Num, Text } from './SettingsFields'
import { SettingsDisclosureCard } from './SettingsDisclosureCard'
import { SettingsPageVisibility } from './SettingsPageFrame'
import { testWebSearch } from './api'
import type { SettingsFieldRequest } from './settings-sections'
import type { SearchServiceSettings, WebAccessSettings, WebSearchProvider, WebSearchTestResult } from './types'

const SERVICES = [
  { id: 'deepseek', name: 'DeepSeek', host: 'api.deepseek.com', docs: 'https://api-docs.deepseek.com/guides/search/' },
  { id: 'perplexity', name: 'Perplexity', host: 'api.perplexity.ai', docs: 'https://docs.perplexity.ai/docs/search/quickstart' },
  { id: 'tavily', name: 'Tavily', host: 'api.tavily.com', docs: 'https://docs.tavily.com/documentation/quickstart' },
  { id: 'keenable', name: 'Keenable', host: 'api.keenable.ai', docs: 'https://docs.keenable.ai/' },
  { id: 'searxng', name: 'SearXNG', host: '', docs: 'https://docs.searxng.org/' },
] as const

type HostedService = 'deepseek' | 'perplexity' | 'tavily' | 'keenable'

const EMPTY_SETTINGS: WebAccessSettings = {}
const ADVANCED_FIELDS = ['searchMaxResults', 'searchProviderTimeoutSeconds', 'fallback', 'fetchMaxResponseKB', 'fetchMaxContentChars']

/** User-scoped search routing shared by writing and game. Saved credentials do
 * not imply a verified connection; preview results belong to one exact draft. */
export function WebAccessSettingsEditor({ value = EMPTY_SETTINGS, inherited = EMPTY_SETTINGS, onChange, fieldRequest }: {
  value?: WebAccessSettings
  inherited?: WebAccessSettings
  onChange: (settings: WebAccessSettings) => void
  fieldRequest?: SettingsFieldRequest
}) {
  const { t, i18n } = useTranslation()
  const visible = useContext(SettingsPageVisibility)
  const id = useId()
  const [addingSearXNG, setAddingSearXNG] = useState(false)
  const [query, setQuery] = useState('')
  const [result, setResult] = useState<WebSearchTestResult | null>(null)
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  const requestRef = useRef<AbortController | null>(null)
  const settings = { ...inherited, ...value }
  const selected = settings.search_provider || (settings.searxng_base_url?.trim() ? 'searxng' : 'builtin')
  const fallback = settings.search_fallback || (settings.search_provider ? 'none' : 'builtin')
  const fingerprint = JSON.stringify(settings)
  const fieldKey = fieldRequest?.fieldKey || ''
  const added = SERVICES.filter(service => service.id === 'searxng'
    ? addingSearXNG || Boolean(settings.searxng_base_url) || selected === 'searxng'
    : Boolean(settings[service.id]) || selected === service.id)
  const available = SERVICES.filter(service => !added.includes(service))
  const providerLabel = (provider: string) => provider === 'builtin' ? t('settings.webAccess.builtin')
    : SERVICES.find(service => service.id === provider)?.name || provider

  useEffect(() => {
    requestRef.current?.abort()
    requestRef.current = null
    setPending(false)
    setResult(null)
    setError('')
    return () => { requestRef.current?.abort() }
  }, [fingerprint, query, visible, i18n.language])

  const change = <K extends keyof WebAccessSettings>(key: K, next: WebAccessSettings[K]) => onChange({ ...value, [key]: next })
  const changeService = (service: HostedService, patch: SearchServiceSettings) =>
    change(service, { ...settings[service], ...patch })
  const add = (service: typeof SERVICES[number]['id']) => {
    if (service === 'searxng') setAddingSearXNG(true)
    // Adding a service must not silently activate legacy SearXNG auto-selection.
    onChange({ ...value, search_provider: selected, search_fallback: settings.search_fallback || (selected === 'searxng' ? 'builtin' : 'none'),
      ...(service === 'searxng' ? {} : { [service]: {} }) })
  }
  const remove = (service: typeof SERVICES[number]['id']) => {
    if (service === 'searxng') { setAddingSearXNG(false); change('searxng_base_url', '') }
    else change(service, undefined)
  }
  const test = async () => {
    if (!query.trim() || pending) return
    const request = new AbortController()
    requestRef.current?.abort()
    requestRef.current = request
    setPending(true)
    setError('')
    setResult(null)
    try {
      const response = await testWebSearch(settings, query.trim(), request.signal)
      if (!request.signal.aborted) setResult(response)
    } catch (cause) {
      if (!request.signal.aborted) {
        console.warn('[settings] search preview failed', { provider: selected, code: cause instanceof APIError ? cause.code : 'network' })
        setError(cause instanceof APIError ? cause.summary : t('settings.webAccess.testFailed'))
      }
    } finally {
      if (requestRef.current === request) { requestRef.current = null; setPending(false) }
    }
  }
  const placeholder = (key: keyof WebAccessSettings) => t('common.defaultValue', { value: String(inherited[key] ?? '') })

  return (
    <div className="space-y-4" data-web-search-settings>
      <SearchSelect label={t('settings.webAccess.defaultService')} description={t('settings.webAccess.description')}
        value={selected} options={['builtin', ...added.map(service => service.id)].map(provider => ({ value: provider, label: providerLabel(provider) }))}
        onChange={provider => onChange({ ...value, search_provider: provider as WebSearchProvider, search_fallback: settings.search_fallback || 'none' })} />

      <div className="space-y-2">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h3 className="text-xs font-semibold">{t('settings.webAccess.services')}</h3>
          <DropdownMenu>
            <DropdownMenuTrigger asChild><Button variant="outline" size="sm" disabled={!available.length}><Plus />{t('settings.webAccess.addService')}</Button></DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {available.map(service => <DropdownMenuItem key={service.id} onSelect={() => add(service.id)}>{service.name}</DropdownMenuItem>)}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
        <div className="flex flex-wrap items-center gap-2 rounded-[var(--nova-radius)] border border-border bg-muted/30 px-3 py-2.5">
          <Badge variant="secondary">{t(selected === 'builtin' ? 'settings.webAccess.defaultBadge' : 'settings.webAccess.freeBadge')}</Badge>
          <div className="min-w-0 flex-1"><p className="text-xs font-semibold">{t('settings.webAccess.builtin')}</p><p className="text-[11px] text-muted-foreground">DuckDuckGo + Bing</p></div>
          <span className="text-[11px] text-muted-foreground">{t('settings.webAccess.noKey')}</span>
        </div>
        {!added.length && <p className="px-1 text-xs leading-5 text-muted-foreground">{t('settings.webAccess.emptyServices')}</p>}
        {added.map(service => {
          // Keenable works without a key (keyless endpoint), so it is usable as soon as it is added.
          const configured = service.id === 'keenable' || Boolean(service.id === 'searxng' ? settings.searxng_base_url?.trim() : settings[service.id]?.api_key?.trim())
          const serviceField = `settings.webAccess.${service.id}Key`
          const reveal = fieldRequest && (fieldKey === serviceField || (service.id === 'searxng' && fieldKey.endsWith('.searxngBaseUrl')) || (service.id !== 'searxng' && fieldKey.endsWith('.mode')))
          return (
            <SettingsDisclosureCard key={service.id} level="connection" badge={t(selected === service.id ? 'settings.webAccess.defaultBadge' : 'settings.webAccess.serviceBadge')}
              title={service.name} subtitle={`${t(configured ? 'settings.webAccess.configured' : 'settings.webAccess.needsConfiguration')} · ${service.host || settings.searxng_base_url || t('settings.webAccess.selfHosted')}`}
              defaultOpen={!configured} revealRequest={reveal ? fieldRequest : undefined}
              actions={<Button variant="ghost" size="icon-sm" aria-label={t('settings.webAccess.removeService', { service: service.name })}
                title={t(selected === service.id ? 'settings.webAccess.removeDefaultHint' : 'settings.webAccess.removeService', { service: service.name })}
                disabled={selected === service.id} onClick={() => remove(service.id)}><Trash2 className="size-3.5" /></Button>}>
              <FieldGroup className="gap-3 p-3">
                {service.id === 'searxng' ? <Text label={t('settings.webAccess.searxngBaseUrl')} value={value.searxng_base_url}
                  placeholder="https://search.example.com" onChange={url => change('searxng_base_url', url)} /> : <>
                  <Field><FieldLabel>{t(serviceField)}</FieldLabel><ApiKeyInput label={t(serviceField)} value={settings[service.id]?.api_key || ''}
                    placeholder={t(service.id === 'keenable' ? 'settings.webAccess.optionalKeyPlaceholder' : 'settings.webAccess.keyPlaceholder')} onChange={api_key => changeService(service.id as HostedService, { api_key })} /></Field>
                  {service.id === 'deepseek' || service.id === 'keenable' ? <FieldDescription>{t(`settings.webAccess.${service.id}Hint`)}</FieldDescription> : <SearchSelect
                    label={t('settings.webAccess.mode')} value={settings[service.id]?.mode || (service.id === 'perplexity' ? 'web' : 'basic')}
                    description={t(service.id === 'perplexity' ? 'settings.webAccess.perplexityHint' : 'settings.webAccess.tavilyHint')}
                    options={(service.id === 'perplexity' ? ['web', 'fast'] : ['basic', 'advanced']).map(mode => ({ value: mode, label: t(`settings.webAccess.mode.${mode}`) }))}
                    onChange={mode => changeService(service.id as 'perplexity' | 'tavily', { mode: mode as SearchServiceSettings['mode'] })} />}
                </>}
                <a className="inline-flex w-fit items-center gap-1 text-xs text-muted-foreground underline underline-offset-4 hover:text-foreground" href={service.docs} target="_blank" rel="noreferrer">
                  {t('settings.webAccess.serviceDocs', { service: service.name })}<ExternalLink className="size-3" aria-hidden="true" />
                </a>
              </FieldGroup>
            </SettingsDisclosureCard>
          )
        })}
      </div>

      <div className="space-y-3 rounded-[var(--nova-radius)] border border-border p-3">
        <Field>
          <FieldLabel htmlFor={`${id}-query`}>{t('settings.webAccess.testTitle')}</FieldLabel>
          <FieldDescription>{t('settings.webAccess.testHint', { service: providerLabel(selected) })}
            {selected !== 'builtin' && selected !== 'searxng' && ` ${t('settings.webAccess.testChargeHint')}`}</FieldDescription>
          <div className="flex flex-col gap-2 sm:flex-row">
            <Input id={`${id}-query`} value={query} placeholder={t('settings.webAccess.queryPlaceholder')} className="min-w-0 flex-1"
              onChange={event => setQuery(event.target.value)} onKeyDown={event => { if (event.key === 'Enter' && !event.nativeEvent.isComposing) { event.preventDefault(); void test() } }} />
            <Button variant="outline" disabled={pending || !query.trim()} onClick={() => void test()}>
              {pending ? <Loader2 className="animate-spin" /> : <Search />}{t(pending ? 'settings.webAccess.testing' : 'settings.webAccess.testButton')}
            </Button>
            {pending && <Button variant="ghost" onClick={() => { requestRef.current?.abort(); requestRef.current = null; setPending(false) }}>{t('common.cancel')}</Button>}
          </div>
        </Field>
        <div aria-live="polite" aria-atomic="true">
          {error && <p role="alert" className="break-words text-xs text-destructive">{error}</p>}
          {result && <p className="text-xs text-muted-foreground">{t(result.status === 'no_results' ? 'settings.webAccess.noResults' : 'settings.webAccess.testSucceeded', {
            service: providerLabel(result.provider), count: result.results?.length || 0, ms: result.elapsed_ms,
          })}</p>}
        </div>
        {!!result?.results?.length && <ol className="space-y-3">
          {result.results.map(item => <li key={item.url} className="min-w-0 border-t border-border pt-3">
            <a href={item.url} target="_blank" rel="noreferrer" className="break-words text-xs font-medium underline underline-offset-4">{item.title}</a>
            <p className="mt-1 truncate text-[11px] text-muted-foreground">{item.url}</p>
            {item.summary && <p className="mt-1 line-clamp-3 break-words text-xs leading-5 text-muted-foreground">{item.summary}</p>}
          </li>)}
        </ol>}
      </div>

      <SettingsDisclosureCard level="connection" badge={t('settings.webAccess.advancedBadge')} title={t('settings.webAccess.advancedTitle')}
        subtitle={t('settings.webAccess.advancedHint')} defaultOpen={false}
        revealRequest={ADVANCED_FIELDS.some(key => fieldKey === `settings.webAccess.${key}`) ? fieldRequest : undefined}>
        <div className="space-y-3 p-3">
          <SearchSelect label={t('settings.webAccess.fallback')} value={fallback} description={t(selected === 'searxng' && !settings.search_provider && !settings.search_fallback
            ? 'settings.webAccess.legacyFallbackHint' : 'settings.webAccess.fallbackHint')}
            options={['none', 'builtin'].map(option => ({ value: option, label: t(`settings.webAccess.fallback.${option}`) }))}
            onChange={option => onChange({ ...value, search_provider: selected, search_fallback: option as 'none' | 'builtin' })} />
          <Num label={t('settings.webAccess.searchMaxResults')} value={value.search_max_results ?? null} placeholder={placeholder('search_max_results')}
            min={1} max={20} onChange={next => change('search_max_results', next)} />
          <Num label={t('settings.webAccess.searchProviderTimeoutSeconds')} value={value.search_provider_timeout_seconds ?? null} placeholder={placeholder('search_provider_timeout_seconds')}
            min={0} onChange={next => change('search_provider_timeout_seconds', next)} />
          <div className="border-t border-border pt-3">
            <Num label={t('settings.webAccess.fetchMaxResponseKB')} value={value.fetch_max_response_kb ?? null} placeholder={placeholder('fetch_max_response_kb')}
              min={1} max={65536} onChange={next => change('fetch_max_response_kb', next)} />
            <Num label={t('settings.webAccess.fetchMaxContentChars')} value={value.fetch_max_content_chars ?? null} placeholder={placeholder('fetch_max_content_chars')}
              min={1} max={262144} onChange={next => change('fetch_max_content_chars', next)} />
            <p className="mt-2 text-xs leading-5 text-muted-foreground">{t('settings.webAccess.hint')}</p>
          </div>
        </div>
      </SettingsDisclosureCard>
    </div>
  )
}

function SearchSelect({ label, description, value, options, onChange }: {
  label: string; description?: string; value: string; options: { value: string; label: string }[]; onChange: (value: string) => void
}) {
  const id = useId()
  return <Field>
    <FieldLabel htmlFor={id}>{label}</FieldLabel>
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger id={id} className="w-full"><SelectValue /></SelectTrigger>
      <SelectContent><SelectGroup>{options.map(option => <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>)}</SelectGroup></SelectContent>
    </Select>
    {description && <FieldDescription>{description}</FieldDescription>}
  </Field>
}
