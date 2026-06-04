import { ShieldAlert, ShieldCheck } from 'lucide-react'
import type { VersionStatus } from '@/lib/api'
import { formatTime, workspaceName } from './version-panel-utils'

interface VersionHeaderProps {
  workspace: string
  status: VersionStatus | null
  changesCount: number
}

export function VersionHeader({ workspace, status, changesCount }: VersionHeaderProps) {
  const hasVersions = status?.has_versions ?? false
  const clean = status?.clean ?? true
  const Icon = !hasVersions || !clean ? ShieldAlert : ShieldCheck
  const label = !hasVersions ? '尚无版本' : clean ? '已保护' : `${changesCount} 个未保存版本变更`

  return (
    <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-2">
      <div className="flex items-center gap-2">
        <Icon className={`h-3.5 w-3.5 ${!hasVersions || !clean ? 'text-[var(--nova-accent)]' : 'text-[var(--nova-accent-green)]'}`} />
        <span className="min-w-0 flex-1 truncate font-medium text-[var(--nova-text)]">{workspaceName(workspace) || '未选择书籍'}</span>
        <span className="rounded-full bg-[var(--nova-active)] px-2 py-0.5 text-[11px] text-[var(--nova-text)]">{label}</span>
      </div>
      <div className="mt-2 flex items-center gap-2 text-[11px] text-[var(--nova-text-faint)]">
        {status?.latest ? <span>当前版本：{formatTime(status.latest.created_at)}</span> : <span>保存第一个版本后即可查看历史和恢复。</span>}
      </div>
    </div>
  )
}
