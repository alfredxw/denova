import { useEffect, useState } from 'react'
import { ChevronDown, Download, Package } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { useWorkspaceStore } from '@/stores/workspace-store'
import { exchange, type Installation, type Preview } from './api'
import { BackupDialog } from './BackupDialog'
import type { ImportDialogProps } from './ImportDialog'

export function AcquiredResources({
  installations,
  onChanged,
  onImport,
  onExport,
  onManage,
  onDiscover,
}: {
  installations: Installation[]
  onChanged: () => Promise<void>
  onImport: (props: Omit<ImportDialogProps, 'onClose' | 'onInstalled'>) => void
  onExport: (props: { installation?: Installation }) => void
  onManage: (item: Installation) => Promise<void>
  onDiscover: () => void
}) {
  const { t } = useTranslation()
  const requested = useWorkspaceStore((state) => state.marketInstallationID)
  const [expanded, setExpanded] = useState<string>()
  const [backup, setBackup] = useState<Installation>()
  const [pending, setPending] = useState('')
  useEffect(() => {
    if (requested) setExpanded(requested)
  }, [requested])
  const run = async (id: string, action: () => Promise<void>) => {
    setPending(id)
    try {
      await action()
    } catch (error) {
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
        <div className="space-y-3">
          {installations.map((item) => (
            <Card key={item.installation_id} className="gap-0 py-0">
              <button
                className="flex w-full items-center gap-3 p-4 text-left"
                aria-expanded={expanded === item.installation_id}
                onClick={() =>
                  setExpanded(
                    expanded === item.installation_id
                      ? undefined
                      : item.installation_id,
                  )
                }
              >
                <Package className="size-5 shrink-0 text-muted-foreground" />
                <span className="min-w-0 flex-1">
                  <span className="block font-medium break-words">
                    {item.package.name}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {item.bindings.length} ·{' '}
                    {t(
                      `market.states.${item.tracking === 'detached' ? 'detached' : item.local_state || 'unchanged'}`,
                    )}
                  </span>
                </span>
                {item.remote_state === 'update_available' && (
                  <Badge>{t('market.updateAvailable')}</Badge>
                )}
                {item.remote_state &&
                  ['check_failed', 'identity_changed'].includes(
                    item.remote_state,
                  ) && (
                    <Badge variant="outline">
                      {t(`market.states.${item.remote_state}`)}
                    </Badge>
                  )}
                <ChevronDown
                  className={cn(
                    'size-4 transition-transform',
                    expanded === item.installation_id && 'rotate-180',
                  )}
                />
              </button>
              {expanded === item.installation_id && (
                <CardContent className="space-y-4 border-t pt-4">
                  <p className="break-all text-xs text-muted-foreground">
                    {item.source.url || item.source.filename}
                  </p>
                  <ul className="space-y-1 text-sm">
                    {item.bindings.map((binding) => (
                      <li
                        key={binding.resource_id}
                        className="flex flex-wrap gap-2"
                      >
                        <span>{t(`market.kinds.${binding.local.kind}`)}</span>
                        <span className="break-all text-muted-foreground">
                          {binding.local.id}
                        </span>
                        {binding.upstream_removed && (
                          <Badge variant="outline">
                            {t('market.states.upstream_removed')}
                          </Badge>
                        )}
                        {binding.ownership === 'reference' && (
                          <Badge variant="outline">
                            {t('market.actions.reference')}
                          </Badge>
                        )}
                      </li>
                    ))}
                  </ul>
                  {item.tracking === 'tracked' &&
                    item.source.kind !== 'file' && (
                      <Select
                        value={item.update_mode}
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
                        <SelectTrigger aria-label={t('market.updatePolicy')}>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="manual">
                            {t('market.policy.manual')}
                          </SelectItem>
                          <SelectItem value="notify">
                            {t('market.policy.notify')}
                          </SelectItem>
                          {item.bindings.every(
                            (binding) =>
                              binding.local.kind === 'skill' ||
                              binding.local.kind === 'style.reference' ||
                              binding.local.kind.startsWith('preset.'),
                          ) && (
                            <SelectItem value="auto_apply">
                              {t('market.policy.auto_apply')}
                            </SelectItem>
                          )}
                        </SelectContent>
                      </Select>
                    )}
                  <div className="flex flex-wrap gap-2">
                    <Button
                      variant="outline"
                      onClick={() =>
                        void run(item.installation_id, () => onManage(item))
                      }
                    >
                      {t('market.manage')}
                    </Button>
                    <Button
                      variant="outline"
                      onClick={() => onExport({ installation: item })}
                    >
                      {t('market.export.title')}
                    </Button>
                    <Button variant="outline" onClick={() => setBackup(item)}>
                      {t('market.backups.title')}
                    </Button>
                    {item.tracking === 'tracked' &&
                      item.source.kind !== 'file' && (
                        <Button
                          variant="outline"
                          disabled={!!pending}
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
                          {t('market.checkUpdate')}
                        </Button>
                      )}
                    {item.tracking === 'tracked' && (
                      <Button
                        variant="ghost"
                        disabled={!!pending}
                        onClick={() =>
                          void run(item.installation_id, async () => {
                            await exchange(
                              `/installations/${item.installation_id}/detach`,
                              {},
                            )
                            await onChanged()
                            toast.success(t('market.detached'))
                          })
                        }
                      >
                        {t('market.detach')}
                      </Button>
                    )}
                  </div>
                </CardContent>
              )}
            </Card>
          ))}
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
    </>
  )
}
