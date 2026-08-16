import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import type { AgentAskAnswer, AgentAskResolution, ChapterIllustration, ChatMessage } from '@/lib/api'
import {
  agentSubAgentSessionKey,
  agentViewAskInteraction,
  agentViewContent,
  agentViewStableKey,
  agentViewToRenderMessage,
  buildAgentSubAgentTimelineGroups,
  isAgentSubAgentTimelineView,
  type AgentExecutionTiming,
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
  timing?: AgentExecutionTiming
  views: AgentMessageView[]
}

/** One stable disclosure for the non-terminal timeline of an Agent run. */
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
  timing,
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
  const duration = useExecutionDuration(timing, running)
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
      <Collapsible
        open={expanded}
        onOpenChange={(open) => {
          userToggledRef.current = true
          setExpanded(open)
        }}
        className="w-full"
      >
        <CollapsibleTrigger
          type="button"
          className="group flex min-w-0 flex-wrap items-center gap-1 py-1 text-left text-xs text-[var(--nova-text-muted)] transition-colors hover:text-[var(--nova-text)]"
        >
          {running ? <span aria-hidden="true" className="size-1.5 animate-pulse rounded-full bg-[var(--nova-text-muted)]" /> : null}
          <span>{label}</span>
          {duration ? (
            <>
              <span aria-hidden="true">·</span>
              <span className="font-mono tabular-nums">{duration}</span>
            </>
          ) : null}
          <ChevronRight
            aria-hidden="true"
            data-agent-execution-toggle-icon
            className={`size-3 opacity-60 transition-[transform,opacity] duration-[var(--nova-motion-fast)] ease-[var(--nova-panel-motion-ease)] group-hover:opacity-100 ${expanded ? 'rotate-90' : ''}`}
          />
        </CollapsibleTrigger>
        <CollapsibleContent data-agent-execution-content className="nova-agent-execution-content">
          <div className="flex flex-col gap-2 py-2">
            {renderProcessItems(processTree)}
          </div>
        </CollapsibleContent>
      </Collapsible>
    </div>
  )
}

function useExecutionDuration(timing: AgentExecutionTiming | undefined, running: boolean) {
  const [, refresh] = useState(0)
  const hasFinalDuration = timing?.durationMS !== undefined && Number.isFinite(timing.durationMS) && timing.durationMS >= 0
  const hasLiveStart = running && timing?.startedAtMS !== undefined && Number.isFinite(timing.startedAtMS)

  useEffect(() => {
    if (hasFinalDuration || !hasLiveStart) return undefined
    const interval = window.setInterval(() => refresh(value => value + 1), 1_000)
    return () => window.clearInterval(interval)
  }, [hasFinalDuration, hasLiveStart, timing?.startedAtMS])

  if (hasFinalDuration) return formatExecutionDuration(timing.durationMS as number)
  if (!hasLiveStart) return ''
  return formatExecutionDuration(Math.max(0, Date.now() - (timing?.startedAtMS as number)))
}

export function formatExecutionDuration(milliseconds: number) {
  const totalSeconds = Math.max(0, Math.floor(milliseconds / 1_000))
  const seconds = totalSeconds % 60
  const totalMinutes = Math.floor(totalSeconds / 60)
  if (totalMinutes === 0) return `${seconds}s`
  const minutes = totalMinutes % 60
  const hours = Math.floor(totalMinutes / 60)
  if (hours === 0) return `${minutes}m${seconds}s`
  return `${hours}h${minutes}m${seconds}s`
}
