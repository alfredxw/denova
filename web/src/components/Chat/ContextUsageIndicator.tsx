import { Gauge } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { formatLocaleNumber } from '@/i18n'
import { cn } from '@/lib/utils'
import type { TokenUsageRecord } from './TokenUsagePanel'

const CONTEXT_USAGE_WARNING_RATIO = 0.9
const CONTEXT_USAGE_CRITICAL_RATIO = 1

export interface ContextUsageSnapshot {
  promptTokens: number
  contextWindowTokens: number
  ratio: number
}

export function ContextUsageIndicator({ messages, agentKind, onOpenDetails, disabled = false }: {
  messages: TokenUsageRecord[]
  agentKind?: string
  onOpenDetails?: () => void
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const usage = useMemo(() => latestContextUsage(messages, agentKind), [agentKind, messages])
  const percent = usage ? Math.round(usage.ratio * 100) : null
  const level = usage ? contextUsageLevel((percent || 0) / 100) : null
  const detail = usage
    ? t(onOpenDetails ? 'chat.contextUsage.detailAction' : 'chat.contextUsage.detail', {
      level: t(`chat.contextUsage.level.${level}`),
      used: formatLocaleNumber(usage.promptTokens),
      limit: formatLocaleNumber(usage.contextWindowTokens),
      percent,
    })
    : t(onOpenDetails ? 'chat.contextUsage.unavailableAction' : 'chat.contextUsage.unavailable')
  const className = cn(
    'inline-flex h-8 shrink-0 items-center gap-1 rounded-md px-1.5 text-[11px] tabular-nums transition-colors',
    (!level || level === 'normal') && 'text-[var(--nova-text-faint)]',
    level === 'warning' && 'bg-[var(--nova-warning-bg)] text-[var(--nova-warning)]',
    level === 'critical' && 'bg-[var(--nova-danger-bg)] text-[var(--nova-danger)]',
  )
  const content = (
    <>
      <Gauge aria-hidden="true" className="h-3.5 w-3.5" />
      <span>{percent === null ? '—%' : `${percent}%`}</span>
    </>
  )

  if (!onOpenDetails) {
    return <span role="status" className={className} aria-label={detail} title={detail} data-context-usage-indicator="true">{content}</span>
  }
  return (
    <button
      type="button"
      className={cn(className, 'hover:bg-[var(--nova-hover)] disabled:pointer-events-none disabled:opacity-45')}
      disabled={disabled}
      onClick={onOpenDetails}
      aria-label={detail}
      title={detail}
      data-context-usage-indicator="true"
    >
      {content}
    </button>
  )
}

/** Uses one model call, never the per-run aggregate, as context occupancy. */
export function latestContextUsage(messages: TokenUsageRecord[], agentKind?: string): ContextUsageSnapshot | null {
  for (let messageIndex = messages.length - 1; messageIndex >= 0; messageIndex -= 1) {
    const message = messages[messageIndex]
    if (agentKind && message.agent_kind !== agentKind) continue
    const contextWindowTokens = positiveNumber(message.context_window_tokens)
    const promptTokens = positiveNumber(message.context_prompt_tokens)
    return contextWindowTokens && promptTokens
      ? contextUsageSnapshot(promptTokens, contextWindowTokens)
      : null
  }
  return null
}

function contextUsageSnapshot(promptTokens: number, contextWindowTokens: number): ContextUsageSnapshot {
  return {
    promptTokens,
    contextWindowTokens,
    ratio: promptTokens / contextWindowTokens,
  }
}

function positiveNumber(value: unknown) {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : 0
}

function contextUsageLevel(ratio: number) {
  if (ratio >= CONTEXT_USAGE_CRITICAL_RATIO) return 'critical'
  if (ratio >= CONTEXT_USAGE_WARNING_RATIO) return 'warning'
  return 'normal'
}
