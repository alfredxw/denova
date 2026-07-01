import type { VersionItem } from './version-timeline'
import type { VersionRestorePlan } from '@/lib/api'
import { useTranslation } from 'react-i18next'
import { AlertTriangle, FileText, ShieldCheck } from 'lucide-react'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'

interface RollbackDialogProps {
  open: boolean
  version?: VersionItem | null
  plan?: VersionRestorePlan | null
  loading?: boolean
  planLoading?: boolean
  onOpenChange: (open: boolean) => void
  onRollback: (version: VersionItem) => void | Promise<void>
}

/** 回滚确认弹窗，通过二次确认避免误重置当前作品工作区。 */
export function RollbackDialog({
  open,
  version,
  plan,
  loading = false,
  planLoading = false,
  onOpenChange,
  onRollback,
}: RollbackDialogProps) {
  const { t } = useTranslation()
  const scope = plan?.scope ?? 'workspace'
  const changes = plan?.changes ?? []
  const versionLabel = version?.title || version?.description || ''
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent className="max-h-[calc(100dvh-2rem)] border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)]">
        <AlertDialogHeader>
          <AlertDialogTitle>{scope === 'paths' ? t('versions.restoreFileTitle') : t('versions.rollbackTitle')}</AlertDialogTitle>
          <AlertDialogDescription className="text-[var(--nova-text-muted)]">
            {planLoading
              ? t('versions.restorePlanLoading')
              : version
                ? (scope === 'paths'
                    ? t('versions.restoreFileDescription', { version: versionLabel })
                    : t('versions.rollbackDescription', { version: versionLabel }))
              : t('versions.rollbackPickVersion')}
          </AlertDialogDescription>
        </AlertDialogHeader>
        {plan && (
          <div className="min-h-0 space-y-2 overflow-hidden text-xs">
            {plan.will_create_backup && (
              <div className="flex min-w-0 items-start gap-2 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-2 text-[var(--nova-text-muted)]">
                <ShieldCheck className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[var(--nova-accent-green)]" />
                <span className="min-w-0">{t('versions.restoreBackupNotice', { message: plan.backup_message || t('versions.source.rollbackBackup') })}</span>
              </div>
            )}
            {scope === 'paths' && (
              <div className="flex min-w-0 items-start gap-2 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-2 text-[var(--nova-text-muted)]">
                <FileText className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                <span className="min-w-0">{t('versions.restorePathNotice')}</span>
              </div>
            )}
            {changes.length === 0 && (
              <div className="flex min-w-0 items-start gap-2 rounded border border-[var(--nova-warning-bg)] bg-[var(--nova-warning-bg)] px-2 py-2 text-[var(--nova-warning)]">
                <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                <span className="min-w-0">{t('versions.restoreNoopWarning')}</span>
              </div>
            )}
            <div className="rounded border border-[var(--nova-border)] bg-[var(--nova-bg)]">
              <div className="flex items-center justify-between gap-2 border-b border-[var(--nova-border)] px-2 py-1.5 text-[11px] font-medium text-[var(--nova-text-muted)]">
                <span>{t('versions.restoreAffectedFiles')}</span>
                <span>{changes.length}</span>
              </div>
              <div className="max-h-48 min-h-0 overflow-y-auto p-1">
                {changes.length === 0 ? (
                  <div className="px-1.5 py-1 text-[var(--nova-text-faint)]">{t('versions.noChanges')}</div>
                ) : changes.map(change => (
                  <div key={`${change.status}:${change.path}`} className="flex min-w-0 items-center gap-2 rounded px-1.5 py-1 text-[11px] text-[var(--nova-text-muted)]">
                    <FileText className="h-3 w-3 shrink-0 text-[var(--nova-text-faint)]" />
                    <span className="min-w-0 flex-1 truncate" title={change.path}>{change.path}</span>
                    <span className="shrink-0 text-[var(--nova-text-faint)]">{t(`versions.change.${change.status}`)}</span>
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={loading}>{t('common.cancel')}</AlertDialogCancel>
          <AlertDialogAction
            className="bg-[var(--nova-warning-bg)] text-[var(--nova-warning)] hover:bg-[var(--nova-warning-bg)]"
            disabled={loading || planLoading || !version}
            onClick={(event) => {
              event.preventDefault()
              if (version) void onRollback(version)
            }}
          >
            {scope === 'paths' ? t('versions.restoreFileConfirm') : t('versions.rollbackConfirm')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
