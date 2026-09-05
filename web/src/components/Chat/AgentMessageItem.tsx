import { memo } from 'react'
import type { CSSProperties } from 'react'
import type { AgentAskAnswer, AgentAskResolution, ChapterIllustration } from '@/lib/api'
import { agentViewToRenderMessage, type AgentMessageView, type AgentPartRef } from '@/lib/agent-message-view'
import { subAgentSessionKey } from './subagent-session'
import { MessageItem, type AssistantMessagePresentation } from './MessageItem'

interface AgentMessageItemProps {
  projectId?: string
  view: AgentMessageView
  assistantPresentation?: AssistantMessagePresentation
  highlightDialogue?: boolean
  messageStyle?: CSSProperties
  onOpenSubAgentSession?: (view: AgentMessageView) => void
  onInsertIllustration?: (illustration: ChapterIllustration) => void
  onGenerateInteractiveImage?: (view: AgentMessageView) => void
  generatingInteractiveImageTurnId?: string
  activeSubAgentSessionKey?: string
  subAgentPresentation?: 'card' | 'content'
  onEditMessage?: (view: AgentMessageView) => void
  onEditAssistantReply?: (view: AgentMessageView) => void
  onCreateBranch?: (view: AgentMessageView) => void
  onRegenerateMessage?: (view: AgentMessageView) => void
  onSwitchMessageVersion?: (view: AgentMessageView, direction: -1 | 1) => void
  onApprovePlan?: (ref: AgentPartRef) => void
  onContinuePlan?: (view: AgentMessageView) => void
  onExitPlanMode?: () => void
  onInteractiveCardLayoutChange?: (element?: HTMLElement) => void
  onResolveAsk?: (view: AgentMessageView, action: { status: 'answered'; answers: AgentAskAnswer[] } | { status: 'cancelled' }) => Promise<AgentAskResolution>
}

export const AgentMessageItem = memo(function AgentMessageItem({
  projectId,
  view,
  assistantPresentation,
  highlightDialogue = false,
  messageStyle,
  onOpenSubAgentSession,
  onInsertIllustration,
  onGenerateInteractiveImage,
  generatingInteractiveImageTurnId,
  activeSubAgentSessionKey,
  subAgentPresentation = 'card',
  onEditMessage,
  onEditAssistantReply,
  onCreateBranch,
  onRegenerateMessage,
  onSwitchMessageVersion,
  onApprovePlan,
  onContinuePlan,
  onExitPlanMode,
  onInteractiveCardLayoutChange,
  onResolveAsk,
}: AgentMessageItemProps) {
  const message = agentViewToRenderMessage(view)
  if (!message) return null
  const openSubAgentSessionFromMessage = onOpenSubAgentSession ? (openedMessage: typeof message) => {
    const sessionKey = subAgentSessionKey(openedMessage)
    if (!sessionKey) return
    onOpenSubAgentSession({
      ...view,
      metadata: { ...view.metadata, subagent: true, subagent_session_id: sessionKey },
    })
  } : undefined
  return (
    <MessageItem
      projectId={projectId}
      message={message}
      assistantPresentation={assistantPresentation}
      highlightDialogue={highlightDialogue}
      messageStyle={messageStyle}
      onEdit={onEditMessage ? () => onEditMessage(view) : undefined}
      onEditAssistantReply={onEditAssistantReply ? () => onEditAssistantReply(view) : undefined}
      onCreateBranch={onCreateBranch ? () => onCreateBranch(view) : undefined}
      onRegenerate={onRegenerateMessage ? () => onRegenerateMessage(view) : undefined}
      onSwitchVersion={onSwitchMessageVersion ? (_message, direction) => onSwitchMessageVersion(view, direction) : undefined}
      onOpenSubAgentSession={openSubAgentSessionFromMessage}
      onInsertIllustration={onInsertIllustration}
      onGenerateInteractiveImage={onGenerateInteractiveImage ? () => onGenerateInteractiveImage(view) : undefined}
      generatingInteractiveImageTurnId={generatingInteractiveImageTurnId}
      activeSubAgentSessionKey={activeSubAgentSessionKey}
      subAgentPresentation={subAgentPresentation}
      onApprovePlan={onApprovePlan ? () => onApprovePlan(view.ref) : undefined}
      onContinuePlan={onContinuePlan ? () => onContinuePlan(view) : undefined}
      onExitPlanMode={onExitPlanMode}
      onInteractiveCardLayoutChange={onInteractiveCardLayoutChange}
      onResolveAsk={onResolveAsk ? (_message, action) => onResolveAsk(view, action) : undefined}
    />
  )
})
