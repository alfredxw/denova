import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Plus } from 'lucide-react'
import { motion } from 'motion/react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { createStablePortalHost, StablePortalSlot } from '@/components/layout/stable-portal-slot'
import type { ImagePreset, Teller } from '@/features/interactive/types'
import { DEFAULT_NARRATIVE_STYLE_ID, resolveNarrativeStyle } from '@/features/interactive/narrative-style'
import { novaEase } from '@/features/motion/motion-tokens'
import { answerSessionAsk, cancelSessionAsk, removeChatContextCompaction } from '@/lib/api'
import type {
  ActiveChatTask,
  AgentAskAnswer,
  AgentAskResolution,
  AgentRuntimeQueuedCommand,
  ChapterIllustration,
  ChapterSummary,
  ContextAnalysis,
  IDEContext,
  SessionSummary,
  TextSelection,
} from '@/lib/api'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { localizeAgentRuntimeReason } from '@/lib/agent-runtime-error'
import {
  agentSubAgentSessionKey,
  agentViewAskID,
  agentViewContent,
  buildAgentMessageViews,
  selectAgentTokenUsageRecords,
  type AgentMessageView,
  type AgentPartRef,
} from '@/lib/agent-message-view'
import { useSkillCommands } from '@/hooks/useSkillCommands'
import { DEFAULT_WRITING_SKILL, resolveWritingSkillSelection, useWritingSkillOptions } from '@/hooks/useWritingSkillOptions'
import type { PersistedUserSettingsController } from '@/hooks/usePersistedUserSettings'
import { AgentChatPane } from './AgentChatPane'
import { LoadingState } from '@/components/common/LoadingState'
import { SessionHistoryPopover } from './SessionHistoryPopover'
import { SessionManagementPanel } from './SessionManagementPanel'
import { SessionRailToggle } from './SessionRailToggle'
import { AgentSubAgentSessionPanel, type AgentSubAgentSessionTarget } from './AgentSubAgentSessionPanel'
import { CONTEXT_ANALYSIS_SIMULATED_MESSAGE, ContextAnalysisDialog } from './ContextAnalysisDialog'
import type { ReferencePickerItem } from './FileReferencePicker'
import { WritingComposerSettingsMenu, WritingImagePresetMenu } from './WritingComposerSettingsMenu'
import { ImageGenerationSettingsMenu } from './ImageGenerationSettingsMenu'
import { formatPlanDiscussionMessage } from '@/lib/plan-mode'
import { useProjectChangeGroups } from '@/features/changes/use-change-review'
import { AgentChangeSummaryCard } from '@/features/changes/agent/AgentChangeSummaryCard'
import {
  MAX_REVIEW_FEEDBACK_COMMENT_COUNT,
  MAX_REVIEW_FEEDBACK_CONTEXT_BYTES,
  reviewFeedbackCommentCount,
  reviewFeedbackContextBytes,
  type ReviewFeedbackBatch,
  type ReviewFeedbackComment,
  type ReviewFeedbackSelection,
} from '@/features/changes/agent/ReviewFeedbackTray'
import { toast } from 'sonner'
import type { ChatSendOptions } from '@/hooks/useAgentChat'
import type { InputAreaSendOptions } from './InputArea'
import { resolveAgentAskAndRefresh } from '@/lib/agent-ask'
import type { ConversationConfigBinding } from '@/features/conversation-config/types'
import { useConversationGoal } from '@/features/agent-goal/use-conversation-goal'
import { useConversationConfig } from '@/features/conversation-config/use-conversation-config'
import { CustomAgentSelect } from '@/features/agents/CustomAgentSelect'
import { useAgentQuickPromptControls } from '@/features/agent-quick-prompts/use-agent-quick-prompt-controls'
import type { AgentQuickPromptScope } from '@/features/agent-quick-prompts/defaults'

export type AgentPanelView = 'chat' | 'sessions'
export type AgentPanelChrome = 'panel' | 'workbench'

const WRITING_AGENT_INIT_EVENT = 'nova:writing-agent-init'
export const WRITING_COMPOSER_SETTING_DEFAULTS = {
  ide_story_teller_id: DEFAULT_NARRATIVE_STYLE_ID,
  interactive_story_teller_id: DEFAULT_NARRATIVE_STYLE_ID,
  ide_image_preset_id: 'game-cg',
  writing_skill_default: DEFAULT_WRITING_SKILL,
} as const

export type WritingComposerSettingsController = PersistedUserSettingsController<typeof WRITING_COMPOSER_SETTING_DEFAULTS>

export interface AgentPanelProps {
  /** Stable identity for every project-owned API and cache key. */
  projectId: string
  workspace: string
  /** Selects project-neutral controls and the General Agent configuration surface. */
  agentKind?: 'writing' | 'general'
  /** Hidden AgentChat tabs remain mounted for parallel streams but ignore global UI intents. */
  active?: boolean
  /**
   * Frame around the panel. `panel` is the docked IDE sidebar; `workbench` embeds the same
   * conversation as a full-width surface (AgentChat tab), where the host owns closing.
   */
  chrome?: AgentPanelChrome
  /** A dock host may keep the same panel section selected across mounted conversations. */
  view?: AgentPanelView
  onViewChange?: (view: AgentPanelView) => void
  /** Project-scoped conversations may be managed while the displayed conversation is running. */
  sessionActionsDisabled?: boolean
  sessionRailVisible?: boolean
  onSessionRailVisibleChange?: (visible: boolean) => void
  /** Keeps first-load history hidden until the virtualized list can mount at its final position. */
  initializing?: boolean
  /** Selects the page-owned prompt starters shown in an empty docked conversation. */
  quickPromptScope?: AgentQuickPromptScope
  /** Keeps unsent configuration drafts isolated while the shared conversation changes resources. */
  composerDraftScope?: string
  /** Owned above the conditional panel so closing the panel cannot discard delayed saves. */
  composerSettings: WritingComposerSettingsController
  currentChapter?: ChapterSummary
  selectedFile: string | null
  tellers: Teller[]
  imagePresets?: ImagePreset[]
  messages: AgentUIMessage[]
  sessions: SessionSummary[]
  activeSessionId: string
  /** The composer may not have messages yet; its conversation configuration is already durable. */
  sessionDraft?: boolean
  /** AgentChat supplies its project-bound identity; Writing derives it from activeSessionId. */
  conversationBinding?: ConversationConfigBinding
  isStreaming: boolean
  /** Session mutations are server-confirmed before the visible binding changes. */
  sessionTransitionPending?: boolean
  /** Real execution state, excluding an idle startup/recovery inspection. */
  isExecutionActive: boolean
  runtimeProjection?: ActiveChatTask | null
  abortPending?: boolean
  commandSubmitting?: boolean
  queueActionPendingCommandID?: string
  activityContent: string
  references: string[]
  loreReferences: string[]
  loreReferenceLabels: Record<string, string>
  loreSuggestions: ReferencePickerItem[]
  styleScenes: string[]
  textSelections: TextSelection[]
  ideContext?: IDEContext
  planMode: boolean
  hasEarlierMessages: boolean
  isLoadingEarlierHistory: boolean
  fileSuggestions: string[]
  onCreateSession: (title?: string, customAgentId?: string) => void | Promise<void>
  onSwitchSession: (id: string) => void | Promise<void>
  onRenameSession: (id: string, title: string) => void | Promise<void>
  onDeleteSession: (id: string) => void | Promise<void>
  onLoadEarlierHistory: () => void | Promise<void>
  onRefreshHistory: (sessionId?: string) => void | Promise<void>
  /** Scoped AgentChat tabs override interaction endpoints so Writing state is never touched. */
  onAnswerAsk?: (sessionId: string, askId: string, answers: AgentAskAnswer[]) => Promise<AgentAskResolution>
  onCancelAsk?: (sessionId: string, askId: string) => Promise<AgentAskResolution>
  onRemoveContextCompaction?: () => Promise<boolean>
  onSend: (message: string, options?: ChatSendOptions) => boolean | Promise<boolean>
  onAnalyzeContext: (
    message: string,
    options?: {
      writingSkill?: string
      ideContext?: IDEContext
      imagePresetId?: string
      tellerId?: string
    },
  ) => Promise<ContextAnalysis>
  onStop: () => void
  onSteerQueuedCommand?: (item: AgentRuntimeQueuedCommand) => boolean | Promise<boolean>
  onDeleteQueuedCommand?: (item: AgentRuntimeQueuedCommand) => boolean | Promise<boolean>
  onEditQueuedCommand?: (item: AgentRuntimeQueuedCommand) => string | null | Promise<string | null>
  onReferenceRemove: (path: string) => void
  onLoreReferenceAdd: (id: string) => void
  onLoreReferenceRemove: (id: string) => void
  onStyleSceneAdd: (scene: string) => void
  onStyleSceneRemove: (scene: string) => void
  onTextSelectionRemove: (index: number) => void
  onInsertIllustration?: (illustration: ChapterIllustration) => void
  onPlanModeChange: (value: boolean) => void
  onPlanModeToggle: () => void
  onApproveProposedPlan: (ref: AgentPartRef) => void
  onExitPlanMode: () => void
  reviewFeedback?: ReviewFeedbackBatch | null
  onReviewFeedbackOpen?: (selection: ReviewFeedbackSelection, comment: ReviewFeedbackComment) => void
  onReviewFeedbackRemove?: (selection: ReviewFeedbackSelection, commentID: string) => void
  onReviewFeedbackSubmitted?: (feedback: ReviewFeedbackBatch) => void
  onReviewFeedbackSubmissionFailed?: (feedback: ReviewFeedbackBatch) => void
  onOpenChangeReview?: (reviewThreadID: string, groupID: string) => void
  onWorkspaceChanged?: (paths: string[]) => void | Promise<void>
  /** Hosts with a tab model own sub-Agent navigation; standalone surfaces use an inline fallback. */
  activeSubAgentSession?: AgentSubAgentSessionTarget | null
  onSubAgentSessionOpen?: (target: AgentSubAgentSessionTarget) => void | Promise<void>
}

/**
 * The writing Agent surface, switchable between conversation, session management and traces.
 * It is docked on the right of the writing workbench and embedded as a tab in AgentChat;
 * `chrome` selects which frame it renders with.
 */
function AgentPanelComponent({
  projectId,
  workspace,
  agentKind = 'writing',
  active = true,
  chrome = 'panel',
  view: controlledView,
  onViewChange,
  sessionActionsDisabled,
  sessionRailVisible = true,
  onSessionRailVisibleChange,
  initializing = false,
  quickPromptScope: configuredQuickPromptScope,
  composerDraftScope,
  composerSettings: persistedSettings,
  tellers,
  imagePresets = [],
  messages,
  sessions,
  activeSessionId,
  sessionDraft = false,
  conversationBinding,
  isStreaming,
  sessionTransitionPending = false,
  isExecutionActive,
  runtimeProjection = null,
  abortPending = false,
  commandSubmitting = false,
  queueActionPendingCommandID = '',
  activityContent,
  references,
  loreReferences,
  loreReferenceLabels,
  loreSuggestions,
  styleScenes,
  textSelections,
  ideContext,
  planMode,
  hasEarlierMessages,
  isLoadingEarlierHistory,
  fileSuggestions,
  onCreateSession,
  onSwitchSession,
  onRenameSession,
  onDeleteSession,
  onLoadEarlierHistory,
  onRefreshHistory,
  onAnswerAsk = answerSessionAsk,
  onCancelAsk = cancelSessionAsk,
  onRemoveContextCompaction = removeChatContextCompaction,
  onSend,
  onAnalyzeContext,
  onStop,
  onSteerQueuedCommand,
  onDeleteQueuedCommand,
  onEditQueuedCommand,
  onReferenceRemove,
  onLoreReferenceAdd,
  onLoreReferenceRemove,
  onStyleSceneAdd,
  onStyleSceneRemove,
  onTextSelectionRemove,
  onInsertIllustration,
  onPlanModeChange,
  onPlanModeToggle,
  onApproveProposedPlan,
  onExitPlanMode,
  reviewFeedback,
  onReviewFeedbackOpen,
  onReviewFeedbackRemove,
  onReviewFeedbackSubmitted,
  onReviewFeedbackSubmissionFailed,
  onOpenChangeReview,
  onWorkspaceChanged,
  activeSubAgentSession = null,
  onSubAgentSessionOpen,
}: AgentPanelProps) {
  const { t } = useTranslation()
  const dockedChrome = chrome === 'panel'
  const generalAgent = agentKind === 'general'
  const [internalView, setInternalView] = useState<AgentPanelView>('chat')
  const view = controlledView ?? internalView
  const setView = useCallback((nextView: AgentPanelView) => {
    if (controlledView === undefined) setInternalView(nextView)
    onViewChange?.(nextView)
  }, [controlledView, onViewChange])
  const [inputPrefill, setInputPrefill] = useState<{
    prompt: string
    nonce: number
    mode?: 'replace' | 'append'
  } | null>(null)
  const [contextAnalysisOpen, setContextAnalysisOpen] = useState(false)
  const [contextAnalysisLoading, setContextAnalysisLoading] = useState(false)
  const [contextAnalysisError, setContextAnalysisError] = useState<string | null>(null)
  const [contextAnalysis, setContextAnalysis] = useState<ContextAnalysis | null>(null)
  const [fallbackSubAgentSession, setFallbackSubAgentSession] = useState<AgentSubAgentSessionTarget | null>(null)
  const [inputAreaHeight, setInputAreaHeight] = useState(0)
  const pendingWritingInitRef = useRef<string | null>(null)
  const recoveryPaused = Boolean(runtimeProjection?.recovery_paused)
  const runtimeRecovering = Boolean(runtimeProjection?.runtime_recoverable && (!runtimeProjection.stream_attached || recoveryPaused))
  const recoveryAbortAvailable = Boolean(runtimeProjection?.recovery_actions?.some((action) => action.kind === 'abort'))
  const activeControlsDisabled =
    isStreaming && (!runtimeProjection?.active_operation_id?.trim() || Boolean(runtimeProjection?.runtime_recoverable && !runtimeProjection.stream_attached))
  const sessionControlsDisabled = sessionActionsDisabled ?? (isStreaming || sessionTransitionPending)
  const [chatPaneHost] = useState(() => createStablePortalHost('relative flex h-full min-h-0 w-full min-w-0 flex-col'))
  const ideTellerId = persistedSettings.values.ide_story_teller_id
  const imagePresetId = persistedSettings.values.ide_image_preset_id
  const configuredWritingSkill = persistedSettings.values.writing_skill_default
  const skillCatalogEnabled = Boolean(projectId.trim())
  const skillCommands = useSkillCommands({
    agentKey: generalAgent ? 'general' : 'ide',
    projectId,
    enabled: skillCatalogEnabled,
  })
  const writingSkillOptions = useWritingSkillOptions(projectId, skillCatalogEnabled)
  const writingSkill = useMemo(() => resolveWritingSkillSelection(configuredWritingSkill, writingSkillOptions), [configuredWritingSkill, writingSkillOptions])
  const changeGroupsQuery = useProjectChangeGroups(active && projectId && activeSessionId && !sessionDraft ? projectId : '', { sessionID: activeSessionId })
  const tokenUsageMessages = useMemo(() => selectAgentTokenUsageRecords(messages), [messages])
  const effectiveConversationBinding = useMemo<ConversationConfigBinding | undefined>(() => conversationBinding ?? (activeSessionId
    ? { mode: generalAgent ? 'agent_chat' : 'writing', project_id: projectId, session_id: activeSessionId }
    : undefined), [activeSessionId, conversationBinding, generalAgent, projectId])
  const agentSelectionConfig = useConversationConfig(effectiveConversationBinding)
  const conversationGoal = useConversationGoal(effectiveConversationBinding, isExecutionActive)
  const activeRunID = useMemo(() => {
    if (!isExecutionActive) return ''
    const runtimeRunID = runtimeProjection?.active_operation_id?.trim()
    if (runtimeRunID) return runtimeRunID
    const views = buildAgentMessageViews(messages)
    for (let index = views.length - 1; index >= 0; index -= 1) {
      if (!views[index].metadata.subagent && views[index].metadata.run_id) return views[index].metadata.run_id || ''
    }
    return ''
  }, [isExecutionActive, messages, runtimeProjection?.active_operation_id])
  const lastRuntimeFailure = !runtimeProjection?.active && runtimeProjection?.last_operation?.status === 'failed'
    ? localizeAgentRuntimeReason(runtimeProjection.last_operation.reason, '', t)
    : ''
  const messageListBottomPadding = inputAreaHeight > 0 ? inputAreaHeight + 20 : undefined
  const styleSceneSuggestions = useMemo(() => {
    const teller = resolveNarrativeStyle(tellers, ideTellerId)
    return Array.from(new Set((teller?.style_rules || []).map((rule) => rule.scene.trim()).filter((scene) => scene && !isGlobalStyleSceneName(scene))))
  }, [ideTellerId, tellers])

  useEffect(() => {
    if (generalAgent) return
    if (!active) return
    const handleWritingInitRequest = (event: Event) => {
      const detail = (event as CustomEvent<{ prompt?: string; autoSend?: boolean }>).detail
      const prompt = detail?.prompt || t('writingAgent.initPrompt')
      setView('chat')
      if (detail?.autoSend && !isStreaming && !persistedSettings.loading) {
        onSend(prompt, {
          writingSkill,
          ideContext,
          imagePresetId,
          tellerId: ideTellerId,
        })
        return
      }
      if (detail?.autoSend && !isStreaming) {
        pendingWritingInitRef.current = prompt
        return
      }
      setInputPrefill((current) => ({
        prompt,
        nonce: (current?.nonce || 0) + 1,
      }))
    }
    window.addEventListener(WRITING_AGENT_INIT_EVENT, handleWritingInitRequest)
    return () => window.removeEventListener(WRITING_AGENT_INIT_EVENT, handleWritingInitRequest)
  }, [active, generalAgent, ideContext, ideTellerId, imagePresetId, isStreaming, onSend, persistedSettings.loading, t, writingSkill])

  useEffect(() => {
    if (generalAgent) return
    if (persistedSettings.loading || isStreaming || !pendingWritingInitRef.current) return
    const prompt = pendingWritingInitRef.current
    pendingWritingInitRef.current = null
    onSend(prompt, {
      writingSkill,
      ideContext,
      imagePresetId,
      tellerId: ideTellerId,
    })
  }, [generalAgent, ideContext, ideTellerId, imagePresetId, isStreaming, onSend, persistedSettings.loading, writingSkill])

  useEffect(() => {
    pendingWritingInitRef.current = null
  }, [workspace])

  useEffect(() => {
    setFallbackSubAgentSession(null)
  }, [activeSessionId])

  const handleAnalyzeContext = async (message: string) => {
    setContextAnalysisLoading(true)
    setContextAnalysisError(null)
    setContextAnalysis(null)
    try {
      setContextAnalysis(
        await onAnalyzeContext(
          message,
          generalAgent
            ? undefined
            : {
                writingSkill,
                ideContext,
                imagePresetId,
                tellerId: ideTellerId,
              },
        ),
      )
    } catch (e) {
      setContextAnalysis(null)
      setContextAnalysisError((e as Error).message)
    } finally {
      setContextAnalysisLoading(false)
    }
  }

  const openContextAnalysis = () => {
    setContextAnalysisOpen(true)
    void handleAnalyzeContext(CONTEXT_ANALYSIS_SIMULATED_MESSAGE)
  }

  const openSubAgentSession = useCallback((message: AgentMessageView) => {
    const key = agentSubAgentSessionKey(message)
    if (!key || !activeSessionId) return
    const target: AgentSubAgentSessionTarget = {
      parentSessionId: activeSessionId,
      sessionKey: key,
      name: message.metadata.agent_name || message.metadata.subagent_type || t('chat.subagent.label'),
    }
    if (onSubAgentSessionOpen) {
      void onSubAgentSessionOpen(target)
      return
    }
    setFallbackSubAgentSession(target)
  }, [activeSessionId, onSubAgentSessionOpen, t])

  const continuePlanDiscussion = useCallback((message: AgentMessageView) => {
    setView('chat')
    onPlanModeChange(true)
    setInputPrefill((current) => ({
      prompt: formatPlanDiscussionMessage(agentViewContent(message)),
      nonce: (current?.nonce || 0) + 1,
    }))
  }, [onPlanModeChange])

  const removeContextCompaction = async () => {
    await onRemoveContextCompaction()
    await handleAnalyzeContext(CONTEXT_ANALYSIS_SIMULATED_MESSAGE)
  }

  const timelineAttachments = useMemo(
    () =>
      (changeGroupsQuery.data ?? [])
        // A change group is mutable until its owning Run reaches a terminal
        // state. Mounting it earlier makes an incomplete review look ready and
        // shifts the live timeline while the Agent is still producing output.
        .filter((summary) => Boolean(summary.run_id) && summary.run_id !== activeRunID)
        .map((summary, index) => ({
          id: summary.id,
          runId: summary.run_id || '',
          content: (
            <AgentChangeSummaryCard
              projectId={projectId}
              summary={summary}
              disabled={isExecutionActive}
              eagerPreload={!isExecutionActive && index === 0}
              onReview={(reviewThreadID, groupID) => onOpenChangeReview?.(reviewThreadID, groupID)}
              onWorkspaceChanged={onWorkspaceChanged}
            />
          ),
        })),
    [activeRunID, changeGroupsQuery.data, isExecutionActive, onOpenChangeReview, onWorkspaceChanged, projectId],
  )

  const sendWithWritingSkill = async (message: string, inputOptions?: InputAreaSendOptions) => {
    if (persistedSettings.loading) return false
    const feedbackSelection = reviewFeedback?.filter((selection) => selection.comments.length) ?? []
    const feedback = feedbackSelection.length
      ? feedbackSelection.map((selection) => ({
          source: selection.source || ('workspace_change' as const),
          reviewThreadId: selection.reviewThreadId,
          commentIds: selection.comments.map((comment) => comment.id),
        }))
      : undefined
    const feedbackCount = reviewFeedbackCommentCount(feedbackSelection)
    const effectiveMessage = message.trim() || (feedback ? t('changes.feedback.defaultMessage', { count: feedbackCount }) : message)
    if (feedbackCount > MAX_REVIEW_FEEDBACK_COMMENT_COUNT) {
      toast.error(
        t('changes.feedback.tooMany', {
          maximum: MAX_REVIEW_FEEDBACK_COMMENT_COUNT,
        }),
      )
      return false
    }
    if (feedbackSelection.length && reviewFeedbackContextBytes(feedbackSelection) > MAX_REVIEW_FEEDBACK_CONTEXT_BYTES) {
      toast.error(t('changes.feedback.tooLarge'))
      return false
    }
    let submissionStarted = false
    let submissionRestored = false
    const handleSubmissionStart = () => {
      if (!feedbackSelection.length || submissionStarted) return
      submissionStarted = true
      onReviewFeedbackSubmitted?.(feedbackSelection)
    }
    const handleSubmissionError = () => {
      if (!feedbackSelection.length || !submissionStarted || submissionRestored) return
      submissionRestored = true
      onReviewFeedbackSubmissionFailed?.(feedbackSelection)
    }
    const accepted = await onSend(effectiveMessage, {
      attachments: inputOptions?.attachments,
      ...(generalAgent ? {} : { writingSkill, ideContext, imagePresetId, tellerId: ideTellerId }),
      reviewFeedback: feedback,
      reviewFeedbackDisplay: feedbackSelection.length
        ? {
            comments: feedbackSelection.flatMap((selection) => selection.comments),
          }
        : undefined,
      loreReferenceLabels,
      onSubmissionStart: handleSubmissionStart,
      onSubmissionError: handleSubmissionError,
    })
    if (feedbackSelection.length && accepted && !submissionStarted) handleSubmissionStart()
    if (!accepted) handleSubmissionError()
    return accepted
  }

  const submitGoal = async (objective: string, inputOptions?: InputAreaSendOptions) => {
    if (planMode) onPlanModeChange(false)
    const next = await conversationGoal.set(objective)
    if (!next) {
      toast.error(t('chat.goal.updateFailed'))
      return false
    }
    return sendWithWritingSkill(objective, inputOptions)
  }

  const pauseGoal = async () => {
    const next = await conversationGoal.pause()
    if (!next) {
      toast.error(t('chat.goal.updateFailed'))
      return
    }
    if (isExecutionActive) onStop()
  }

  const clearGoal = async () => {
    const next = await conversationGoal.clear()
    if (!next) {
      toast.error(t('chat.goal.updateFailed'))
      return
    }
    if (isExecutionActive) onStop()
  }

  const returnQueuedCommandToEditor = useCallback(async (item: AgentRuntimeQueuedCommand) => {
    const prompt = await onEditQueuedCommand?.(item)
    if (typeof prompt !== 'string') return
    setInputPrefill((current) => ({
      prompt,
      nonce: (current?.nonce || 0) + 1,
    }))
  }, [onEditQueuedCommand])

  const quickPromptScope = configuredQuickPromptScope ?? (generalAgent ? undefined : 'writing')
  const fillQuickPrompt = useCallback((prompt: string) => {
    setInputPrefill((current) => ({
      prompt,
      nonce: (current?.nonce || 0) + 1,
      mode: 'append',
    }))
  }, [])
  const quickPrompts = useAgentQuickPromptControls({
    scope: quickPromptScope,
    disabled: persistedSettings.loading,
    onFill: fillQuickPrompt,
    onSend: sendWithWritingSkill,
  })
  // Prompt starters belong to the docked page surface. The full AgentChat workbench opens on a
  // clean conversation because its project tabs are not tied to one page task.
  const emptyChatContent =
    dockedChrome && messages.length === 0 && !isStreaming ? quickPrompts.cards : null
  const resolveAsk = useCallback(
    async (view: AgentMessageView, action: { status: 'answered'; answers: AgentAskAnswer[] } | { status: 'cancelled' }) => {
      const askID = agentViewAskID(view)
      if (!activeSessionId || !askID) throw new Error('Cannot resolve an Ask without its Session and interaction IDs')
      return resolveAgentAskAndRefresh(
        action,
        {
          answer: (answers) => onAnswerAsk(activeSessionId, askID, answers),
          cancel: () => onCancelAsk(activeSessionId, askID),
        },
        () => onRefreshHistory(activeSessionId),
      )
    },
    [activeSessionId, onAnswerAsk, onCancelAsk, onRefreshHistory],
  )
  const messageListProps = {
    projectId,
    attachmentScope: activeSessionId ? { kind: 'session' as const, id: activeSessionId } : undefined,
    messages,
    isStreaming,
    visible: active,
    isExecutionActive,
    activityContent: runtimeRecovering ? t('chat.activity.recovering') : recoveryPaused ? t('chat.activity.recoveryPaused') : activityContent,
    scrollResetKey: `${workspace || 'none'}:${activeSessionId || 'current'}`,
    bottomPaddingClassName: 'pb-36',
    bottomPaddingPx: messageListBottomPadding,
    collapseTraceGroups: true,
    activeTraceDisplay: 'expanded' as const,
    hasEarlierMessages,
    isLoadingEarlierMessages: isLoadingEarlierHistory,
    onLoadEarlierMessages: onLoadEarlierHistory,
    timelineAttachments,
    onOpenSubAgentSession: openSubAgentSession,
    onInsertIllustration,
    activeSubAgentSessionKey: activeSubAgentSession?.parentSessionId === activeSessionId
      ? activeSubAgentSession.sessionKey
      : fallbackSubAgentSession?.sessionKey || '',
    onApprovePlan: onApproveProposedPlan,
    onContinuePlan: continuePlanDiscussion,
    onExitPlanMode,
    onResolveAsk: resolveAsk,
    activeRunId: runtimeProjection?.active_operation_id,
    afterContent: lastRuntimeFailure ? (
      <div
        role="alert"
        className="whitespace-pre-wrap break-words rounded-lg border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-xs leading-relaxed text-[var(--nova-danger)]"
      >
        {t('chat.activity.requestFailed', { error: lastRuntimeFailure })}
      </div>
    ) : undefined,
    afterContentKey: lastRuntimeFailure
      ? `runtime-failure:${runtimeProjection?.last_operation?.operation_id || lastRuntimeFailure}`
      : undefined,
  }
  const inputAreaProps = {
    onSend: sendWithWritingSkill,
    attachmentsEnabled: true,
    onStop,
    disabled: sessionTransitionPending,
    sendBlocked: persistedSettings.loading || sessionTransitionPending,
    generationActive: isStreaming,
    resumeAvailable: !isStreaming && Boolean(runtimeProjection?.pending_interruption_id?.trim()),
    queuedCommands: runtimeProjection?.queue || [],
    queueActionPendingCommandID,
    onQueuedCommandSteer: onSteerQueuedCommand,
    onQueuedCommandDelete: onDeleteQueuedCommand,
    onQueuedCommandEdit: returnQueuedCommandToEditor,
    abortPending,
    commandSubmitting,
    activeControlsDisabled,
    activeStopDisabled: activeControlsDisabled && !recoveryAbortAvailable,
    planMode,
    onTogglePlanMode: onPlanModeToggle,
    goal: conversationGoal.goal,
    goalPending: conversationGoal.saving,
    onGoalSubmit: submitGoal,
    onGoalPause: pauseGoal,
    onGoalClear: clearGoal,
    draftKey: `ide-agent:${workspace || 'global'}:${activeSessionId || 'current'}${composerDraftScope ? `:${composerDraftScope}` : ''}`,
    inputPrefill,
    onInputPrefillConsumed: () => setInputPrefill(null),
    referencedFiles: references,
    onReferenceRemove,
    fileSuggestions,
    loreReferences: generalAgent ? [] : loreReferences,
    loreReferenceLabels,
    onLoreReferenceAdd,
    onLoreReferenceRemove,
    loreSuggestions: generalAgent ? [] : loreSuggestions,
    styleScenes: generalAgent ? [] : styleScenes,
    onStyleSceneAdd,
    onStyleSceneRemove,
    styleSceneSuggestions: generalAgent ? [] : styleSceneSuggestions,
    textSelections,
    onTextSelectionRemove,
    reviewFeedback,
    onReviewFeedbackOpen,
    onReviewFeedbackRemove,
    skills: skillCommands,
    quickPrompts: quickPrompts.commands,
    onContextAnalyze: sessionDraft ? undefined : openContextAnalysis,
    tokenUsageMessages,
    agentKey: generalAgent ? ('general' as const) : ('ide' as const),
    workspace,
    conversationBinding: effectiveConversationBinding,
    composerSettingsControl: (
      <>
        {quickPrompts.menuItem}
        {!generalAgent ? (
          <>
            <ImageGenerationSettingsMenu projectId={projectId} disabled={!workspace || persistedSettings.loading || isStreaming}>
              <WritingImagePresetMenu
                enabled={Boolean(workspace) && !persistedSettings.loading && !isStreaming}
                imagePresets={imagePresets}
                imagePresetID={imagePresetId}
                saving={persistedSettings.isSaving('ide_image_preset_id')}
                onChange={(value) => persistedSettings.persist('ide_image_preset_id', value)}
              />
            </ImageGenerationSettingsMenu>
            <WritingComposerSettingsMenu
              enabled={Boolean(workspace) && !persistedSettings.loading}
              tellers={tellers}
              tellerID={ideTellerId}
              writingSkills={writingSkillOptions}
              writingSkill={writingSkill}
              savingTeller={persistedSettings.isSaving('ide_story_teller_id')}
              savingWritingSkill={persistedSettings.isSaving('writing_skill_default')}
              onTellerChange={(value) => persistedSettings.persist('ide_story_teller_id', value)}
              onWritingSkillChange={(value) => persistedSettings.persist('writing_skill_default', value)}
            />
          </>
        ) : null}
      </>
    ),
    onboardingAnchor: 'agent-input',
    floating: true,
    onHeightChange: setInputAreaHeight,
  }
  const chatPane = initializing ? (
    <LoadingState label={t('router.loading')} layout="conversation" className="h-full min-h-0" />
  ) : (
    <AgentChatPane
      className="min-w-0 flex-1"
      contentClassName={dockedChrome ? undefined : 'mx-auto w-full max-w-[56rem]'}
      sessionTransitionPending={sessionTransitionPending}
      emptyContent={emptyChatContent}
      messageListProps={messageListProps}
      inputAreaProps={inputAreaProps}
    />
  )
  const chatPanePortal = view === 'chat' && chatPaneHost ? createPortal(chatPane, chatPaneHost, 'agent-chat-pane') : null
  return (
    <aside
      className={`nova-sidebar relative flex h-full min-h-0 flex-col overflow-hidden ${
        dockedChrome ? 'border-l border-[var(--nova-border)] bg-[var(--nova-surface)] shadow-[-14px_0_30px_-28px_rgba(0,0,0,0.64)]' : 'bg-[var(--nova-bg)]'
      }`}
    >
      {/*
        AgentChat supplies its own tab strip, conversation tree and new-chat entry points, so
        the docked panel's header would only duplicate them. It is rendered for the writing
        workbench only.
      */}
      {dockedChrome && (
        <div className="flex h-9 shrink-0 items-center gap-1.5 border-b border-[var(--nova-border)] px-2">
          <div
            className="flex h-7 shrink-0 items-center rounded-[var(--nova-radius)] bg-[var(--nova-surface-2)] p-0.5"
            role="group"
            aria-label={t('chat.sessionControls')}
          >
            <button
              type="button"
              onClick={() => setView('chat')}
              className={`flex h-6 items-center rounded-[6px] px-2 text-[11px] transition-colors ${view === 'chat' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
            >
              {t('chat.view.chat')}
            </button>
            <span aria-hidden="true" className="mx-1 h-3.5 w-px bg-[var(--nova-border)]" />
            <SessionHistoryPopover
              sessions={sessions}
              activeSessionId={activeSessionId}
              active={view === 'sessions'}
              disabled={sessionControlsDisabled}
              onSwitch={onSwitchSession}
              onManage={() => setView('sessions')}
            />
            <button
              type="button"
              disabled={sessionControlsDisabled}
              onClick={() => void onCreateSession()}
              className="nova-nav-item flex size-6 shrink-0 items-center justify-center rounded-[6px] disabled:cursor-not-allowed disabled:opacity-45"
              aria-label={t('chat.newSession')}
            >
              <motion.span
                className="flex"
                animate={sessionTransitionPending ? { rotate: 90, scale: 0.78 } : { rotate: 0, scale: 1 }}
                transition={{ duration: sessionTransitionPending ? 0.1 : 0.16, ease: novaEase }}
              >
                <Plus className="size-3.5" />
              </motion.span>
            </button>
          </div>
          <CustomAgentSelect
            projectId={projectId}
            runtimeKind={generalAgent ? 'general' : 'ide'}
            value={agentSelectionConfig.snapshot?.custom_agent_id ?? ''}
            disabled={sessionControlsDisabled || agentSelectionConfig.loading}
            size="sm"
            className="min-w-0 max-w-48 border-transparent bg-[var(--nova-surface-2)] py-0 text-[11px] *:data-[slot=select-value]:block *:data-[slot=select-value]:truncate"
            onValueChange={(customAgentId) => {
              if (customAgentId === undefined) return
              if (customAgentId === (agentSelectionConfig.snapshot?.custom_agent_id ?? '')) return
              void Promise.resolve(onCreateSession(undefined, customAgentId)).catch((cause) => {
                toast.error(t('chat.sessionRail.createFailed'), { description: cause instanceof Error ? cause.message : String(cause) })
              })
            }}
          />
          <div className="min-w-0 flex-1" />
          {onSessionRailVisibleChange && !sessionRailVisible ? (
            <SessionRailToggle
              visible={sessionRailVisible}
              onVisibleChange={onSessionRailVisibleChange}
            />
          ) : null}
        </div>
      )}

      {view === 'chat' ? (
        <>
          <div className="relative flex min-h-0 flex-1">
            {!fallbackSubAgentSession ? (
              <StablePortalSlot host={chatPaneHost} fallback={chatPane} wrapFallback={false} className="relative flex min-h-0 min-w-0 flex-1 flex-col" />
            ) : (
              <div className="absolute inset-0 z-30">
                <AgentSubAgentSessionPanel
                  projectId={projectId}
                  messages={messages}
                  sessionKey={fallbackSubAgentSession.sessionKey}
                  onClose={() => setFallbackSubAgentSession(null)}
                  onResolveAsk={resolveAsk}
                />
              </div>
            )}
          </div>
          <ContextAnalysisDialog
            open={contextAnalysisOpen}
            loading={contextAnalysisLoading}
            error={contextAnalysisError}
            analysis={contextAnalysis}
            onOpenChange={setContextAnalysisOpen}
            onRemoveCompaction={removeContextCompaction}
          />
        </>
      ) : (
        <SessionManagementPanel
          sessions={sessions}
          activeSessionId={activeSessionId}
          disabled={sessionControlsDisabled}
          onCreate={onCreateSession}
          onSwitch={onSwitchSession}
          onRename={onRenameSession}
          onDelete={onDeleteSession}
          onEnterChat={() => setView('chat')}
        />
      )}
      {chatPanePortal}
      {quickPrompts.dialog}
    </aside>
  )
}

export const AgentPanel = memo(AgentPanelComponent)

function isGlobalStyleSceneName(scene: string) {
  const normalized = scene.trim().toLowerCase()
  return normalized === '全局' || normalized === 'global'
}
