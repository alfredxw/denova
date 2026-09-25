import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { fetchAPI } from '@/lib/api-client/client'
import { exchange, type Installation, type Plan } from './api'

type Backup = {
  backup_id: string
  state: string
  created_at: string
  files: number
}
export function BackupDialog({
  installation,
  onClose,
  onChanged,
}: {
  installation: Installation
  onClose: () => void
  onChanged: () => Promise<void>
}) {
  const { t, i18n } = useTranslation()
  const [backups, setBackups] = useState<Backup[]>([])
  const [plan, setPlan] = useState<Plan>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => {
    let alive = true
    void exchange<Backup[]>(
      `/installations/${installation.installation_id}/backups`,
    )
      .then((items) => {
        if (alive) setBackups(items)
      })
      .catch(() => {
        if (alive) setError(t('market.errors.operationFailed'))
      })
    return () => {
      alive = false
    }
  }, [installation.installation_id, t])
  const run = async (action: () => Promise<void>) => {
    setBusy(true)
    setError('')
    try {
      await action()
    } catch (error) {
      setError(
        error instanceof Error
          ? error.message
          : t('market.errors.operationFailed'),
      )
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose()
      }}
    >
      <DialogContent className="max-h-[90dvh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{t('market.backups.title')}</DialogTitle>
          <DialogDescription>
            {t(plan ? 'market.backups.confirm' : 'market.backups.help')}
          </DialogDescription>
        </DialogHeader>
        {plan ? (
          <ul className="space-y-2">
            {plan.items.map((item) => (
              <li key={item.resource_id} className="break-words text-sm">
                {item.name} · {t(`market.kinds.${item.local.kind}`)}
              </li>
            ))}
          </ul>
        ) : (
          <div className="divide-y rounded-lg border">
            {backups.map((backup) => (
              <div
                key={backup.backup_id}
                className="flex flex-wrap items-center justify-between gap-3 p-3"
              >
                <span className="text-sm">
                  {new Date(backup.created_at).toLocaleString(i18n.language)}
                  <span className="block text-xs text-muted-foreground">
                    {t('market.backups.files', { count: backup.files })}
                  </span>
                </span>
                <div className="flex flex-wrap gap-2">
                  <Button
                    variant="outline"
                    disabled={busy}
                    onClick={() =>
                      void run(async () => {
                        const response = await fetchAPI(
                          `/api/resource-exchange/backups/${backup.backup_id}/download`,
                        )
                        const url = URL.createObjectURL(await response.blob())
                        const link = document.createElement('a')
                        link.href = url
                        link.download = `denova-backup-${backup.backup_id}.zip`
                        link.click()
                        setTimeout(() => URL.revokeObjectURL(url), 1000)
                      })
                    }
                  >
                    {t('market.backups.download')}
                  </Button>
                  {backup.state === 'committed' && (
                    <Button
                      variant="outline"
                      disabled={busy}
                      onClick={() =>
                        void run(async () =>
                          setPlan(
                            await exchange<Plan>(
                              `/backups/${backup.backup_id}/restore-plan`,
                              {},
                            ),
                          ),
                        )
                      }
                    >
                      {t('market.backups.review')}
                    </Button>
                  )}
                </div>
              </div>
            ))}
            {!backups.length && (
              <p className="p-3 text-sm text-muted-foreground">
                {t('market.backups.empty')}
              </p>
            )}
          </div>
        )}
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        <DialogFooter>
          <Button
            variant="outline"
            disabled={busy}
            onClick={plan ? () => setPlan(undefined) : onClose}
          >
            {t(plan ? 'market.back' : 'common.cancel')}
          </Button>
          {plan && (
            <Button
              disabled={busy}
              onClick={() =>
                void run(async () => {
                  await exchange(`/plans/${plan.plan_id}/apply`, {})
                  await onChanged()
                  toast.success(t('market.backups.done'))
                  onClose()
                })
              }
            >
              {t(busy ? 'market.working' : 'market.backups.restore')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
