import { Clock3, FileText } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { VersionStatus } from '@/lib/api'

export function AutoSummary({ status }: { status: VersionStatus | null }) {
  const { t } = useTranslation()
  const auto = status?.auto
  return (
    <div className="mt-2 grid grid-cols-2 gap-2">
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1.5">
        <div className="flex items-center gap-1 text-[var(--nova-text)]"><Clock3 className="h-3 w-3" />{t('versions.auto.timed')}</div>
        <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">{auto?.timed_enabled ? t('versions.auto.timedHint', { count: auto.timed_interval_minutes }) : t('versions.auto.closed')}</div>
      </div>
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1.5">
        <div className="flex items-center gap-1 text-[var(--nova-text)]"><FileText className="h-3 w-3" />{t('versions.auto.agent')}</div>
        <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">{auto?.agent_enabled ? t('versions.auto.agentHint', { count: auto.agent_char_threshold }) : t('versions.auto.closed')}</div>
      </div>
    </div>
  )
}
