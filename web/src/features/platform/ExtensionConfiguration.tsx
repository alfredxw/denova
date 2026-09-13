import { useEffect, useId, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { MoreHorizontal } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Textarea } from '@/components/ui/textarea'
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuCheckboxItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { APIError } from '@/lib/api-client/client'
import { management, platformError, type ConfigurationDocument, type ConfigurationProblem, type PackageKind } from './api'
import { ConfigurationForm, configurationOverrides } from './ConfigurationForm'

export function ExtensionConfiguration({ kind, packageId, releaseId, active = true, onDirtyChange, onSaved }: {
  kind: PackageKind; packageId: string; releaseId: string; active?: boolean; onDirtyChange?: (dirty: boolean) => void; onSaved: () => void
}) {
  const { t, i18n } = useTranslation()
  const client = useQueryClient()
  const endpoint = `/packages/${kind}/${packageId}/settings`
  const locale = i18n.language.startsWith('zh') ? 'zh-CN' : 'en-US'
  const queryKey = ['platform', 'settings', kind, packageId, releaseId, locale]
  const query = useQuery({ queryKey, queryFn: () => management<ConfigurationDocument>(`${endpoint}?locale=${locale}`), enabled: active, placeholderData: previous => previous, staleTime: 0, refetchOnWindowFocus: false })
  return <section aria-label={t('platform.settings.' + kind)} className="flex min-w-0 flex-col gap-3">
    {query.isPending ? <><h2 className="text-sm font-semibold">{t('platform.settings.' + kind)}</h2><p role="status">{t('common.loading')}</p></> : !query.data ? <InlineErrorNotice message={platformError(query.error)} /> : <>
    {query.error && <InlineErrorNotice message={platformError(query.error)} />}
    <ConfigurationEditor key={releaseId} kind={kind} initial={query.data} endpoint={endpoint} locale={locale} onDirtyChange={onDirtyChange} onSaved={saved => { client.setQueryData(queryKey, saved); onSaved() }} />
    </>}
  </section>
}

function ConfigurationEditor({ kind, initial, endpoint, locale, onDirtyChange, onSaved }: {
  kind: PackageKind; initial: ConfigurationDocument; endpoint: string; locale: string; onDirtyChange?: (dirty: boolean) => void; onSaved: (saved: ConfigurationDocument) => void
}) {
  const { t } = useTranslation()
  const tomlId = useId()
  const [baseline, setBaseline] = useState(initial)
  const observed = useRef(initial)
  const [mode, setMode] = useState<'form' | 'toml'>(initial.problem ? 'toml' : 'form')
  const [values, setValues] = useState(initial.values)
  const [toml, setToml] = useState(initial.toml)
  const [busy, setBusy] = useState(false)
  const [problem, setProblem] = useState<ConfigurationProblem | undefined>(initial.problem)
  const [error, setError] = useState('')
  const dirty = mode === 'form' ? JSON.stringify(values) !== JSON.stringify(baseline.values) : toml !== baseline.toml
  useEffect(() => onDirtyChange?.(dirty || busy), [busy, dirty, onDirtyChange])
  useEffect(() => {
    if (observed.current === initial) return
    observed.current = initial
    // Background refreshes may update a clean editor, but never rebase a user's draft.
    if (dirty || busy) return
    setBaseline(initial); setValues(initial.values); setToml(initial.toml); setProblem(initial.problem)
  }, [busy, dirty, initial])
  const submit = async (action: 'save' | 'convert') => {
    setBusy(true); setError(''); setProblem(undefined)
    try {
      const result = await management<ConfigurationDocument>(`${endpoint}${action === 'convert' ? '/validate' : ''}?locale=${locale}`, action === 'save' ? 'PUT' : 'POST', {
        releaseId: baseline.releaseId,
        expectedRevision: baseline.revision,
        ...(mode === 'toml'
          ? { format: 'toml', toml }
          : { format: 'values', overrides: configurationOverrides(values, baseline.form?.defaults ?? {}) }),
      })
      setValues(result.values); setToml(result.toml)
      if (action === 'save') {
        setBaseline(result)
        onSaved(result)
      }
      else setMode(mode === 'form' ? 'toml' : 'form')
    } catch (cause) {
      console.error('[extensions] configuration operation failed', { endpoint, action, cause })
      if (cause instanceof APIError && typeof cause.payload.messageKey === 'string') setProblem(cause.payload as unknown as ConfigurationProblem)
      else setError(platformError(cause))
    } finally { setBusy(false) }
  }
  const heading = <h2 className="text-sm font-semibold">{t('platform.settings.' + kind)}</h2>
  if (!initial.form && !initial.problem) return <>{heading}<p className="text-sm text-muted-foreground">{t('platform.settings.empty')}</p></>
  return <div className="flex min-w-0 flex-col gap-3">
    <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
      {heading}
      <div className="ml-auto flex flex-wrap items-center gap-2">
        <p className="mr-1 text-xs text-muted-foreground" title={t('platform.settings.help')}>{t(kind === 'plugin' ? 'platform.settings.effectiveNextTask' : 'platform.settings.effectiveNextStart')}</p>
        {dirty && <Button variant="ghost" size="sm" disabled={busy} onClick={() => { setValues(baseline.values); setToml(baseline.toml); setProblem(baseline.problem); setError('') }}>{t('common.cancel')}</Button>}
        <Button variant="outline" size="sm" disabled={busy || !dirty} onClick={() => void submit('save')}>{t('platform.settings.save')}</Button>
        <DropdownMenu><DropdownMenuTrigger asChild><Button variant="ghost" size="icon-sm" aria-label={t('platform.settings.more')} disabled={busy}><MoreHorizontal /></Button></DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuGroup aria-label={t('platform.settings.editor')}>
              <DropdownMenuCheckboxItem checked={mode === 'form'} onCheckedChange={() => { if (mode !== 'form') void submit('convert') }}>{t('platform.settings.form')}</DropdownMenuCheckboxItem>
              <DropdownMenuCheckboxItem checked={mode === 'toml'} onCheckedChange={() => { if (mode !== 'toml') void submit('convert') }}>TOML</DropdownMenuCheckboxItem>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuGroup><DropdownMenuItem onSelect={() => { setValues(initial.form?.defaults ?? {}); setToml(''); setProblem(undefined); setError('') }}>{t('platform.settings.restoreDefaults')}</DropdownMenuItem></DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
    {mode === 'form' && initial.form ? <ConfigurationForm definition={initial.form} values={values} disabled={busy} onChange={setValues} />
      : <Field><FieldLabel htmlFor={tomlId}>{t('platform.settings.toml')}</FieldLabel>
        <Textarea id={tomlId} rows={12} spellCheck={false} value={toml} disabled={busy} onChange={event => setToml(event.target.value)} aria-invalid={Boolean(problem)} />
        <FieldDescription>{t('platform.settings.tomlHelp')}</FieldDescription></Field>}
    {problem && <InlineErrorNotice message={[t(problem.messageKey), problem.version ? t('platform.settings.conflictVersion', { version: problem.version }) : '', ...(problem.fields ?? []).map(field => `${field.path?.join('.') || t('platform.settings.form')}: ${t('platform.settings.invalidValue')}`)].filter(Boolean).join(' ')} />}
    {error && <InlineErrorNotice message={error} />}
    {dirty && <p role="status" className="text-xs text-muted-foreground">{t('platform.settings.unsaved')}</p>}
  </div>
}
