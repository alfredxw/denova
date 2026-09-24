import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { notifyAgentChatProjectUpdated } from '@/features/agent-chat/api'
import { management, platformError, type Candidate, type CatalogEntry, type DevelopmentSource, type GitHubUpdate } from './api'
import { openExtensionSource } from './extension-navigation'

/** Upstream checks never change the installed code or any saved consumer binding. */
export function GitHubInstallation({ item, disabled, onUpdate }: {
  item: CatalogEntry
  disabled: boolean
  onUpdate: (candidate: Candidate) => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [busy, setBusy] = useState(false)
  const [update, setUpdate] = useState<GitHubUpdate | null>(null)
  const [error, setError] = useState('')
  const source = item.source!
  const endpoint = `/packages/${item.kind}/${item.id}/update`
  const run = async (action: () => Promise<void>) => {
    setBusy(true); setError('')
    try { await action() } catch (error) {
      console.error('[extensions] GitHub operation failed', { packageId: item.id, error })
      setError(platformError(error))
    } finally { setBusy(false) }
  }
  return <div className="flex min-w-0 flex-col gap-3">
    <p className="break-all text-sm text-muted-foreground">
      <a href={source.url} target="_blank" rel="noreferrer" className="underline underline-offset-4">{source.url}</a>
      {' · '}{source.ref}{' · '}<span className="whitespace-nowrap">{source.commit?.slice(0, 7)}</span>{source.path !== '.' && <> · {source.path}</>}
    </p>
    <div className="flex flex-wrap gap-2">
      <Button variant="outline" disabled={busy} onClick={() => void run(async () => {
        const result = await management<GitHubUpdate>(endpoint)
        setUpdate(result)
        if (result.status === 'current') toast.info(t('platform.github.upToDate'))
      })}>{t(busy ? 'platform.github.loading' : 'platform.github.checkUpdates')}</Button>
      {update?.status === 'available' && <Button disabled={busy || disabled} onClick={() => void run(async () => {
        onUpdate(await management<Candidate>(endpoint, 'POST', { commit: update.source.commit }))
      })}>{t('platform.github.update')}</Button>}
      <Button variant="outline" disabled={busy || disabled} onClick={() => void run(async () => {
        const imported = await management<DevelopmentSource>('/packages/github/import', 'POST', update?.source ?? source)
        notifyAgentChatProjectUpdated(imported.projectId)
        await client.invalidateQueries({ queryKey: ['platform', 'development'] })
        await openExtensionSource(imported)
      })}>{t('platform.github.importSource')}</Button>
    </div>
    {disabled && <p className="text-sm text-muted-foreground">{t('platform.github.saveDraftFirst')}</p>}
    {update?.status === 'available' && <p role="status" className="text-sm text-muted-foreground">{t('platform.github.available', { commit: update.source.commit?.slice(0, 7) })}</p>}
    {error && <><InlineErrorNotice message={error} /><p className="text-sm text-muted-foreground">{t('platform.github.buildHelp')}</p></>}
  </div>
}
