import { useMemo } from 'react'
import type { CSSProperties } from 'react'
import { Bot, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { AgentUIMessage } from '@/lib/agent-ui'
import type { AgentAskAnswer, AgentAskResolution } from '@/lib/api'
import { buildAgentMessageViews, type AgentMessageView } from '@/lib/agent-message-view'
import { MessageList, type AgentMessageListProjection } from './AgentMessageList'
import { isActiveSubAgentStatus, selectSubAgentSessionViews, subAgentStatusFromViews, subAgentStatusTranslationKey } from './subagent-session'

export interface AgentSubAgentSessionTarget {
  parentSessionId: string
  sessionKey: string
  name: string
}

export type AgentSubAgentAskResolver = (
  view: AgentMessageView,
  action: { status: 'answered'; answers: AgentAskAnswer[] } | { status: 'cancelled' },
) => Promise<AgentAskResolution>

interface AgentSubAgentSessionPanelBaseProps {
  projectId?: string
  messages: AgentUIMessage[]
  sessionKey: string
  highlightDialogue?: boolean
  messageStyle?: CSSProperties
  onResolveAsk?: AgentSubAgentAskResolver
}

type AgentSubAgentSessionPanelProps = AgentSubAgentSessionPanelBaseProps & (
  | { chrome?: 'panel'; onClose: () => void }
  | { chrome: 'tab'; onClose?: never }
)

export function AgentSubAgentSessionPanel(props: AgentSubAgentSessionPanelProps) {
  const { projectId, messages, sessionKey, highlightDialogue = false, messageStyle, onResolveAsk } = props
  const { t } = useTranslation()
  const projection = useMemo<AgentMessageListProjection>(() => {
    const views = selectSubAgentSessionViews(buildAgentMessageViews(messages), sessionKey)
    return {
      views,
      initialPosition: views.some(view => view.streaming) ? 'end' : 'start',
      subAgentPresentation: 'content',
    }
  }, [messages, sessionKey])
  const sessionViews = projection.views
  const first = sessionViews[0]
  const name = first?.metadata.agent_name || first?.metadata.subagent_type || t('chat.subagent.label')
  const status = subAgentStatusFromViews(sessionViews)
  const running = status ? isActiveSubAgentStatus(status) : sessionViews.some((view) => view.streaming)

  return (
    <section
      aria-label={t('chat.subagent.sessionTitle', { name })}
      className={`flex h-full min-h-0 flex-col bg-[var(--nova-surface-2)] ${props.chrome !== 'tab' ? 'border-l border-[var(--nova-border)] shadow-[-12px_0_26px_-24px_rgba(0,0,0,0.72)]' : ''}`}
    >
      {props.chrome !== 'tab' ? (
        <div className="flex h-10 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-3">
          <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]">
            <Bot className="h-3.5 w-3.5" />
          </span>
          <div className="min-w-0 flex-1">
            <div className="truncate text-xs font-medium text-[var(--nova-text)]">{t('chat.subagent.sessionTitle', { name })}</div>
            <div className="truncate text-[10px] text-[var(--nova-text-faint)]">{t(subAgentStatusTranslationKey(status, running))}</div>
          </div>
          <button
            type="button"
            onClick={props.onClose}
            className="nova-nav-item rounded p-1"
            aria-label={t('chat.subagent.closeSession')}
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </div>
      ) : null}
      {sessionViews.length === 0 ? (
        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4 [overflow-anchor:none]">
          <div className="rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-4 text-xs text-[var(--nova-text-faint)]">
            {t('chat.subagent.empty')}
          </div>
        </div>
      ) : (
        <MessageList
          projectId={projectId}
          messages={messages}
          projection={projection}
          isStreaming={running}
          isExecutionActive={running}
          activityContent=""
          scrollResetKey={sessionKey}
          bottomPaddingPx={16}
          contentClassName="px-4"
          highlightDialogue={highlightDialogue}
          messageStyle={messageStyle}
          onResolveAsk={onResolveAsk}
        />
      )}
    </section>
  )
}
