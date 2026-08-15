import { useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { AgentAskAnswer, AgentAskResolution, ChapterIllustration, ChatMessage } from '@/lib/api'
import {
  agentSubAgentSessionKey,
  agentViewAskInteraction,
  agentViewContent,
  agentViewStableKey,
  agentViewToRenderMessage,
  buildAgentSubAgentTimelineGroups,
  isAgentSubAgentTimelineView,
  type AgentMessageView,
} from '@/lib/agent-message-view'
import { AgentMessageItem } from './AgentMessageItem'
import { MessageItem } from './MessageItem'
import { buildSubAgentProgressMessage } from './subagent-session'
import { buildToolCallTree, type AgentProcessNode } from './tool-call-tree'

interface AgentExecutionProcessProps {
  projectId?: string
  active: boolean
  activeSubAgentSessionKey?: string
  activeTraceDisplay: 'expanded' | 'collapsed'
  highlightDialogue: boolean
  messageStyle?: CSSProperties
  onGenerateInteractiveImage?: (view: AgentMessageView) => void
  onInsertIllustration?: (illustration: ChapterIllustration) => void
  onOpenTrace?: (runID: string) => void
  onOpenSubAgentSession?: (view: AgentMessageView) => void
  onInteractiveCardLayoutChange?: (element?: HTMLElement) => void
  onResolveAsk?: (view: AgentMessageView, action: { status: 'answered'; answers: AgentAskAnswer[] } | { status: 'cancelled' }) => Promise<AgentAskResolution>
  views: AgentMessageView[]
}

/** One disclosure for every non-terminal display segment in an Agent run. */
export function AgentExecutionProcess({
  projectId,
  active,
  activeSubAgentSessionKey,
  activeTraceDisplay,
  highlightDialogue,
  messageStyle,
  onGenerateInteractiveImage,
  onInsertIllustration,
  onOpenTrace,
  onOpenSubAgentSession,
  onInteractiveCardLayoutChange,
  onResolveAsk,
  views,
}: AgentExecutionProcessProps) {
  const { t } = useTranslation()
  const running = active || views.some((view) => view.streaming || view.status === 'running')
  const [expanded, setExpanded] = useState(activeTraceDisplay === 'expanded' && running)
  const userToggledRef = useRef(false)
  const wasRunningRef = useRef(running)
  const progressCount = views.filter((view) => !view.metadata.subagent && view.kind === 'assistant' && agentViewContent(view).trim()).length
  const toolCount = views.filter((view) => view.kind === 'tool').length
  const subAgentCount = new Set(views.filter(isAgentSubAgentTimelineView).map(agentSubAgentSessionKey)).size
  const label = [
    running ? t('chat.trace.executing') : t('chat.trace.execution'),
    progressCount > 0 ? t('chat.trace.progressUpdates', { count: progressCount }) : '',
    toolCount > 0 ? t('chat.trace.toolCalls', { count: toolCount }) : '',
    subAgentCount > 0 ? t('chat.subagent.label') : '',
  ].filter(Boolean).join(' · ')
  const processTree = useMemo(() => buildToolCallTree(views), [views])
  useLayoutEffect(() => {
    const wasRunning = wasRunningRef.current
    wasRunningRef.current = running
    if (wasRunning && !running) {
      userToggledRef.current = false
      setExpanded(false)
    } else if (running && !userToggledRef.current && activeTraceDisplay === 'expanded') {
      setExpanded(true)
    }
  }, [activeTraceDisplay, running])

  const renderProcessItems = (nodes: AgentProcessNode[], depth = 0) => {
    const processItems: ReactNode[] = []
    const nodeViews = nodes.map(node => node.view)
    const subAgentGroupsByStart = new Map(
      (onOpenSubAgentSession ? buildAgentSubAgentTimelineGroups(nodeViews) : []).map(group => [group.startIndex, group]),
    )
    for (let index = 0; index < nodes.length; index += 1) {
      const node = nodes[index]
      const view = node.view
      const subAgentGroup = subAgentGroupsByStart.get(index)
      if (onOpenSubAgentSession && subAgentGroup) {
        const pendingApprovalView = subAgentGroup.views.find(item => agentViewAskInteraction(item)?.status === 'pending')
        if (pendingApprovalView) {
          processItems.push(
            <AgentMessageItem
              projectId={projectId}
              key={`subagent-approval-${subAgentGroup.key}`}
              view={pendingApprovalView}
              highlightDialogue={highlightDialogue}
              messageStyle={messageStyle}
              onOpenTrace={onOpenTrace}
              onInteractiveCardLayoutChange={onInteractiveCardLayoutChange}
              onResolveAsk={onResolveAsk}
            />,
          )
          index = subAgentGroup.nextIndex - 1
          continue
        }
        const progress = buildSubAgentProgressMessage(subAgentGroup.views.map(item => agentViewToRenderMessage(item)).filter((item): item is ChatMessage => Boolean(item)))
        if (progress) {
          processItems.push(
            <MessageItem
              projectId={projectId}
              key={`subagent-${subAgentGroup.key}`}
              message={progress}
              highlightDialogue={highlightDialogue}
              messageStyle={messageStyle}
              onOpenSubAgentSession={() => onOpenSubAgentSession(subAgentGroup.views[0])}
              activeSubAgentSessionKey={activeSubAgentSessionKey}
              onOpenTrace={onOpenTrace}
            />,
          )
          index = subAgentGroup.nextIndex - 1
          continue
        }
      }
      if (view.kind === 'reasoning' && !view.streaming && !agentViewContent(view).trim()) continue
      processItems.push(
        <div key={agentViewStableKey(view) || index} data-tool-call-depth={depth} className="min-w-0">
          <AgentMessageItem
            projectId={projectId}
            view={view}
            assistantPresentation={view.kind === 'assistant' ? 'progress' : undefined}
            highlightDialogue={highlightDialogue}
            messageStyle={messageStyle}
            onInsertIllustration={onInsertIllustration}
            onGenerateInteractiveImage={onGenerateInteractiveImage}
            onOpenTrace={onOpenTrace}
            onInteractiveCardLayoutChange={onInteractiveCardLayoutChange}
            onResolveAsk={onResolveAsk}
          />
          {node.children.length > 0 ? (
            <div className={`${depth < 3 ? 'ml-3' : 'ml-0'} mt-2 space-y-2 border-l border-[var(--nova-border)] pl-3`}>
              {renderProcessItems(node.children, depth + 1)}
            </div>
          ) : null}
        </div>,
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
            {renderProcessItems(processTree)}
          </div>
        ) : null}
      </div>
    </div>
  )
}
