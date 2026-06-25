import type { CSSProperties } from 'react'
import { Bot, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ChatMessage } from '@/lib/api'
import { useBottomScrollLock } from '@/hooks/useBottomScrollLock'
import { MessageItem } from './MessageItem'
import { subAgentSessionKey } from './subagent-session'

interface SubAgentSessionPanelProps {
  messages: ChatMessage[]
  sessionKey: string
  onClose: () => void
  highlightDialogue?: boolean
  messageStyle?: CSSProperties
}

export function SubAgentSessionPanel({ messages, sessionKey, onClose, highlightDialogue = false, messageStyle }: SubAgentSessionPanelProps) {
  const { t } = useTranslation()
  const sessionMessages = messages.filter((message) => subAgentSessionKey(message) === sessionKey && message.role !== 'token_usage')
  const first = sessionMessages[0]
  const name = first?.agent_name || first?.subagent_type || t('chat.subagent.label')
  const running = sessionMessages.some((message) => message.streaming)
  const scrollContentKey = sessionMessages.map((message) => [
    message.id || '',
    message.role || '',
    message.status || '',
    message.streaming ? 'streaming' : '',
    (message.content || '').length,
    (message.args || '').length,
    (message.result || '').length,
  ].join(':')).join('|')
  const scrollLock = useBottomScrollLock<HTMLDivElement>({
    resetKey: sessionKey,
    contentKey: scrollContentKey,
  })

  return (
    <section className="flex h-full min-h-0 flex-col border-l border-[var(--nova-border)] bg-[var(--nova-surface)]">
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-3">
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
      <div
        ref={scrollLock.ref}
        onScroll={scrollLock.onScroll}
        onWheel={scrollLock.onWheel}
        onKeyDown={scrollLock.onKeyDown}
        className="min-h-0 flex-1 space-y-3 overflow-y-auto px-4 py-4 [overflow-anchor:none]"
      >
        {sessionMessages.length === 0 ? (
          <div className="rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-4 text-xs text-[var(--nova-text-faint)]">
            {t('chat.subagent.empty')}
          </div>
        ) : (
          sessionMessages.map((message, index) => (
            <MessageItem
              key={message.id || message.created_at || index}
              message={message}
              highlightDialogue={highlightDialogue}
              messageStyle={messageStyle}
              subAgentPresentation="content"
            />
          ))
        )}
      </div>
    </section>
  )
}
