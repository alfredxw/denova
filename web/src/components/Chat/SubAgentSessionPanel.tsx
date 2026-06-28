import { useCallback, useMemo } from 'react'
import type { CSSProperties } from 'react'
import { Bot, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Virtuoso } from 'react-virtuoso'
import type { Components } from 'react-virtuoso'
import type { ChatMessage } from '@/lib/api'
import { MessageItem } from './MessageItem'
import { subAgentSessionKey } from './subagent-session'
import { VIRTUOSO_BOTTOM_THRESHOLD, useVirtuosoBottomLock } from './useVirtuosoBottomLock'

interface SubAgentSessionPanelProps {
  messages: ChatMessage[]
  sessionKey: string
  onClose: () => void
  highlightDialogue?: boolean
  messageStyle?: CSSProperties
}

const SUBAGENT_SESSION_COMPONENTS: Components<ChatMessage> = {
  Header: SubAgentSessionListPadding,
  Footer: SubAgentSessionListPadding,
}

export function SubAgentSessionPanel({ messages, sessionKey, onClose, highlightDialogue = false, messageStyle }: SubAgentSessionPanelProps) {
  const { t } = useTranslation()
  const sessionMessages = useMemo(
    () => messages.filter((message) => subAgentSessionKey(message) === sessionKey && message.role !== 'token_usage'),
    [messages, sessionKey],
  )
  const first = sessionMessages[0]
  const name = first?.agent_name || first?.subagent_type || t('chat.subagent.label')
  const running = sessionMessages.some((message) => message.streaming)
  const scrollContentKey = useMemo(() => sessionMessages.map((message) => [
    message.id || '',
    message.role || '',
    message.status || '',
    message.streaming ? 'streaming' : '',
    (message.content || '').length,
    (message.args || '').length,
    (message.result || '').length,
  ].join(':')).join('|'), [sessionMessages])
  const scrollLock = useVirtuosoBottomLock({
    resetKey: sessionKey,
    contentKey: scrollContentKey,
    itemCount: sessionMessages.length,
  })
  const itemContent = useCallback((index: number, message?: ChatMessage) => {
    const resolvedMessage = message || sessionMessages[index]
    if (!resolvedMessage) return null
    return (
      <div data-nova-chat-item="subagent-message" className="min-w-0 px-4 pb-3 last:pb-0">
        <MessageItem
          message={resolvedMessage}
          highlightDialogue={highlightDialogue}
          messageStyle={messageStyle}
          subAgentPresentation="content"
        />
      </div>
    )
  }, [highlightDialogue, messageStyle, sessionMessages])

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
      {sessionMessages.length === 0 ? (
        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4 [overflow-anchor:none]">
          <div className="rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-4 text-xs text-[var(--nova-text-faint)]">
            {t('chat.subagent.empty')}
          </div>
        </div>
      ) : (
        <Virtuoso
          ref={scrollLock.virtuosoRef}
          scrollerRef={scrollLock.scrollerRef}
          onScroll={scrollLock.onScroll}
          onWheel={scrollLock.onWheel}
          onKeyDown={scrollLock.onKeyDown}
          atBottomStateChange={scrollLock.onAtBottomStateChange}
          atBottomThreshold={VIRTUOSO_BOTTOM_THRESHOLD}
          followOutput={scrollLock.followOutput}
          initialItemCount={Math.min(sessionMessages.length, 40)}
          data={sessionMessages}
          components={SUBAGENT_SESSION_COMPONENTS}
          computeItemKey={(index, message) => message?.id || message?.created_at || index}
          itemContent={itemContent}
          overscan={{ main: 360, reverse: 180 }}
          increaseViewportBy={{ top: 300, bottom: 560 }}
          className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden [overflow-anchor:none]"
          aria-label={t('chat.subagent.sessionTitle', { name })}
        />
      )}
    </section>
  )
}

function SubAgentSessionListPadding() {
  return <div aria-hidden="true" className="h-4 shrink-0" />
}
