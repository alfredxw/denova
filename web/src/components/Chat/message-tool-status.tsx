import { AlertTriangle, CheckCircle2, Loader2, Square } from 'lucide-react'
import type { ChatMessageStatus } from '@/lib/api'

export function ToolStatusIcon({ status, warning = false }: { status?: ChatMessageStatus; warning?: boolean }) {
  if (status === 'error') {
    return <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] text-[10px] text-[var(--nova-danger)]">!</span>
  }
  if (warning) {
    return (
      <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full border border-[var(--nova-warning)]/40 bg-[var(--nova-warning-bg)] text-[var(--nova-warning)]">
        <AlertTriangle className="h-3.5 w-3.5" />
      </span>
    )
  }
  if (status === 'success') {
    return (
      <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full border border-[var(--nova-accent-green)]/45 bg-[var(--nova-accent-green)]/10 text-[var(--nova-accent-green)]">
        <CheckCircle2 className="h-3.5 w-3.5" />
      </span>
    )
  }
  if (status === 'cancelled') {
    return (
      <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-faint)]">
        <Square className="h-2.5 w-2.5 fill-current" />
      </span>
    )
  }
  return (
    <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-faint)]">
      <Loader2 className="h-3.5 w-3.5 animate-spin will-change-transform" />
    </span>
  )
}
