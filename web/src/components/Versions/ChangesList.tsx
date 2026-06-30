import type { VersionChange } from '@/lib/api'
import { useTranslation } from 'react-i18next'
import { dirName, fileName, statusColor, statusLabel, statusText } from './version-panel-utils'

interface ChangesListProps {
  changes: VersionChange[]
  onOpenDiff: (path: string) => void
}

export function ChangesList({ changes, onOpenDiff }: ChangesListProps) {
  const { t } = useTranslation()
  if (changes.length === 0) {
    return <div className="rounded bg-[var(--nova-surface)] px-2 py-2 text-[var(--nova-text-faint)]">{t('versions.noChanges')}</div>
  }
  return (
    <div className="min-w-0 space-y-0.5 overflow-hidden">
      {changes.map(change => (
        <button key={`${change.status}:${change.path}`} type="button" className="group flex w-full min-w-0 items-center gap-2 overflow-hidden rounded px-1.5 py-1 text-left hover:bg-[var(--nova-hover)]" title={change.path} onClick={() => onOpenDiff(change.path)}>
          <span className="flex h-4 w-4 shrink-0 items-center justify-center rounded-sm border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[9px] text-[var(--nova-text-muted)]">{statusLabel(change.status)}</span>
          <span className="min-w-0 flex-1 truncate text-[var(--nova-text-muted)]">{fileName(change.path)}</span>
          <span className="hidden min-w-0 max-w-[45%] truncate text-[10px] text-[var(--nova-text-faint)] sm:inline">{dirName(change.path)}</span>
          <span className={`shrink-0 text-[11px] ${statusColor(change.status)}`}>{statusText(change.status, t)}</span>
        </button>
      ))}
    </div>
  )
}
