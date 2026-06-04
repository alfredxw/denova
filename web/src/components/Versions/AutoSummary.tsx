import { Clock3, FileText } from 'lucide-react'
import type { VersionStatus } from '@/lib/api'

export function AutoSummary({ status }: { status: VersionStatus | null }) {
  const auto = status?.auto
  return (
    <div className="mt-2 grid grid-cols-2 gap-2">
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1.5">
        <div className="flex items-center gap-1 text-[var(--nova-text)]"><Clock3 className="h-3 w-3" />定时保存</div>
        <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">{auto?.timed_enabled ? `有变更每 ${auto.timed_interval_minutes} 分钟` : '已关闭'}</div>
      </div>
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1.5">
        <div className="flex items-center gap-1 text-[var(--nova-text)]"><FileText className="h-3 w-3" />Agent 保存</div>
        <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">{auto?.agent_enabled ? `约 ${auto.agent_char_threshold} 字触发` : '已关闭'}</div>
      </div>
    </div>
  )
}
