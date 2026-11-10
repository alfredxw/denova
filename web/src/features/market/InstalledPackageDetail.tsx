import { useLayoutEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ArchiveRestore, ArrowLeft, FileArchive, Link2, MoreHorizontal, Package, RefreshCw, Settings2, Unlink, Upload } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { getBooks } from '@/lib/api'
import { localized, type CatalogEntry } from '@/features/platform/api'
import { openInstalledExtension } from '@/features/platform/extension-navigation'
import { InstalledExtensionToggle } from '@/features/platform/InstalledExtensionToggle'
import { GameDefaultsDialog } from '@/features/interactive/components/GameDefaultsDialog'
import { exchange, type Installation } from './api'
import { BackupDialog } from './BackupDialog'
import type { ImportDialogProps } from './ImportDialog'

/** Reads the current installation snapshot; selection and return navigation belong to ResourceCenterView. */
export function InstalledPackageDetail({ installation: item, extensions, blockedReason, onBack, onChanged, onCheckUpdate, onImport, onExport }: {
  installation: Installation
  extensions: CatalogEntry[]
  blockedReason?: string
  onBack: () => void
  onChanged: () => Promise<void>
  onCheckUpdate: (installation: Installation) => Promise<void>
  onImport: (props: Omit<ImportDialogProps, 'onClose' | 'onInstalled'>) => void
  onExport: (props: { installation: Installation }) => void
}) {
  const { t, i18n } = useTranslation()
  const heading = useRef<HTMLHeadingElement>(null)
  const [pending, setPending] = useState(false)
  const [dialog, setDialog] = useState<'backup' | 'defaults'>()
  const books = useQuery({ queryKey: ['resources', 'books'], queryFn: getBooks, enabled: !!item.project_id })
  const translation = item.package.translations?.[i18n.language]
  const name = translation?.name || item.package.name
  const description = translation?.description || item.package.description
  const usage = translation?.usage || item.package.usage
  const compatibility = translation?.compatibility || item.package.compatibility
  const canCheckUpdate = item.tracking === 'tracked' && item.source.kind !== 'file'
  const targetName = item.project_id
    ? books.data?.find(book => book.project_id === item.project_id)?.name || item.project_id
    : t('market.import.global')
  useLayoutEffect(() => { heading.current?.focus({ preventScroll: true }) }, [item.installation_id])

  const run = async (operation: string, action: () => Promise<void>) => {
    setPending(true)
    try {
      await action()
    } catch (error) {
      console.error('[market] installed package action failed', { operation, installationID: item.installation_id, error })
      toast.error(error instanceof Error ? error.message : t('market.errors.operationFailed'))
    } finally { setPending(false) }
  }

  return <div className="h-full min-h-0 overflow-auto @container" data-testid="market-installation-detail">
    <article className="mx-auto flex max-w-6xl flex-col gap-6 p-4 @xl:p-6 @4xl:p-8">
      <Button variant="ghost" className="-ml-3 self-start" onClick={onBack}><ArrowLeft data-icon="inline-start" />{t('market.back')}</Button>
      <header className="flex min-w-0 flex-col gap-4">
        <div className="flex items-start gap-3">
          <div className="flex size-11 shrink-0 items-center justify-center rounded-lg border bg-muted/30"><Package className="size-5 text-muted-foreground" /></div>
          <div className="flex min-w-0 flex-1 flex-col gap-2">
            <h1 ref={heading} tabIndex={-1} className="text-2xl font-semibold tracking-tight outline-none [overflow-wrap:anywhere]">{name}</h1>
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-muted-foreground">
              <span className="[overflow-wrap:anywhere]">{t('market.acquired.installedIn')}: {targetName}</span>
              {item.package.version && <span className="[overflow-wrap:anywhere]">v{item.package.version}</span>}
              {item.package.author && <span className="[overflow-wrap:anywhere]">{item.package.author}</span>}
            </div>
          </div>
        </div>
        {description && <p className="max-w-3xl whitespace-pre-wrap leading-relaxed [overflow-wrap:anywhere]">{description}</p>}
        <div className="flex flex-wrap gap-2">
          <Badge variant="secondary">{t(`market.states.${item.tracking === 'detached' ? 'detached' : item.local_state || 'unchanged'}`)}</Badge>
          {item.tracking === 'tracked' && item.remote_state === 'update_available' && <Badge><RefreshCw data-icon="inline-start" />{t('market.updateAvailable')}</Badge>}
          {item.tracking === 'tracked' && (item.remote_state === 'check_failed' || item.remote_state === 'identity_changed') && <Badge variant="outline">{t(`market.states.${item.remote_state}`)}</Badge>}
        </div>
        <div className="flex flex-wrap gap-2">
          {canCheckUpdate && <Button disabled={pending || !!blockedReason} onClick={() => void run('check-update', () => onCheckUpdate(item))}>
            <RefreshCw data-icon="inline-start" />{t('market.checkUpdate')}
          </Button>}
          {item.tracking === 'tracked' && <Button variant={canCheckUpdate ? 'outline' : 'default'} disabled={pending || !!blockedReason} onClick={() => onImport({ installation: item })}>
            <Upload data-icon="inline-start" />{t('market.update.fromFile')}
          </Button>}
          <DropdownMenu>
            <DropdownMenuTrigger asChild><Button variant="outline" disabled={pending}><MoreHorizontal data-icon="inline-start" />{t('market.acquired.actions')}</Button></DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="w-auto max-w-[calc(100vw-2rem)]">
              <DropdownMenuGroup>
                <DropdownMenuItem disabled={!!blockedReason} onSelect={() => onExport({ installation: item })}><Upload />{t('market.export.title')}</DropdownMenuItem>
                <DropdownMenuItem disabled={!!blockedReason} onSelect={() => setDialog('backup')}><ArchiveRestore />{t('market.backups.title')}</DropdownMenuItem>
                {item.game_defaults && Object.keys(item.game_defaults).length > 0 && <DropdownMenuItem onSelect={() => setDialog('defaults')}><Settings2 />{t('gameDefaults.adopt')}</DropdownMenuItem>}
              </DropdownMenuGroup>
              {item.tracking === 'tracked' && <>
                <DropdownMenuSeparator />
                <DropdownMenuGroup><DropdownMenuItem onSelect={() => void run('detach-source', async () => {
                  await exchange(`/installations/${item.installation_id}/detach`, {})
                  await onChanged()
                  toast.success(t('market.detached'))
                })}><Unlink />{t('market.detach')}</DropdownMenuItem></DropdownMenuGroup>
              </>}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </header>
      <Separator />
      <div className="grid min-w-0 items-start gap-6 @4xl:grid-cols-[minmax(0,1fr)_minmax(0,18rem)] @4xl:gap-8">
        <div className="flex min-w-0 flex-col gap-6">
          <section className="flex min-w-0 flex-col gap-3" aria-label={t('market.contents.title')}>
            <div className="flex flex-wrap items-center gap-2"><h2 className="font-semibold">{t('market.contents.title')}</h2><Badge variant="secondary">{t('market.acquired.resourceCount', { count: item.bindings.length })}</Badge></div>
            <ul className="min-w-0 divide-y">
              {item.bindings.map(binding => {
                const extension = extensions.find(entry => !entry.removed && `extension.${entry.kind}` === binding.local.kind && entry.id === binding.local.id)
                const manifest = extension?.releases.find(release => release.ref.releaseId === extension.currentRelease)?.manifest
                return <li key={binding.resource_id} className="flex min-w-0 flex-wrap items-center gap-3 py-3">
                  <div className="flex min-w-0 flex-1 flex-col items-start gap-2">
                    <span className="text-sm [overflow-wrap:anywhere]">{localized(manifest?.name, i18n.language) || t(`market.kinds.${binding.local.kind}`)}</span>
                    <span className="text-xs font-mono text-muted-foreground [overflow-wrap:anywhere]">{binding.local.id}</span>
                    {(extension || binding.upstream_removed || binding.ownership === 'reference') && <div className="flex flex-wrap gap-1.5">
                      {extension && <Badge variant="secondary">{t(`market.kinds.${binding.local.kind}`)}</Badge>}
                      {binding.upstream_removed && <Badge variant="outline" className="h-auto whitespace-normal">{t('market.states.upstream_removed')}</Badge>}
                      {binding.ownership === 'reference' && <Badge variant="outline" className="h-auto whitespace-normal">{t('market.actions.reference')}</Badge>}
                    </div>}
                  </div>
                  {extension && <div className="flex items-center gap-2">
                    <InstalledExtensionToggle item={extension} onChanged={onChanged} disabled={pending} />
                    <Button size="sm" variant="outline" onClick={() => openInstalledExtension(extension.kind, extension.id)}>{t('market.manageExtension')}</Button>
                  </div>}
                </li>
              })}
            </ul>
          </section>
          {usage && <section className="flex flex-col gap-3"><Separator /><h2 className="font-semibold">{t('market.contents.usage')}</h2><p className="whitespace-pre-wrap text-sm leading-7 text-muted-foreground [overflow-wrap:anywhere]">{usage}</p></section>}
        </div>
        <aside className="flex min-w-0 flex-col gap-4 rounded-xl border bg-muted/20 p-4 @4xl:sticky @4xl:top-6">
          <h2 className="flex items-center gap-2 text-sm font-semibold">{item.source.kind === 'file' ? <FileArchive className="size-4" /> : <Link2 className="size-4" />}{t('market.acquired.source')}</h2>
          <p className="text-sm text-muted-foreground [overflow-wrap:anywhere]">{item.source.url || item.source.filename}</p>
          {(item.source.ref || item.source.path) && <p className="text-xs text-muted-foreground [overflow-wrap:anywhere]">{[item.source.ref, item.source.path].filter(Boolean).join(' · ')}</p>}
          {item.source.commit && <p className="text-xs text-muted-foreground [overflow-wrap:anywhere]">{t('market.import.commit', { commit: item.source.commit.slice(0, 12) })}</p>}
          {canCheckUpdate && <><Separator /><FieldGroup><Field>
            <FieldLabel htmlFor="market-installation-policy">{t('market.updatePolicy')}</FieldLabel>
            <Select value={item.update_mode} disabled={pending} onValueChange={mode => void run('change-update-policy', async () => {
              await exchange(`/installations/${item.installation_id}/policy`, { update_mode: mode })
              await onChanged()
            })}>
              <SelectTrigger id="market-installation-policy" className="w-full min-w-0 [&_[data-slot=select-value]]:truncate"><SelectValue /></SelectTrigger>
              <SelectContent><SelectGroup>
                <SelectItem value="manual">{t('market.policy.manual')}</SelectItem>
                <SelectItem value="notify">{t('market.policy.notify')}</SelectItem>
                {item.bindings.every(binding => binding.local.kind === 'skill' || binding.local.kind === 'style.reference' || binding.local.kind.startsWith('preset.')) && <SelectItem value="auto_apply">{t('market.policy.auto_apply')}</SelectItem>}
              </SelectGroup></SelectContent>
            </Select>
          </Field></FieldGroup></>}
          {compatibility && <><Separator /><h2 className="text-sm font-semibold">{t('market.contents.compatibility')}</h2><p className="whitespace-pre-wrap text-sm text-muted-foreground [overflow-wrap:anywhere]">{compatibility}</p></>}
        </aside>
      </div>
    </article>
    {dialog === 'backup' && <BackupDialog installation={item} onClose={() => setDialog(undefined)} onChanged={onChanged} />}
    {dialog === 'defaults' && item.game_defaults && <GameDefaultsDialog defaults={item.game_defaults} projectID={item.project_id} onClose={() => setDialog(undefined)} />}
  </div>
}
