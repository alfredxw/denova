import { useMemo } from 'react'
import { Activity } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { MessageList } from '@/components/Chat/MessageList'
import type { ChatMessage } from '@/lib/api'
import { chatMessagesToAgentUIMessages } from '@/lib/agent-legacy-message'
import type { DirectorPlanMetadata, TurnDisplayEvent } from '../../types'
import type { DirectorStatusLike } from './types'
import { directorPlanTotals, directorStatusFallback, displayEventToChatMessage, formatBytes, formatShortDate } from './utils'

// 导演执行过程：以消息流形式展示后台导演的规划、工具调用记录。消息列表为虚拟滚动，
// 必须有确定高度，因此保留有界容器（区别于文档预览的自然高度）。
export function DirectorProcessCard({ status, metadata, loading, displayEvents }: {
  status?: DirectorStatusLike
  metadata?: DirectorPlanMetadata
  loading: boolean
  displayEvents: TurnDisplayEvent[]
}) {
  const { t } = useTranslation()
  const process = useDirectorProcessMessages({ status, metadata, loading, displayEvents })
  return (
    <section data-testid="director-process-panel" className="rounded-[12px] border border-[var(--nova-border)] bg-[var(--director-panel)] p-3">
      <div className="flex min-w-0 items-center gap-2 text-xs font-semibold text-[var(--nova-text)]">
        <Activity className="h-3.5 w-3.5 shrink-0 text-[var(--director-brass)]" />
        <span className="director-console__display truncate text-sm">{t('directorPanel.process.title')}</span>
      </div>
      <p className="mt-1 text-[11px] leading-5 text-[var(--nova-text-muted)]">{t('directorPanel.process.description')}</p>

      <div className="mt-3">
        {process.messages.length > 0 || process.streaming ? (
          <div className="flex h-[380px] min-h-[240px] flex-col overflow-hidden rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface)]">
            <MessageList
              messages={process.messages}
              isStreaming={process.streaming}
              activityContent={process.activityContent}
              scrollResetKey={process.scrollKey}
              bottomPaddingClassName="pb-3"
              messageStyle={{ fontSize: '12px', lineHeight: 1.55 }}
              collapseTraceBeforeAssistant
            />
          </div>
        ) : (
          <div className="flex min-h-[160px] items-center justify-center rounded-[10px] border border-dashed border-[var(--nova-border)] px-4 text-center text-xs leading-5 text-[var(--nova-text-muted)]">{t('directorPanel.process.empty')}</div>
        )}
      </div>
    </section>
  )
}

function useDirectorProcessMessages({
  status,
  metadata,
  loading,
  displayEvents,
}: {
  status?: DirectorStatusLike
  metadata?: DirectorPlanMetadata
  loading: boolean
  displayEvents: TurnDisplayEvent[]
}) {
  const { t } = useTranslation()
  return useMemo(() => {
    const currentStatus = loading && !status?.status ? 'loading' : status?.status || ''
    const running = currentStatus === 'running' || currentStatus === 'loading'
    const hasDirectorSignal = Boolean(currentStatus || status || metadata || displayEvents.length)
    const totals = directorPlanTotals(status, metadata)
    const summary = status?.error || status?.summary || directorStatusFallback(currentStatus, t)
    const updatedAt = status?.updated_at || metadata?.updated_at || ''
    const progress = t('directorPanel.directorChat.planProgress', {
      completed: totals.completed,
      planned: totals.planned,
      visible: formatBytes(totals.visibleBytes),
      total: formatBytes(totals.totalBytes),
      turns: metadata?.branch_planning_turns || 5,
    })
    const meta = updatedAt ? t('directorPanel.directorChat.updatedAt', { time: formatShortDate(updatedAt) }) : currentStatus || t('snapshot.noRecord')
    const toolStatus = currentStatus === 'failed' ? 'error' : running ? 'running' : 'success'
    const showFileTool = ['running', 'ready', 'failed', 'conflict'].includes(currentStatus)
    const persistedMessages = displayEvents.map((event, index) => displayEventToChatMessage(event, `director-event-${index}`))
    const fileToolMessages: ChatMessage[] = persistedMessages.length > 0
      ? persistedMessages
      : showFileTool
        ? [{
            id: 'director-run-tool',
            role: 'tool_call',
            name: 'edit_file',
            status: toolStatus,
            args: JSON.stringify({ file_path: 'director.md' }),
            result: toolStatus === 'success' ? progress : '',
            created_at: updatedAt,
          }]
        : []
    const directorMessages: ChatMessage[] = hasDirectorSignal ? [
      {
        id: 'director-run-request',
        role: 'user',
        content: t('directorPanel.directorChat.request'),
      },
      {
        id: 'director-run-thinking',
        role: 'thinking',
        content: summary,
        streaming: running,
        created_at: updatedAt,
      },
      ...fileToolMessages,
      {
        id: 'director-run-result',
        role: currentStatus === 'failed' ? 'error' : 'assistant',
        content: `${summary}\n\n${t('snapshot.director.plan')}: ${progress}\n${meta}`,
        streaming: running,
        created_at: updatedAt,
      },
    ] : []
    const messages = chatMessagesToAgentUIMessages(directorMessages)
    return {
      messages,
      streaming: running,
      activityContent: running ? summary : '',
      scrollKey: `director-process:${metadata?.revision || ''}:${currentStatus}:${updatedAt}`,
    }
  }, [displayEvents, loading, metadata, status, t])
}
