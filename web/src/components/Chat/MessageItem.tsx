import { memo } from 'react'
import type { CSSProperties } from 'react'
import type { ChapterIllustration, ChatMessage } from '@/lib/api'
import { subAgentSessionKey } from './subagent-session'
import { Message as AIMessage, MessageContent as AIMessageContent } from '@/components/ai-elements/message'
import { Shimmer } from '@/components/ai-elements/shimmer'
import { StreamingContentStage } from './StreamingContentStage'
import { AskInteractionCard, type AskInteractionResolver } from './AskInteractionCard'
import { ToolApprovalCard } from './ToolApprovalCard'
import {
  MarkdownContent,
  PlainTextContent,
  StreamingPlaceholder,
  ThinkingBlock,
  isPlainAssistantText,
  sanitizeThinkTags,
} from './message-content'
import { ContextCompactionBlock, ProposedPlanBlock } from './message-plan'
import { ChapterIllustrationBlock, InteractiveImageBlock, InteractiveImageStrip } from './message-media'
import {
  MessageInlineMeta,
  RuleRollBlock,
  SentMessageReferences,
  SubAgentOutputWindow,
  TraceLinkButton,
} from './message-metadata'
import { ToolExecutionBlock, ToolResultBlock } from './message-tool'
import { TodoListBlock } from './message-todo'

interface MessageItemProps {
  projectId?: string
  message: ChatMessage
  highlightDialogue?: boolean
  messageStyle?: CSSProperties
  onEdit?: (message: ChatMessage) => void
  onEditAssistantReply?: (message: ChatMessage) => void
  onCreateBranch?: (message: ChatMessage) => void
  onRegenerate?: (message: ChatMessage) => void
  onSwitchVersion?: (message: ChatMessage, direction: -1 | 1) => void
  onOpenSubAgentSession?: (message: ChatMessage) => void
  onInsertIllustration?: (illustration: ChapterIllustration) => void
  onGenerateInteractiveImage?: (message: ChatMessage) => void
  generatingInteractiveImageTurnId?: string
  activeSubAgentSessionKey?: string
  subAgentPresentation?: 'card' | 'content'
  onApprovePlan?: (message: ChatMessage) => void
  onContinuePlan?: (message: ChatMessage) => void
  onExitPlanMode?: () => void
  onOpenTrace?: (runID: string) => void
  onInteractiveCardLayoutChange?: (element?: HTMLElement) => void
  onResolveAsk?: AskInteractionResolver
}

/** Dispatches one role-specific message to its focused renderer. */
export const MessageItem = memo(function MessageItem({ projectId = '', message, highlightDialogue = false, messageStyle, onEdit, onEditAssistantReply, onCreateBranch, onRegenerate, onSwitchVersion, onOpenSubAgentSession, onInsertIllustration, onGenerateInteractiveImage, generatingInteractiveImageTurnId, activeSubAgentSessionKey, subAgentPresentation = 'card', onApprovePlan, onContinuePlan, onExitPlanMode, onOpenTrace, onInteractiveCardLayoutChange, onResolveAsk }: MessageItemProps) {
  const { role, content = '' } = message
  const canEdit = role === 'user' && Boolean(message.turn_id) && Boolean(onEdit)
  const canEditAssistantReply = role === 'assistant' && !message.subagent && Boolean(message.turn_id) && Boolean(onEditAssistantReply) && !message.streaming
  const canCreateBranch = role === 'assistant' && !message.subagent && Boolean(message.turn_id) && Boolean(onCreateBranch) && !message.streaming
  const canRegenerate = (role === 'assistant' || role === 'error') && Boolean(onRegenerate) && !message.streaming
  const canGenerateInteractiveImage = role === 'assistant' && Boolean(message.turn_id) && Boolean(onGenerateInteractiveImage) && !message.streaming
  const versionCount = message.turn_versions?.length || 0
  const markedVersionIndex = message.turn_versions?.findIndex((version) => version.current) ?? -1
  const versionIndex = message.turn_version_index ?? markedVersionIndex
  const canSwitchVersion = role === 'assistant' && versionCount > 1 && versionIndex >= 0 && Boolean(onSwitchVersion) && !message.streaming

  switch (role) {
    case 'user':
      return (
        <AIMessage from="user" className="max-w-none items-end">
          <div className="nova-message-body-with-meta nova-message-body-with-meta-user max-w-[88%]">
            <AIMessageContent className="nova-user-message rounded-lg bg-[var(--nova-user-message-bg-to)] px-3 py-2 text-sm leading-5 text-[var(--nova-user-message-text)] whitespace-pre-wrap group-[.is-user]:px-3 group-[.is-user]:py-2" style={messageStyle}>
              <SentMessageReferences references={message.user_references} />
              <span>{content}</span>
            </AIMessageContent>
            <MessageInlineMeta message={message} content={content} align="right" reserveSpace={Boolean(onEdit)} onEdit={canEdit ? onEdit : undefined} />
          </div>
        </AIMessage>
      )

    case 'assistant': {
      if (message.subagent && subAgentPresentation === 'card') {
        return (
          <SubAgentOutputWindow
            message={message}
            content={content}
            highlightDialogue={highlightDialogue}
            messageStyle={messageStyle}
            projectId={projectId}
            onOpen={onOpenSubAgentSession}
            active={Boolean(activeSubAgentSessionKey && activeSubAgentSessionKey === subAgentSessionKey(message))}
          />
        )
      }
      // Streaming content may not have arrived yet, or may contain only hidden
      // reasoning. Show activity instead of an empty bubble in either case.
      const streamingTargetContent = message.streaming === true && message.streaming_target_content && message.streaming_target_content !== content
        ? message.streaming_target_content
        : undefined
      const visibleContent = sanitizeThinkTags(streamingTargetContent || content).trim()
      const reserveMetaSpace = message.streaming === true || Boolean(canEditAssistantReply || canCreateBranch || onGenerateInteractiveImage || onRegenerate || onSwitchVersion)
      return (
        <AIMessage from="assistant" className="max-w-none">
          <div className="w-full">
            <div className="nova-message-body-with-meta nova-message-body-with-meta-assistant">
              <AIMessageContent className="chat-agent-message block w-full gap-0 px-1 text-sm text-[var(--nova-text)]" style={messageStyle}>
                {/* Streaming keeps one cheap text node. Plain completed text keeps that node mounted;
                    only completed Markdown pays the parser cost. */}
                {message.streaming && !visibleContent ? (
                  <StreamingPlaceholder />
                ) : (
                  <StreamingContentStage content={content} targetContent={streamingTargetContent} streaming={message.streaming === true}>
                    {(value) => message.streaming || isPlainAssistantText(value)
                      ? <PlainTextContent content={sanitizeThinkTags(value)} />
                      : <MarkdownContent content={value} highlightDialogue={highlightDialogue} projectId={projectId} />}
                  </StreamingContentStage>
                )}
              </AIMessageContent>
              <InteractiveImageStrip message={message} projectId={projectId} />
              <MessageInlineMeta
                message={message}
                content={content}
                align="left"
                reserveSpace={reserveMetaSpace}
                hideActions={message.streaming === true}
                onEdit={canEditAssistantReply ? onEditAssistantReply : undefined}
                editLabelKey="chat.action.editAssistantReply"
                onCreateBranch={canCreateBranch ? onCreateBranch : undefined}
                onGenerateInteractiveImage={canGenerateInteractiveImage ? onGenerateInteractiveImage : undefined}
                generatingInteractiveImage={Boolean(message.turn_id && generatingInteractiveImageTurnId === message.turn_id)}
                interactiveImageGenerationDisabled={Boolean(generatingInteractiveImageTurnId)}
                onRegenerate={canRegenerate ? onRegenerate : undefined}
                onSwitchVersion={canSwitchVersion ? onSwitchVersion : undefined}
                versionIndex={versionIndex}
                versionCount={versionCount}
              />
            </div>
          </div>
        </AIMessage>
      )
    }

    case 'thinking':
      return <ThinkingBlock message={message} content={content} streaming={message.streaming === true} />

    case 'tool_call':
      if ((message.name || '') === 'generate_interactive_image') {
        return <InteractiveImageBlock message={message} projectId={projectId} />
      }
      if (['generate_image', 'generate_chapter_illustration'].includes(message.name || '') && message.illustration) {
        return <ChapterIllustrationBlock message={message} projectId={projectId} onInsert={onInsertIllustration} />
      }
      if ((message.name || '') === 'todo') {
        return <TodoListBlock message={message} />
      }
      if ((message.name || '') === 'ask') {
        if (message.ask?.kind === 'tool_approval') return <ToolApprovalCard message={message} onResolve={onResolveAsk} />
        return <AskInteractionCard message={message} onResolve={onResolveAsk} />
      }
      return <ToolExecutionBlock message={message} onResolve={onResolveAsk} onLayoutChange={onInteractiveCardLayoutChange} />

    case 'ask':
      if (message.ask?.kind === 'tool_approval') return <ToolApprovalCard message={message} onResolve={onResolveAsk} />
      return <AskInteractionCard message={message} onResolve={onResolveAsk} />

    case 'rule_roll':
      return <RuleRollBlock message={message} />

    case 'tool_result':
      if ((message.name || '') === 'generate_interactive_image' || message.interactive_image) {
        return <InteractiveImageBlock message={message} projectId={projectId} />
      }
      if (message.illustration) {
        return <ChapterIllustrationBlock message={message} projectId={projectId} onInsert={onInsertIllustration} />
      }
      return <ToolResultBlock content={content} />

    case 'context_compaction':
      return <ContextCompactionBlock message={message} />

    // Usage records are summarized in TokenUsagePanel rather than the timeline.
    case 'token_usage':
      return null

    case 'proposed_plan':
      return <ProposedPlanBlock projectId={projectId} message={message} highlightDialogue={highlightDialogue} onApprove={onApprovePlan} onContinue={onContinuePlan} onExit={onExitPlanMode} onLayoutChange={onInteractiveCardLayoutChange} />

    case 'system':
      if (!content.trim()) return null
      return (
        <div className="flex justify-center">
          <span className="rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-1 text-xs text-[var(--nova-text-muted)]">
            {content}
          </span>
        </div>
      )

    case 'error':
      return (
        <div className="flex justify-center">
          <div className="nova-message-body-with-meta max-w-full">
            <div className="inline-flex max-w-full flex-wrap items-center justify-center gap-2 rounded-full border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-1 text-xs text-[var(--nova-danger)]">
              <span className="min-w-0 truncate">{content}</span>
              <TraceLinkButton runID={message.run_id} onOpenTrace={onOpenTrace} />
            </div>
            <MessageInlineMeta message={message} content={content} align="left" onRegenerate={canRegenerate ? onRegenerate : undefined} />
          </div>
        </div>
      )

    default: {
      const exhaustiveRole: never = role
      return exhaustiveRole
    }
  }
})

/** Lightweight activity shown before an active Agent emits a concrete message. */
export function AgentActivityShimmer({ content }: { content: string }) {
  return (
    <div className="flex justify-start px-1 py-1" role="status" aria-live="polite">
      <Shimmer as="span" className="text-sm font-medium">{content}</Shimmer>
    </div>
  )
}
