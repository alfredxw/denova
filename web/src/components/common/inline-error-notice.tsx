import { AlertTriangle } from 'lucide-react'

interface InlineErrorNoticeProps {
  message: string
  title?: string
  className?: string
}

/** InlineErrorNotice 用于 IDE 面板内的紧凑错误提示。 */
export function InlineErrorNotice({ message, title = '操作失败', className = '' }: InlineErrorNoticeProps) {
  return (
    <div className={`flex items-start gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-2 text-xs leading-5 text-[var(--nova-text-muted)] shadow-[var(--nova-shadow)] ${className}`}>
      <span className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded border border-red-500/30 bg-red-500/10 text-red-300">
        <AlertTriangle className="h-3.5 w-3.5" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="mr-1 font-medium text-[var(--nova-text)]">{title}</span>
        <span className="break-words text-[var(--nova-text-muted)]">{message}</span>
      </span>
    </div>
  )
}
