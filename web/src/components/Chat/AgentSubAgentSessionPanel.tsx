import { useCallback, useMemo } from 'react'
import type { CSSProperties } from 'react'
import { Bot, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Virtuoso } from 'react-virtuoso'
import type { Components } from 'react-virtuoso'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { agentSubAgentSessionKey, agentViewContent, buildAgentMessageViews, type AgentMessageView } from '@/lib/agent-message-view'
import { AgentMessageItem } from './AgentMessageItem'
import { VIRTUOSO_BOTTOM_THRESHOLD, useVirtuosoBottomLock } from './useVirtuosoBottomLock'
import { ScrollToBottomButton } from './ScrollToBottomButton'

interface AgentSubAgentSessionPanelProps {
  messages: AgentUIMessage[]
  sessionKey: string
  onClose: () => void
  highlightDialogue?: boolean
  messageStyle?: CSSProperties
}

const SUBAGENT_SESSION_COMPONENTS: Components<AgentMessageView> = {
  Header: SubAgentSessionListPadding,
  Footer: SubAgentSessionListPadding,
}

export function AgentSubAgentSessionPanel({ messages, sessionKey, onClose, highlightDialogue = false, messageStyle }: AgentSubAgentSessionPanelProps) {
  const { t } = useTranslation()
  const sessionViews = useMemo(
    () => buildAgentMessageViews(messages).filter((view) => agentSubAgentSessionKey(view) === sessionKey && view.kind !== 'token-usage'),
    [messages, sessionKey],
  )
  const first = sessionViews[0]
  const name = first?.metadata.agent_name || first?.metadata.subagent_type || t('chat.subagent.label')
  const running = sessionViews.some((view) => view.streaming)
  const scrollContentKey = useMemo(() => sessionViews.map((view) => [
    view.partId || '',
    view.kind || '',
    view.status || '',
    view.streaming ? 'streaming' : '',
    agentViewContent(view).length,
  ].join(':')).join('|'), [sessionViews])
  const scrollLock = useVirtuosoBottomLock({
    resetKey: sessionKey,
    contentKey: scrollContentKey,
    itemCount: sessionViews.length,
  })
  const itemContent = useCallback((index: number, view?: AgentMessageView) => {
    const resolvedView = view || sessionViews[index]
    if (!resolvedView) return null
    return (
      <div data-nova-chat-item="subagent-message" className="min-w-0 px-4 pb-3 last:pb-0">
        <AgentMessageItem
          view={resolvedView}
          highlightDialogue={highlightDialogue}
          messageStyle={messageStyle}
          subAgentPresentation="content"
        />
      </div>
    )
  }, [highlightDialogue, messageStyle, sessionViews])

  return (
    <section className="flex h-full min-h-0 flex-col border-l border-[var(--nova-border)] bg-[var(--nova-surface-2)] shadow-[-12px_0_26px_-24px_rgba(15,23,42,0.82)]">
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-3">
        <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]">
          <Bot className="h-3.5 w-3.5" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="truncate text-xs font-medium text-[var(--nova-text)]">{t('chat.subagent.sessionTitle', { name })}</div>
          <div className="truncate text-[10px] text-[var(--nova-text-faint)]">{running ? t('chat.subagent.status.streaming') : t('chat.subagent.status.done')}</div>
        </div>
        <button
          type="button"
          onClick={onClose}
          className="nova-nav-item rounded p-1"
          aria-label={t('chat.subagent.closeSession')}
          title={t('common.close')}
        >
          <X className="h-3.5 w-3.5" />
        </button>
      </div>
      {sessionViews.length === 0 ? (
        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4 [overflow-anchor:none]">
          <div className="rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-4 text-xs text-[var(--nova-text-faint)]">
            {t('chat.subagent.empty')}
          </div>
        </div>
      ) : (
        <div className="relative flex min-h-0 flex-1 flex-col">
          <Virtuoso
            ref={scrollLock.virtuosoRef}
            scrollerRef={scrollLock.scrollerRef}
            onScroll={scrollLock.onScroll}
            onWheel={scrollLock.onWheel}
            onKeyDown={scrollLock.onKeyDown}
            atBottomStateChange={scrollLock.onAtBottomStateChange}
            atBottomThreshold={VIRTUOSO_BOTTOM_THRESHOLD}
            followOutput={scrollLock.followOutput}
            initialItemCount={Math.min(sessionViews.length, 40)}
            data={sessionViews}
            components={SUBAGENT_SESSION_COMPONENTS}
            computeItemKey={(index, view) => subAgentSessionMessageKey(view, index)}
            itemContent={itemContent}
            overscan={{ main: 360, reverse: 180 }}
            increaseViewportBy={{ top: 300, bottom: 560 }}
            className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden [overflow-anchor:none]"
            aria-label={t('chat.subagent.sessionTitle', { name })}
          />
          <ScrollToBottomButton
            visible={scrollLock.isAwayFromBottom}
            onClick={scrollLock.scrollToBottom}
            bottomOffsetPx={16}
            rightOffsetPx={16}
          />
        </div>
      )}
    </section>
  )
}

function SubAgentSessionListPadding() {
  return <div aria-hidden="true" className="h-4 shrink-0" />
}

function subAgentSessionMessageKey(view: AgentMessageView | undefined, index: number) {
  if (view?.partId) return view.partId
  if (view?.metadata.created_at) return `${view.metadata.created_at}-${index}`
  return index
}
