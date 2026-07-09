import { memo } from 'react'
import type { CSSProperties } from 'react'
import type { ChapterIllustration } from '@/lib/api'
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
  onRegenerateMessage?: (view: AgentMessageView) => void
  onSwitchMessageVersion?: (view: AgentMessageView, direction: -1 | 1) => void
  onSubmitPlanQuestion?: (ref: AgentPartRef, content: string, preview: string) => void
  onApprovePlan?: (ref: AgentPartRef) => void
  onContinuePlan?: (view: AgentMessageView) => void
  onExitPlanMode?: () => void
  onOpenTrace?: (runID: string) => void
  onPlanCardLayoutChange?: () => void
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
  onRegenerateMessage,
  onSwitchMessageVersion,
  onSubmitPlanQuestion,
  onApprovePlan,
  onContinuePlan,
  onExitPlanMode,
  onOpenTrace,
  onPlanCardLayoutChange,
}: AgentMessageItemProps) {
  const message = agentViewToRenderMessage(view)
  if (!message) return null
  return (
    <MessageItem
      message={message}
      highlightDialogue={highlightDialogue}
      messageStyle={messageStyle}
      onEdit={onEditMessage ? () => onEditMessage(view) : undefined}
      onRegenerate={onRegenerateMessage ? () => onRegenerateMessage(view) : undefined}
      onSwitchVersion={onSwitchMessageVersion ? (_message, direction) => onSwitchMessageVersion(view, direction) : undefined}
      onOpenSubAgentSession={onOpenSubAgentSession ? () => onOpenSubAgentSession(view) : undefined}
      onInsertIllustration={onInsertIllustration}
      onGenerateInteractiveImage={onGenerateInteractiveImage ? () => onGenerateInteractiveImage(view) : undefined}
      generatingInteractiveImageTurnId={generatingInteractiveImageTurnId}
      activeSubAgentSessionKey={activeSubAgentSessionKey}
      subAgentPresentation={subAgentPresentation}
      onSubmitPlanQuestion={onSubmitPlanQuestion ? (_message, content, preview) => onSubmitPlanQuestion(view.ref, content, preview) : undefined}
      onApprovePlan={onApprovePlan ? () => onApprovePlan(view.ref) : undefined}
      onContinuePlan={onContinuePlan ? () => onContinuePlan(view) : undefined}
      onExitPlanMode={onExitPlanMode}
      onOpenTrace={onOpenTrace}
      onPlanCardLayoutChange={onPlanCardLayoutChange}
    />
  )
})
