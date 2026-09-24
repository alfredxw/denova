import { useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
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
  const query = useQuery({ queryKey, queryFn: () => management<ConfigurationDocument>(`${endpoint}?releaseId=${releaseId}&locale=${locale}`), enabled: active, placeholderData: previous => previous?.releaseId === releaseId ? previous : undefined, staleTime: 0, refetchOnWindowFocus: false })
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
  const [baseline, setBaseline] = useState(initial)
  const observed = useRef(initial)
  const [values, setValues] = useState(initial.values)
  const [busy, setBusy] = useState(false)
  const [problem, setProblem] = useState<ConfigurationProblem | undefined>(initial.problem)
  const [error, setError] = useState('')
  const dirty = Boolean(baseline.problem) || JSON.stringify(values) !== JSON.stringify(baseline.values)
  useEffect(() => onDirtyChange?.(dirty || busy), [busy, dirty, onDirtyChange])
  useEffect(() => {
    if (observed.current === initial) return
    observed.current = initial
    // Background refreshes may update a clean editor, but never rebase a user's draft.
    if (dirty || busy) return
    setBaseline(initial); setValues(initial.values); setProblem(initial.problem)
  }, [busy, dirty, initial])
  const submit = async () => {
    setBusy(true); setError(''); setProblem(undefined)
    try {
      const result = await management<ConfigurationDocument>(`${endpoint}?locale=${locale}`, 'PUT', {
        releaseId: baseline.releaseId,
        expectedRevision: baseline.revision,
        overrides: configurationOverrides(values, baseline.form?.defaults ?? {}),
      })
      setValues(result.values)
      setBaseline(result)
      onSaved(result)
    } catch (cause) {
      console.error('[extensions] configuration operation failed', { endpoint, cause })
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
        {dirty && <Button variant="ghost" size="sm" disabled={busy} onClick={() => { setValues(baseline.values); setProblem(baseline.problem); setError('') }}>{t('common.cancel')}</Button>}
        <Button variant="outline" size="sm" disabled={busy || !dirty} onClick={() => void submit()}>{t('platform.settings.save')}</Button>
        <Button variant="ghost" size="sm" disabled={busy} onClick={() => { setValues(initial.form?.defaults ?? {}); setError('') }}>{t('platform.settings.restoreDefaults')}</Button>
      </div>
    </div>
    {initial.form && <ConfigurationForm definition={initial.form} values={values} disabled={busy} onChange={setValues} />}
    {problem && <InlineErrorNotice message={[t(problem.messageKey), ...(problem.fields ?? []).map(field => `${field.path?.join('.') || t('platform.settings.form')}: ${t('platform.settings.invalidValue')}`)].filter(Boolean).join(' ')} />}
    {error && <InlineErrorNotice message={error} />}
    {dirty && <p role="status" className="text-xs text-muted-foreground">{t('platform.settings.unsaved')}</p>}
  </div>
}
