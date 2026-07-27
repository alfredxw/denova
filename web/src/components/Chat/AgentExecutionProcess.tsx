import { useEffect, useRef, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ChapterIllustration, ChatMessage } from '@/lib/api'
import {
  agentSubAgentSessionKey,
  agentViewContent,
  agentViewToRenderMessage,
  isAgentSubAgentTimelineView,
  type AgentMessageView,
} from '@/lib/agent-message-view'
import { AgentMessageItem } from './AgentMessageItem'
import { MessageItem } from './MessageItem'
import { StreamingContentStage } from './StreamingContentStage'
import { buildSubAgentProgressMessage } from './subagent-session'

interface AgentExecutionProcessProps {
  active: boolean
  activeSubAgentSessionKey?: string
  activeTraceDisplay: 'expanded' | 'collapsed'
  highlightDialogue: boolean
  messageStyle?: CSSProperties
  onGenerateInteractiveImage?: (view: AgentMessageView) => void
  onInsertIllustration?: (illustration: ChapterIllustration) => void
  onOpenTrace?: (runID: string) => void
  onOpenSubAgentSession?: (view: AgentMessageView) => void
  views: AgentMessageView[]
}

/** One disclosure for every non-terminal display segment in an Agent run. */
export function AgentExecutionProcess({
  active,
  activeSubAgentSessionKey,
  activeTraceDisplay,
  highlightDialogue,
  messageStyle,
  onGenerateInteractiveImage,
  onInsertIllustration,
  onOpenTrace,
  onOpenSubAgentSession,
  views,
}: AgentExecutionProcessProps) {
  const { t } = useTranslation()
  const running = active || views.some((view) => view.streaming || view.status === 'running')
  const [expanded, setExpanded] = useState(activeTraceDisplay === 'expanded' && running)
  const userToggledRef = useRef(false)
  const progressCount = views.filter((view) => !view.metadata.subagent && view.kind === 'assistant' && agentViewContent(view).trim()).length
  const toolCount = views.filter((view) => view.kind === 'tool').length
  const subAgentCount = new Set(views.filter(isAgentSubAgentTimelineView).map(agentSubAgentSessionKey)).size
  const label = [
    running ? t('chat.trace.executing') : t('chat.trace.execution'),
    progressCount > 0 ? t('chat.trace.progressUpdates', { count: progressCount }) : '',
    toolCount > 0 ? t('chat.trace.toolCalls', { count: toolCount }) : '',
    subAgentCount > 0 ? t('chat.subagent.label') : '',
  ].filter(Boolean).join(' · ')
  useEffect(() => {
    if (running) {
      if (activeTraceDisplay === 'expanded') {
        userToggledRef.current = false
        setExpanded(true)
      }
      return
    }
    if (!userToggledRef.current) setExpanded(false)
  }, [activeTraceDisplay, running])

  const renderProcessItems = () => {
    const processItems: ReactNode[] = []
    for (let index = 0; index < views.length; index += 1) {
      const view = views[index]
      if (onOpenSubAgentSession && isAgentSubAgentTimelineView(view)) {
        const sessionKey = agentSubAgentSessionKey(view)
        const group: AgentMessageView[] = []
        let nextIndex = index
        while (nextIndex < views.length && isAgentSubAgentTimelineView(views[nextIndex]) && agentSubAgentSessionKey(views[nextIndex]) === sessionKey) {
          group.push(views[nextIndex])
          nextIndex += 1
        }
        const progress = buildSubAgentProgressMessage(group.map(item => agentViewToRenderMessage(item)).filter((item): item is ChatMessage => Boolean(item)))
        if (progress) {
          processItems.push(
            <MessageItem
              key={`subagent-${sessionKey}`}
              message={progress}
              highlightDialogue={highlightDialogue}
              messageStyle={messageStyle}
              onOpenSubAgentSession={() => onOpenSubAgentSession(group[0])}
              activeSubAgentSessionKey={activeSubAgentSessionKey}
              onOpenTrace={onOpenTrace}
            />,
          )
          index = nextIndex - 1
          continue
        }
      }
      processItems.push(
        view.kind === 'reasoning'
          ? (
            <div key={view.partId || index} className="whitespace-pre-wrap text-xs leading-relaxed text-[var(--nova-text-muted)]">
              <StreamingContentStage
                content={agentViewContent(view)}
                targetContent={view.streaming ? view.metadata.streaming_target_content : undefined}
                streaming={view.streaming}
              >
                {(value) => value}
              </StreamingContentStage>
            </div>
          )
          : (
            <AgentMessageItem
              key={view.partId || index}
              view={view}
              highlightDialogue={highlightDialogue}
              messageStyle={messageStyle}
              onInsertIllustration={onInsertIllustration}
              onGenerateInteractiveImage={onGenerateInteractiveImage}
              onOpenTrace={onOpenTrace}
            />
          ),
      )
    }
    return processItems
  }

  return (
    <div className="flex justify-start" data-agent-execution-process>
      <div className="w-full">
        <button
          type="button"
          className="flex items-center gap-1 py-1 text-xs text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]"
          aria-expanded={expanded}
          onClick={() => {
            userToggledRef.current = true
            setExpanded(current => !current)
          }}
        >
          {expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
          {running ? <span aria-hidden="true" className="h-1.5 w-1.5 animate-pulse rounded-full bg-[var(--nova-text-muted)]" /> : null}
          {label}
        </button>
        {expanded ? (
          <div className="space-y-2 border-l border-[var(--nova-border)] px-3 py-2">
            {renderProcessItems()}
          </div>
        ) : null}
      </div>
    </div>
  )
}
