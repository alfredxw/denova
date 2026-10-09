import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  ArchiveRestore,
  ChevronDown,
  Download,
  FileArchive,
  Link2,
  Package,
  RefreshCw,
  Settings2,
  Unlink,
  Upload,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { cn } from '@/lib/utils'
import { getBooks } from '@/lib/api'
import { localized, type CatalogEntry } from '@/features/platform/api'
import { openInstalledExtension } from '@/features/platform/extension-navigation'
import { exchange, type Installation, type Preview } from './api'
import { BackupDialog } from './BackupDialog'
import type { ImportDialogProps } from './ImportDialog'
import { GameDefaultsDialog } from '@/features/interactive/components/GameDefaultsDialog'

export function AcquiredResources({
  installations,
  onChanged,
  onImport,
  onExport,
  onDiscover,
  requested,
  extensions,
  blockedReason,
}: {
  installations: Installation[]
  onChanged: () => Promise<void>
  onImport: (props: Omit<ImportDialogProps, 'onClose' | 'onInstalled'>) => void
  onExport: (props: { installation?: Installation }) => void
  onDiscover: () => void
  requested?: string
  extensions: CatalogEntry[]
  blockedReason?: string
}) {
  const { t, i18n } = useTranslation()
  const books = useQuery({ queryKey: ['resources', 'books'], queryFn: getBooks, enabled: installations.some(item => !!item.project_id) })
  const [expanded, setExpanded] = useState<string>()
  const [backup, setBackup] = useState<Installation>()
  const [defaults, setDefaults] = useState<Installation>()
  const [pending, setPending] = useState('')
  useEffect(() => {
    if (requested) setExpanded(requested)
  }, [requested])
  const run = async (id: string, action: () => Promise<void>) => {
    setPending(id)
    try {
      await action()
    } catch (error) {
      console.error('[market] installed package action failed', {
        installationID: id,
        error,
      })
      toast.error(
        error instanceof Error
          ? error.message
          : t('market.errors.operationFailed'),
      )
    } finally {
      setPending('')
    }
  }
  return (
    <>
      {installations.length ? (
        <div className="flex min-w-0 flex-col gap-3">
          {installations.map((item) => {
            const canCheckUpdate =
              item.tracking === 'tracked' && item.source.kind !== 'file'
            return (
              <Collapsible
                key={item.installation_id}
                open={expanded === item.installation_id}
                onOpenChange={(open) =>
                  setExpanded(open ? item.installation_id : undefined)
                }
                asChild
              >
                <Card
                  size="sm"
                  data-testid="market-installation-card"
                  className="@container/installation min-w-0 gap-0 py-0"
                >
                  <CardHeader className="relative grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3 p-3 transition-colors hover:bg-muted/40 has-focus-visible:ring-2 has-focus-visible:ring-inset has-focus-visible:ring-ring has-data-[slot=card-description]:grid-rows-1 sm:p-4">
                    <div className="flex size-8 items-center justify-center rounded-lg bg-muted text-muted-foreground">
                      <Package className="size-4" />
                    </div>
                    <div className="flex min-w-0 flex-col gap-1.5">
                      <CardTitle>
                        <CollapsibleTrigger className="text-left [overflow-wrap:anywhere] after:absolute after:inset-0 focus:outline-none">
                          {item.package.name}
                        </CollapsibleTrigger>
                      </CardTitle>
                      <CardDescription className="flex flex-wrap items-center gap-x-3 gap-y-2">
                        <span>{item.project_id ? books.data?.find(book => book.project_id === item.project_id)?.name || item.project_id : t('market.import.global')}</span>
                        <span>
                          {t('market.acquired.resourceCount', {
                            count: item.bindings.length,
                          })}
                        </span>
                        {item.package.version && (
                          <span className="[overflow-wrap:anywhere]">
                            v{item.package.version}
                          </span>
                        )}
                        <Badge variant="secondary">
                          {t(
                            `market.states.${item.tracking === 'detached' ? 'detached' : item.local_state || 'unchanged'}`,
                          )}
                        </Badge>
                        {item.remote_state === 'update_available' &&
                          item.tracking === 'tracked' && (
                            <Badge>
                              <RefreshCw data-icon="inline-start" />
                              {t('market.updateAvailable')}
                            </Badge>
                          )}
                        {item.remote_state &&
                          ['check_failed', 'identity_changed'].includes(item.remote_state) && (
                            <Badge variant="outline">
                              {t(`market.states.${item.remote_state}`)}
                            </Badge>
                          )}
                      </CardDescription>
                    </div>
                    <ChevronDown
                      aria-hidden
                      className={cn(
                        'size-4 shrink-0 text-muted-foreground transition-transform',
                        expanded === item.installation_id && 'rotate-180',
                      )}
                    />
                  </CardHeader>
                  <CollapsibleContent>
                    <Separator />
                    <CardContent className="grid min-w-0 gap-4 p-3 sm:p-4 @2xl/installation:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
                      <div className="flex min-w-0 flex-col gap-3">
                        <div className="flex min-w-0 flex-col gap-2">
                          <h3 className="flex items-center gap-2 text-sm font-medium">
                            {item.source.kind === 'file' ? (
                              <FileArchive className="size-4 text-muted-foreground" />
                            ) : (
                              <Link2 className="size-4 text-muted-foreground" />
                            )}
                            {t('market.acquired.source')}
                          </h3>
                          <p className="text-sm text-muted-foreground [overflow-wrap:anywhere]">
                            {item.source.url || item.source.filename}
                          </p>
                        </div>
                        {canCheckUpdate && (
                          <FieldGroup>
                            <Field>
                              <FieldLabel htmlFor={`market-policy-${item.installation_id}`}>
                                {t('market.updatePolicy')}
                              </FieldLabel>
                              <Select
                                value={item.update_mode}
                                disabled={!!pending}
                                onValueChange={(mode) =>
                                  void run(item.installation_id, async () => {
                                    await exchange(
                                      `/installations/${item.installation_id}/policy`,
                                      { update_mode: mode },
                                    )
                                    await onChanged()
                                  })
                                }
                              >
                                <SelectTrigger
                                  id={`market-policy-${item.installation_id}`}
                                  className="w-full min-w-0 [&_[data-slot=select-value]]:truncate"
                                >
                                  <SelectValue />
                                </SelectTrigger>
                                <SelectContent>
                                  <SelectGroup>
                                    <SelectItem value="manual">
                                      {t('market.policy.manual')}
                                    </SelectItem>
                                    <SelectItem value="notify">
                                      {t('market.policy.notify')}
                                    </SelectItem>
                                    {item.bindings.every((binding) =>
                                      binding.local.kind === 'skill' ||
                                      binding.local.kind === 'style.reference' ||
                                      binding.local.kind.startsWith('preset.'),
                                    ) && (
                                      <SelectItem value="auto_apply">
                                        {t('market.policy.auto_apply')}
                                      </SelectItem>
                                    )}
                                  </SelectGroup>
                                </SelectContent>
                              </Select>
                            </Field>
                          </FieldGroup>
                        )}
                      </div>
                      <div className="flex min-w-0 flex-col gap-2">
                        <h3 className="text-sm font-medium">{t('market.contents.title')}</h3>
                        <ul className="grid min-w-0 gap-x-4 gap-y-2 @xl/installation:grid-cols-2">
                          {item.bindings.map((binding) => {
                            const extension = extensions.find(entry => !entry.removed && `extension.${entry.kind}` === binding.local.kind && entry.id === binding.local.id)
                            const manifest = extension?.releases.find(release => release.ref.releaseId === extension.currentRelease)?.manifest
                            return (
                            <li
                              key={binding.resource_id}
                              className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1"
                            >
                              <span className="max-w-full text-sm">
                                {t(`market.kinds.${binding.local.kind}`)}
                              </span>
                              <span
                                title={binding.local.id}
                                className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground"
                              >
                                {localized(manifest?.name, i18n.language) || binding.local.id}
                              </span>
                              {extension && <Button size="sm" variant="link" className="h-auto p-0" onClick={() => openInstalledExtension(extension.kind, extension.id)}>{t('market.manageExtension')}</Button>}
                              {(binding.upstream_removed || binding.ownership === 'reference') && (
                                <div className="flex basis-full flex-wrap gap-1.5">
                                  {binding.upstream_removed && (
                                    <Badge variant="outline" className="h-auto whitespace-normal">
                                      {t('market.states.upstream_removed')}
                                    </Badge>
                                  )}
                                  {binding.ownership === 'reference' && (
                                    <Badge variant="outline" className="h-auto whitespace-normal">
                                      {t('market.actions.reference')}
                                    </Badge>
                                  )}
                                </div>
                              )}
                            </li>
                          )})}
                        </ul>
                      </div>
                    </CardContent>
                    <CardFooter className="flex-wrap gap-2 p-3 sm:px-4">
                      {canCheckUpdate && (
                        <Button
                          size="sm"
                          disabled={!!pending || !!blockedReason}
                          onClick={() =>
                            void run(item.installation_id, async () => {
                              const preview = await exchange<Preview>(
                                `/installations/${item.installation_id}/check`,
                                {},
                              )
                              await onChanged()
                              onImport({
                                preview,
                                installation: item,
                              })
                            })
                          }
                        >
                          <RefreshCw data-icon="inline-start" />
                          {t('market.checkUpdate')}
                        </Button>
                      )}
                      {item.tracking === 'tracked' && item.source.kind === 'file' && <Button size="sm" disabled={!!pending || !!blockedReason} onClick={() => onImport({ installation: item })}>
                        <Upload data-icon="inline-start" />{t('market.update.fromFile')}
                      </Button>}
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={!!pending}
                        onClick={() => onExport({ installation: item })}
                      >
                        <Upload data-icon="inline-start" />
                        {t('market.export.title')}
                      </Button>
                      <Button size="sm" variant="outline" disabled={!!pending || !!blockedReason} onClick={() => setBackup(item)}>
                        <ArchiveRestore data-icon="inline-start" />
                        {t('market.backups.title')}
                      </Button>
                      {item.game_defaults && Object.keys(item.game_defaults).length > 0 && (
                        <Button size="sm" variant="outline" disabled={!!pending} onClick={() => setDefaults(item)}>
                          <Settings2 data-icon="inline-start" />
                          {t('gameDefaults.adopt')}
                        </Button>
                      )}
                      {item.tracking === 'tracked' && (
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={!!pending}
                          onClick={() => void run(item.installation_id, async () => {
                            await exchange(`/installations/${item.installation_id}/detach`, {})
                            await onChanged()
                            toast.success(t('market.detached'))
                          })}
                        >
                          <Unlink data-icon="inline-start" />
                          {t('market.detach')}
                        </Button>
                      )}
                    </CardFooter>
                  </CollapsibleContent>
                </Card>
              </Collapsible>
            )
          })}
        </div>
      ) : (
        <Empty>
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <Download />
            </EmptyMedia>
            <EmptyTitle>{t('market.acquiredEmpty')}</EmptyTitle>
            <EmptyDescription>{t('market.acquiredHelp')}</EmptyDescription>
          </EmptyHeader>
          <Button onClick={() => onDiscover()}>{t('market.discover')}</Button>
        </Empty>
      )}
      {backup && (
        <BackupDialog
          installation={backup}
          onClose={() => setBackup(undefined)}
          onChanged={onChanged}
        />
      )}
      {defaults?.game_defaults && <GameDefaultsDialog defaults={defaults.game_defaults} projectID={defaults.project_id} onClose={() => setDefaults(undefined)} />}
    </>
  )
}
