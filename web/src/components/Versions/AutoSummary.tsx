import { Clock3 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { VersionStatus } from '@/lib/api'

export function AutoSummary({ status }: { status: VersionStatus | null }) {
  const { t } = useTranslation()
  const auto = status?.auto
  return (
    <div className="mt-2 grid min-w-0 grid-cols-1 gap-2">
      <div className="min-w-0 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1.5">
        <div className="flex min-w-0 items-center gap-1 text-[var(--nova-text)]"><Clock3 className="h-3 w-3 shrink-0" /><span className="min-w-0 truncate">{t('versions.auto.timed')}</span></div>
        <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">{auto?.timed_enabled ? t('versions.auto.timedHint', { count: auto.timed_interval_minutes }) : t('versions.auto.closed')}</div>
      </div>
    </div>
  )
}
