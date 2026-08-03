import { memo } from 'react'
import type { CSSProperties } from 'react'
import type { AgentAskAnswer, AgentAskResolution, ChapterIllustration } from '@/lib/api'
import { agentViewToRenderMessage, type AgentMessageView, type AgentPartRef } from '@/lib/agent-message-view'
import { MessageItem } from './MessageItem'

interface AgentMessageItemProps {
  view: AgentMessageView
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
  onOpenTrace?: (runID: string) => void
  onInteractiveCardLayoutChange?: (element?: HTMLElement) => void
  onResolveAsk?: (view: AgentMessageView, action: { status: 'answered'; answers: AgentAskAnswer[] } | { status: 'cancelled' }) => Promise<AgentAskResolution>
}

export const AgentMessageItem = memo(function AgentMessageItem({
  view,
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
  onOpenTrace,
  onInteractiveCardLayoutChange,
  onResolveAsk,
}: AgentMessageItemProps) {
  const message = agentViewToRenderMessage(view)
  if (!message) return null
  return (
    <MessageItem
      message={message}
      highlightDialogue={highlightDialogue}
      messageStyle={messageStyle}
      onEdit={onEditMessage ? () => onEditMessage(view) : undefined}
      onEditAssistantReply={onEditAssistantReply ? () => onEditAssistantReply(view) : undefined}
      onCreateBranch={onCreateBranch ? () => onCreateBranch(view) : undefined}
      onRegenerate={onRegenerateMessage ? () => onRegenerateMessage(view) : undefined}
      onSwitchVersion={onSwitchMessageVersion ? (_message, direction) => onSwitchMessageVersion(view, direction) : undefined}
      onOpenSubAgentSession={onOpenSubAgentSession ? () => onOpenSubAgentSession(view) : undefined}
      onInsertIllustration={onInsertIllustration}
      onGenerateInteractiveImage={onGenerateInteractiveImage ? () => onGenerateInteractiveImage(view) : undefined}
      generatingInteractiveImageTurnId={generatingInteractiveImageTurnId}
      activeSubAgentSessionKey={activeSubAgentSessionKey}
      subAgentPresentation={subAgentPresentation}
      onApprovePlan={onApprovePlan ? () => onApprovePlan(view.ref) : undefined}
      onContinuePlan={onContinuePlan ? () => onContinuePlan(view) : undefined}
      onExitPlanMode={onExitPlanMode}
      onOpenTrace={onOpenTrace}
      onInteractiveCardLayoutChange={onInteractiveCardLayoutChange}
      onResolveAsk={onResolveAsk ? (_message, action) => onResolveAsk(view, action) : undefined}
    />
  )
})
