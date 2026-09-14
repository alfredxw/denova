import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, Download, FolderOpen, Gamepad2, MoreHorizontal, Play, Puzzle, Square, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import { Separator } from '@/components/ui/separator'
import { Field, FieldLabel } from '@/components/ui/field'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { useWorkspaceStore } from '@/stores/workspace-store'
import { management, localized, platformError, type Candidate, type RuntimeSnapshot } from './api'
import { type ExtensionEntry } from './extension-directory'
import { openExtensionSource, startExtensionGame } from './extension-navigation'
import { InstalledExtensionSettings } from './InstalledExtensionSettings'
import { GitHubInstallation } from './GitHubInstallation'

export function ExtensionDetails({ entry, runtimes, active, dirty, onDirtyChange, onRefresh, onUpdate }: {
  entry: ExtensionEntry
  runtimes: RuntimeSnapshot[]
  active: boolean
  dirty: boolean
  /** Keep this editor mounted while it has unsaved changes or a pending save. */
  onDirtyChange: (dirty: boolean) => void
  onRefresh: () => void
  onUpdate: (candidate: Candidate) => void
}) {
  const { t, i18n } = useTranslation()
  const { installed: item, manifest, sources } = entry
  const [confirm, setConfirm] = useState(false)
  const enabledId = useId()
  const [busy, setBusy] = useState(false)
  const [failedCover, setFailedCover] = useState('')
  const current = item.releases.find(release => release.ref.releaseId === item.currentRelease)
  const activeRuntimes = runtimes.filter(runtime => runtime.context.environment === 'installed' && runtime.context.source.package.kind === item.kind && runtime.context.source.package.id === item.id)
  const name = localized(manifest?.name, i18n.language) || item.id
  const description = localized(manifest?.description, i18n.language)
  const Icon = item.kind === 'game' ? Gamepad2 : Puzzle
  const cover = item.kind === 'game' && manifest?.game?.cover && current
    ? `/api/platform/manage/packages/game/${encodeURIComponent(item.id)}/cover?releaseId=${encodeURIComponent(current.ref.releaseId)}` : ''
  const showCover = Boolean(cover && failedCover !== cover)
  const run = async (action: () => Promise<void>) => {
    setBusy(true)
    try { await action() } catch (cause) {
      console.error('[extensions] installed package operation failed', { kind: item.kind, packageId: item.id, cause })
      toast.error(platformError(cause))
    } finally { setBusy(false) }
  }
  return <article className="mx-auto flex w-full max-w-6xl flex-col gap-5 p-4 @xl:p-6 @4xl:p-8">
    <header className="flex items-start gap-4 @xl:gap-6">
      {showCover && <img src={cover} alt={t('platform.extensions.cover', { name })} className="aspect-[3/4] w-[clamp(6rem,18cqw,10rem)] shrink-0 rounded-lg border object-cover" onError={() => {
        console.error('[extensions] loading game cover failed', { packageId: item.id, releaseId: current?.ref.releaseId })
        setFailedCover(cover)
      }} />}
      <div className="grid min-w-0 flex-1 gap-x-6 gap-y-4 @xl:grid-cols-[minmax(0,1fr)_auto]">
        <div className="flex flex-wrap items-start gap-x-4 gap-y-3 @xl:col-span-2">
          <div className="flex min-w-[min(100%,12rem)] flex-1 items-center gap-3">
            {!showCover && <div className="flex size-10 shrink-0 items-center justify-center rounded-lg border bg-muted/50"><Icon className="size-5 text-muted-foreground" aria-hidden="true" /></div>}
            <h1 className="text-xl font-semibold tracking-tight [overflow-wrap:anywhere] @xl:text-3xl">{name}</h1>
          </div>
          <div className="ml-auto flex items-center gap-2">
            <Field orientation="horizontal" className="w-auto shrink-0 gap-2">
              <FieldLabel htmlFor={enabledId} className="text-xs text-muted-foreground">{t(item.enabled ? 'platform.extensions.enabled' : 'platform.disabled')}</FieldLabel>
              <Switch id={enabledId} checked={item.enabled} disabled={busy} aria-label={t('platform.enableNamed', { name })} onCheckedChange={enabled => void run(async () => {
                await management(`/packages/${item.kind}/${item.id}`, 'PATCH', { enabled }); onRefresh()
                if (!enabled) toast.info(t('platform.disableKeepsRunning'))
              })} />
            </Field>
            <DropdownMenu>
              <DropdownMenuTrigger asChild><Button variant="ghost" size="icon-sm" disabled={busy} aria-label={t('platform.extensions.manageNamed', { name })}><MoreHorizontal /></Button></DropdownMenuTrigger>
              <DropdownMenuContent align="end"><DropdownMenuGroup><DropdownMenuItem variant="destructive" onSelect={() => setConfirm(true)}><Trash2 />{t('platform.uninstall')}</DropdownMenuItem></DropdownMenuGroup></DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>
        <div className={'flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground' + (item.kind === 'game' ? ' @xl:col-span-2' : '')}>
          <span>{t('platform.type.' + item.kind)}</span>
          {manifest && <><span aria-hidden="true">·</span><span>v{manifest.version}</span></>}
          <span aria-hidden="true">·</span><span className="select-text [overflow-wrap:anywhere]">{item.id}</span>
        </div>
        <div className={'flex flex-wrap items-center gap-2' + (item.kind === 'game' ? ' @xl:col-span-2' : ' @xl:justify-end')}>
          {item.kind === 'game' && <Button size="lg" className="h-10 px-5 @xl:min-w-36" disabled={busy || !item.enabled || Boolean(item.unavailableReason) || !current} onClick={() => startExtensionGame(item.id)}><Play data-icon="inline-start" />{t('platform.extensions.startGame')}</Button>}
          {sources.length === 1 && <Button variant="outline" className="h-10 px-4 @xl:min-w-32" disabled={busy} onClick={() => void run(() => openExtensionSource(sources[0]))}><FolderOpen data-icon="inline-start" />{t('platform.openSource')}</Button>}
          {sources.length > 1 && <DropdownMenu><DropdownMenuTrigger asChild><Button variant="outline" size="sm" disabled={busy}><FolderOpen data-icon="inline-start" />{t('platform.openSource')}<ChevronDown data-icon="inline-end" /></Button></DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="max-w-[min(22rem,90vw)]"><DropdownMenuGroup>{sources.map(source => <DropdownMenuItem key={source.developmentId} className="break-all" onSelect={() => void run(() => openExtensionSource(source))}>{source.projectName} · {source.relativePath}</DropdownMenuItem>)}</DropdownMenuGroup></DropdownMenuContent>
          </DropdownMenu>}
          {sources.length === 0 && <Button variant="outline" size="sm" title={t('platform.extensions.noSourceHelp')} onClick={() => useWorkspaceStore.getState().setMode('agentchat')}><FolderOpen data-icon="inline-start" />{t('platform.openWorkbench')}</Button>}
          {current && <Button variant="outline" asChild>
            <a href={`/api/platform/manage/packages/${item.kind}/${encodeURIComponent(item.id)}/archive?releaseId=${encodeURIComponent(current.ref.releaseId)}`}><Download data-icon="inline-start" />{t('platform.exportPackage')}</a>
          </Button>}
        </div>
        {item.kind === 'game' && <p className="text-xs leading-relaxed text-muted-foreground @xl:col-span-2">{t('platform.extensions.startGameHelp')}</p>}
      </div>
    </header>
    {item.source && <GitHubInstallation key={`${item.currentRelease}:${JSON.stringify(item.source)}`} item={item} disabled={dirty} onUpdate={onUpdate} />}
    {item.unavailableReason && <InlineErrorNotice message={t(item.unavailableReason)} />}
    <Separator />
    <section className="flex min-w-0 flex-col gap-2" aria-label={t('platform.extensions.introduction.' + item.kind)}>
      <h2 className="text-sm font-semibold">{t('platform.extensions.introduction.' + item.kind)}</h2>
      <p className="whitespace-pre-wrap text-sm leading-7 text-muted-foreground [overflow-wrap:anywhere]">{description || t(item.kind === 'game' ? 'platform.extensions.gameHelp' : 'platform.extensions.pluginHelp')}</p>
      {item.kind === 'plugin' && Boolean(manifest?.contributes?.tools?.length) && <p className="text-sm leading-7 text-muted-foreground">{t('platform.extensions.pluginToolsHelp')}</p>}
    </section>
    <Separator />
    {current && <section aria-label={t('platform.usageSettings')}>
      <InstalledExtensionSettings key={current.ref.releaseId} item={item} release={current} active={active} onDirtyChange={onDirtyChange} onSaved={() => { onRefresh(); toast.success(t('platform.settingsSaved')) }} />
    </section>}
    {activeRuntimes.length > 0 && <><Separator /><section className="flex flex-wrap items-center justify-between gap-3">
      <div className="flex min-w-0 flex-col gap-1"><h2 className="text-sm font-semibold">{t('platform.runningCount', { count: activeRuntimes.length })}</h2><p className="text-xs text-muted-foreground">{t('platform.extensions.runningHelp')}</p></div>
      <Button variant="outline" size="sm" disabled={busy} onClick={() => void run(async () => {
        const results = await Promise.allSettled(activeRuntimes.map(runtime => management(`/runtimes/${runtime.id}/stop`, 'POST', {})))
        onRefresh()
        const failure = results.find(result => result.status === 'rejected')
        if (failure?.status === 'rejected') throw failure.reason
      })}><Square data-icon="inline-start" />{t('platform.stopRunning')}</Button>
    </section></>}
    {Boolean(manifest?.requires?.length) && <><Separator /><section className="flex flex-col gap-2">
      <h2 className="text-sm font-semibold">{t('platform.dependency')}</h2>
      <ul className="divide-y">{manifest?.requires?.map(dependency => <li key={dependency.pluginId} className="flex flex-wrap items-center justify-between gap-2 py-3 text-xs"><span className="[overflow-wrap:anywhere]">{dependency.pluginId}</span><Badge variant="outline">{dependency.versionRange}</Badge></li>)}</ul>
    </section></>}
    <Dialog open={confirm} onOpenChange={value => { if (!busy) setConfirm(value) }}>
      <DialogContent>
        <DialogHeader><DialogTitle>{t('platform.uninstall')}</DialogTitle><DialogDescription>{t('platform.uninstallDescription')}</DialogDescription></DialogHeader>
        <DialogFooter>
          <Button variant="outline" disabled={busy} onClick={() => setConfirm(false)}>{t('common.cancel')}</Button>
          <Button variant="destructive" disabled={busy} onClick={() => void run(async () => {
            await management(`/packages/${item.kind}/${item.id}`, 'PATCH', { enabled: false, removed: true })
            setConfirm(false); onDirtyChange(false); onRefresh()
          })}>{t('platform.uninstall')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </article>
}
