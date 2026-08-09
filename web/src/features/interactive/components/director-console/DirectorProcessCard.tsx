import { useMemo } from 'react'
import { Activity } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { MessageList } from '@/components/Chat/MessageList'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { createAgentDataMessage, createAgentReasoningMessage, createAgentTextMessage, createAgentToolMessage } from '@/lib/agent-ui-message'
import type { DirectorPlanMetadata, TurnDisplayEvent } from '../../types'
import type { DirectorStatusLike } from './types'
import { directorPlanTotals, directorStatusFallback, displayEventToAgentUIMessage, formatBytes, formatShortDate } from './utils'

// 导演执行过程：以消息流形式展示后台导演的规划、工具调用记录。消息列表为虚拟滚动，
// 必须有确定高度，因此保留有界容器（区别于文档预览的自然高度）。
export function DirectorProcessCard({ projectId, status, metadata, loading, displayEvents }: {
  projectId: string
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
              projectId={projectId}
              messages={process.messages}
              isStreaming={process.streaming}
              activityContent={process.activityContent}
              scrollResetKey={process.scrollKey}
              bottomPaddingClassName="pb-3"
              messageStyle={{ fontSize: '12px', lineHeight: 1.55 }}
              collapseTraceGroups
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
    const persistedMessages = displayEvents.map((event, index) => displayEventToAgentUIMessage(event, `director-event-${index}`))
    const fileToolMessages: AgentUIMessage[] = persistedMessages.length > 0
      ? persistedMessages
      : showFileTool
        ? [createAgentToolMessage({
            id: 'director-run-tool',
            name: 'edit',
            state: toolStatus === 'error' ? 'output-error' : toolStatus === 'success' ? 'output-available' : 'input-available',
            input: { path: 'director.md' },
            output: toolStatus === 'success' ? progress : undefined,
            metadata: { created_at: updatedAt || undefined, display_role: 'tool_call' },
          })]
        : []
    const directorMessages: AgentUIMessage[] = hasDirectorSignal ? [
      createAgentTextMessage({
        id: 'director-run-request',
        role: 'user',
        text: t('directorPanel.directorChat.request'),
        metadata: { display_role: 'user' },
      }),
      createAgentReasoningMessage({
        id: 'director-run-thinking',
        text: summary,
        state: running ? 'streaming' : 'done',
        metadata: { created_at: updatedAt || undefined, display_role: 'thinking' },
      }),
      ...fileToolMessages,
      currentStatus === 'failed'
        ? createAgentDataMessage({
            id: 'director-run-result',
            type: 'agent-error',
            metadata: { created_at: updatedAt || undefined, display_role: 'error' },
            data: { id: 'director-run-result', role: 'error', content: `${summary}\n\n${t('snapshot.director.plan')}: ${progress}\n${meta}` },
          })
        : createAgentTextMessage({
            id: 'director-run-result',
            role: 'assistant',
            text: `${summary}\n\n${t('snapshot.director.plan')}: ${progress}\n${meta}`,
            state: running ? 'streaming' : 'done',
            metadata: { created_at: updatedAt || undefined, display_role: 'assistant' },
          }),
    ] : []
    return {
      messages: directorMessages,
      streaming: running,
      activityContent: running ? summary : '',
      scrollKey: `director-process:${metadata?.revision || ''}:${currentStatus}:${updatedAt}`,
    }
  }, [displayEvents, loading, metadata, status, t])
}
