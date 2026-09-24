import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useTheme } from 'next-themes'
import { useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { StoryPicker, type StoryPickerProps } from '@/features/interactive/components/StoryPicker'
import { GamePlayer } from './GamePlayer'
import { management, managementBase, localized, platformError, type CatalogEntry, type Instance, type RuntimeSnapshot } from './api'

/** A storyline always resumes its saved release. Opening never creates a new save. */
export function InstalledGameStory({ instance, item, active, picker, onRefresh }: {
  instance: Instance
  item?: CatalogEntry
  active: boolean
  picker: StoryPickerProps
  onRefresh: () => void
}) {
  const { t, i18n } = useTranslation()
  const { resolvedTheme } = useTheme()
  const queryClient = useQueryClient()
  const [runtime, setRuntime] = useState<RuntimeSnapshot | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [upgrade, setUpgrade] = useState<string | null>(null)
  const release = item?.releases.find(release => release.ref.releaseId === instance.releaseId)
  const gameName = localized(release?.manifest.name, i18n.language) || t('platform.type.game')
  const open = async () => {
    setBusy(true); setError('')
    try { setRuntime(await management(`/instances/${instance.instanceId}/open`, 'POST', { locale: i18n.language, theme: resolvedTheme })) }
    catch (error) { setError(platformError(error)) }
    finally { setBusy(false) }
  }
  useEffect(() => {
    if (!active) return
    let cancelled = false
    setBusy(true)
    void management<RuntimeSnapshot>(`/instances/${instance.instanceId}/open`, 'POST', { locale: i18n.language, theme: resolvedTheme })
      .then(value => { if (!cancelled) { setRuntime(value); setError('') } })
      .catch(error => { if (!cancelled) setError(platformError(error)) })
      .finally(() => { if (!cancelled) setBusy(false) })
    return () => { cancelled = true }
    // Appearance changes are delivered to the frame without starting another runtime.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [instance.instanceId, active])
  const storyControls = <div className="flex flex-wrap items-center gap-2">
      <StoryPicker {...picker} />
      <Badge variant="outline" className="max-w-full truncate">{gameName}{release ? ' · ' + release.manifest.version : ''}</Badge>
      <Button size="sm" variant="ghost" asChild><a href={`${managementBase}/instances/${instance.instanceId}/export`}>{t('platform.exportSave')}</a></Button>
      {item && item.currentRelease !== instance.releaseId && <Button size="sm" variant="outline" disabled={busy || !item.enabled || item.removed || !!item.unavailableReason} onClick={() => { setError(''); setUpgrade(item.currentRelease) }}>{t('platform.github.useInstalledUpdate')}</Button>}
    </div>
  return <div className="flex min-h-0 flex-1 flex-col bg-background">
    {!runtime && <div className="border-b px-3 py-2">{storyControls}</div>}
    {!runtime && error && <InlineErrorNotice className="m-3" message={error} />}
    {runtime ? <GamePlayer runtime={runtime} title={instance.title} visible={active} menuContent={storyControls} onExit={() => setRuntime(null)} onOpenInstance={created => {
      queryClient.setQueryData<Instance[]>(['platform', 'instances'], previous => [...(previous ?? []).filter(item => item.instanceId !== created.instanceId), created])
      picker.onSelect('game:' + created.instanceId)
    }} /> : <div className="flex flex-1 flex-col items-center justify-center gap-4 p-6 text-center">
      <h2 className="break-words text-lg font-semibold">{instance.title}</h2>
      {(!item?.enabled || item.removed || item.unavailableReason) && <p className="max-w-md text-sm text-muted-foreground">{t('platform.savedGameUnavailable')}</p>}
      <Button disabled={busy || !item?.enabled || item.removed || !!item.unavailableReason} onClick={() => void open()}>{t(busy ? 'common.loading' : 'platform.continue')}</Button>
    </div>}
    <Dialog open={!!upgrade} onOpenChange={open => { if (!open && !busy) setUpgrade(null) }}>
      <DialogContent>
        <DialogHeader><DialogTitle>{t('platform.github.useInstalledUpdate')}</DialogTitle><DialogDescription>{t('platform.upgradeDescription')}</DialogDescription></DialogHeader>
        {error && <InlineErrorNotice message={error} />}
        <DialogFooter>
          <Button variant="outline" disabled={busy} onClick={() => setUpgrade(null)}>{t('common.cancel')}</Button>
          <Button disabled={busy} onClick={() => {
            setBusy(true)
            void management(`/instances/${instance.instanceId}/upgrade`, 'POST', { releaseId: upgrade })
              .then(() => { setUpgrade(null); setRuntime(null); onRefresh() })
              .catch(error => {
                console.error('[extensions] game update failed', { instanceId: instance.instanceId, releaseId: upgrade, error })
                setError(platformError(error))
              })
              .finally(() => setBusy(false))
          }}>{t('platform.confirm')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
}
