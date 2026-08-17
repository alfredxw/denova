import { useTranslation } from 'react-i18next'
import type { ChatMessage } from '@/lib/api'

export function AgentSourceBadge({ message, compact = false }: { message: ChatMessage; compact?: boolean }) {
  const { t } = useTranslation()
  const name = message.agent_name || message.subagent_type || t('chat.subagent.label')
  const label = compact ? name : t('chat.subagent.outputFrom', { name })
  return (
    <span className={`mb-1 inline-flex max-w-full items-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)] ${compact ? 'mb-0 min-w-0' : ''}`}>
      <span className="truncate">{label}</span>
    </span>
  )
}
