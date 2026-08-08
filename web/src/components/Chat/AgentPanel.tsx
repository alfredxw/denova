import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Activity, Bot, ChevronLeft, FileText, PenLine, Plus, SearchCheck, Sparkles, WandSparkles, X } from 'lucide-react'
import { Group, Panel, Separator } from 'react-resizable-panels'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { createStablePortalHost, StablePortalSlot } from '@/components/layout/stable-portal-slot'
import type { ImagePreset, Teller } from '@/features/interactive/types'
import { DEFAULT_NARRATIVE_STYLE_ID, resolveNarrativeStyle } from '@/features/interactive/narrative-style'
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
import { SessionManagementPanel } from './SessionManagementPanel'
import { AgentTracePanel } from './AgentTracePanel'
import { AgentSubAgentSessionPanel } from './AgentSubAgentSessionPanel'
import { CONTEXT_ANALYSIS_SIMULATED_MESSAGE, ContextAnalysisDialog } from './ContextAnalysisDialog'
import type { ReferencePickerItem } from './FileReferencePicker'
import { WritingComposerSettingsMenu } from './WritingComposerSettingsMenu'
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
import { resolveAgentAskAndRefresh } from '@/lib/agent-ask'
import type { ConversationConfigBinding } from '@/features/conversation-config/types'

type AgentPanelView = 'chat' | 'sessions' | 'traces'
export type AgentPanelChrome = 'panel' | 'workbench'

const WRITING_AGENT_INIT_EVENT = 'nova:writing-agent-init'
export const WRITING_COMPOSER_SETTING_DEFAULTS = {
  ide_story_teller_id: DEFAULT_NARRATIVE_STYLE_ID,
  interactive_story_teller_id: DEFAULT_NARRATIVE_STYLE_ID,
  ide_image_preset_id: 'game-cg',
  writing_skill_default: DEFAULT_WRITING_SKILL,
} as const

export type WritingComposerSettingsController = PersistedUserSettingsController<typeof WRITING_COMPOSER_SETTING_DEFAULTS>

interface AgentPanelProps {
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
  onCreateSession: (title?: string) => void | Promise<void>
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
  onClose: () => void
  onSubAgentDetailsChange?: (open: boolean) => void
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
  composerSettings: persistedSettings,
  currentChapter,
  selectedFile,
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
  onClose,
  onSubAgentDetailsChange,
}: AgentPanelProps) {
  const { t } = useTranslation()
  const dockedChrome = chrome === 'panel'
  const generalAgent = agentKind === 'general'
  const [view, setView] = useState<AgentPanelView>('chat')
  const [inputPrefill, setInputPrefill] = useState<{
    prompt: string
    nonce: number
  } | null>(null)
  const [contextAnalysisOpen, setContextAnalysisOpen] = useState(false)
  const [contextAnalysisLoading, setContextAnalysisLoading] = useState(false)
  const [contextAnalysisError, setContextAnalysisError] = useState<string | null>(null)
  const [contextAnalysis, setContextAnalysis] = useState<ContextAnalysis | null>(null)
  const [activeSubAgentSessionKey, setActiveSubAgentSessionKey] = useState('')
  const [selectedTraceRunId, setSelectedTraceRunId] = useState('')
  const [inputAreaHeight, setInputAreaHeight] = useState(0)
  const pendingWritingInitRef = useRef<string | null>(null)
  const recoveryPaused = Boolean(runtimeProjection?.recovery_paused)
  const runtimeRecovering = Boolean(runtimeProjection?.runtime_recoverable && (!runtimeProjection.stream_attached || recoveryPaused))
  const recoveryAbortAvailable = Boolean(runtimeProjection?.recovery_actions?.some((action) => action.kind === 'abort'))
  const activeControlsDisabled =
    isStreaming && (!runtimeProjection?.active_operation_id?.trim() || Boolean(runtimeProjection?.runtime_recoverable && !runtimeProjection.stream_attached))
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
  const activeRunID = useMemo(() => {
    if (!isExecutionActive) return ''
    const views = buildAgentMessageViews(messages)
    for (let index = views.length - 1; index >= 0; index -= 1) {
      if (!views[index].metadata.subagent && views[index].metadata.run_id) return views[index].metadata.run_id || ''
    }
    return ''
  }, [isExecutionActive, messages])
  const messageListBottomPadding = inputAreaHeight > 0 ? inputAreaHeight + 20 : undefined
  const styleSceneSuggestions = useMemo(() => {
    const teller = resolveNarrativeStyle(tellers, ideTellerId, 'writing')
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
    onSubAgentDetailsChange?.(Boolean(activeSubAgentSessionKey))
  }, [activeSubAgentSessionKey, onSubAgentDetailsChange])

  useEffect(() => {
    return () => {
      onSubAgentDetailsChange?.(false)
    }
  }, [onSubAgentDetailsChange])

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
    if (key) setActiveSubAgentSessionKey(key)
  }, [])

  const openTraceRun = useCallback((runID: string) => {
    if (!runID) return
    setSelectedTraceRunId(runID)
    setView('traces')
  }, [])

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
        .filter((summary) => Boolean(summary.run_id) && summary.run_id !== activeRunID)
        .map((summary, index) => ({
          id: summary.id,
          runId: summary.run_id || '',
          content: (
            <AgentChangeSummaryCard
              projectId={projectId}
              summary={summary}
              disabled={isStreaming}
              eagerPreload={!isStreaming && index === 0}
              onReview={(reviewThreadID, groupID) => onOpenChangeReview?.(reviewThreadID, groupID)}
              onWorkspaceChanged={onWorkspaceChanged}
            />
          ),
        })),
    [activeRunID, changeGroupsQuery.data, isStreaming, onOpenChangeReview, onWorkspaceChanged, projectId],
  )

  const sendWithWritingSkill = async (message: string) => {
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

  const returnQueuedCommandToEditor = useCallback(async (item: AgentRuntimeQueuedCommand) => {
    const prompt = await onEditQueuedCommand?.(item)
    if (typeof prompt !== 'string') return
    setInputPrefill((current) => ({
      prompt,
      nonce: (current?.nonce || 0) + 1,
    }))
  }, [onEditQueuedCommand])

  // Quick actions are writing-workbench affordances tied to the current chapter. The AgentChat
  // workbench is a general project surface, so it opens on a clean conversation instead.
  const emptyChatContent =
    dockedChrome && messages.length === 0 && !isStreaming ? (
      <AgentQuickActions chapter={currentChapter} selectedFile={selectedFile} disabled={persistedSettings.loading} onSend={sendWithWritingSkill} />
    ) : null
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
    activeSubAgentSessionKey,
    onApprovePlan: onApproveProposedPlan,
    onContinuePlan: continuePlanDiscussion,
    onExitPlanMode,
    onOpenTrace: openTraceRun,
    onResolveAsk: resolveAsk,
  }
  const inputAreaProps = {
    onSend: sendWithWritingSkill,
    onStop,
    disabled: sessionTransitionPending,
    sendBlocked: persistedSettings.loading || sessionTransitionPending,
    generationActive: isStreaming,
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
    draftKey: `ide-agent:${workspace || 'global'}:${activeSessionId || 'current'}`,
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
    onContextAnalyze: sessionDraft ? undefined : openContextAnalysis,
    tokenUsageMessages,
    onOpenTrace: openTraceRun,
    agentKey: generalAgent ? ('general' as const) : ('ide' as const),
    workspace,
    conversationBinding: conversationBinding ?? (activeSessionId
      ? { mode: 'writing' as const, project_id: projectId, session_id: activeSessionId }
      : undefined),
    writingSkillControl: generalAgent ? undefined : (
      <WritingComposerSettingsMenu
        enabled={Boolean(workspace) && !persistedSettings.loading && !isStreaming}
        tellers={tellers}
        tellerID={ideTellerId}
        imagePresets={imagePresets}
        imagePresetID={imagePresetId}
        writingSkills={writingSkillOptions}
        writingSkill={writingSkill}
        savingTeller={persistedSettings.isSaving('ide_story_teller_id')}
        savingImagePreset={persistedSettings.isSaving('ide_image_preset_id')}
        savingWritingSkill={persistedSettings.isSaving('writing_skill_default')}
        onTellerChange={(value) => persistedSettings.persist('ide_story_teller_id', value)}
        onImagePresetChange={(value) => persistedSettings.persist('ide_image_preset_id', value)}
        onWritingSkillChange={(value) => persistedSettings.persist('writing_skill_default', value)}
      />
    ),
    onboardingAnchor: 'agent-input',
    floating: true,
    onHeightChange: setInputAreaHeight,
  }
  const chatPane = (
    <AgentChatPane
      className="min-w-0 flex-1"
      contentClassName={dockedChrome ? undefined : 'mx-auto w-full max-w-[56rem]'}
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
        <div className="flex h-10 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-3">
          <div className="flex min-w-0 shrink-0 items-center gap-2 text-xs font-medium text-[var(--nova-text)]">
            <Bot className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
            {t('chat.agent')}
          </div>
          <div
            className="flex h-7 min-w-0 shrink-0 items-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0.5"
            aria-label={t('chat.panelSwitch')}
          >
            <button
              type="button"
              onClick={() => setView('chat')}
              className={`rounded-[6px] px-2 py-0.5 text-[11px] transition-colors ${view === 'chat' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
            >
              {t('chat.view.chat')}
            </button>
            <button
              type="button"
              onClick={() => setView('sessions')}
              className={`rounded-[6px] px-2 py-0.5 text-[11px] transition-colors ${view === 'sessions' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
            >
              {t('chat.view.sessions')}
            </button>
            <button
              type="button"
              onClick={() => setView('traces')}
              className={`rounded-[6px] px-1.5 py-0.5 text-[11px] transition-colors ${view === 'traces' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text-muted)]'}`}
              aria-label={t('chat.view.traces')}
            >
              <Activity className="h-3 w-3" />
            </button>
          </div>
          <button
            type="button"
            disabled={isStreaming || sessionTransitionPending}
            onClick={() => void onCreateSession()}
            className="nova-nav-item flex h-7 w-7 shrink-0 items-center justify-center rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] disabled:cursor-not-allowed disabled:opacity-45"
            aria-label={t('chat.newSession')}
          >
            <Plus className="h-3.5 w-3.5" />
          </button>
          <div className="min-w-0 flex-1" />
          <button type="button" onClick={onClose} className="nova-nav-item rounded p-1" aria-label={t('chat.closeAgent')}>
            <X className="h-3.5 w-3.5" />
          </button>
        </div>
      )}

      {/*
        Without the docked header there is no view switcher, so a trace opened from a message
        card needs its own way back to the conversation.
      */}
      {!dockedChrome && view !== 'chat' && (
        <div className="flex h-9 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-3">
          <button
            type="button"
            onClick={() => setView('chat')}
            className="nova-nav-item flex h-7 items-center gap-1.5 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 text-[11px]"
          >
            <ChevronLeft className="h-3 w-3" />
            {t('chat.view.chat')}
          </button>
          <span className="min-w-0 truncate text-[11px] text-[var(--nova-text-faint)]">{t('chat.view.traces')}</span>
        </div>
      )}

      {view === 'chat' ? (
        <>
          <div className="relative flex min-h-0 flex-1">
            {!activeSubAgentSessionKey ? (
              <StablePortalSlot host={chatPaneHost} fallback={chatPane} wrapFallback={false} className="relative flex min-h-0 min-w-0 flex-1 flex-col" />
            ) : (
              <>
                <Group
                  id="nova-agent-subagent-details"
                  orientation="horizontal"
                  disableCursor
                  resizeTargetMinimumSize={{ coarse: 16, fine: 1 }}
                  className="absolute inset-0 hidden lg:flex"
                >
                  <Panel id="agent-chat" defaultSize="52%" minSize="300px" className="min-w-[300px]">
                    <StablePortalSlot host={chatPaneHost} fallback={chatPane} wrapFallback={false} className="relative flex h-full min-h-0 min-w-0 flex-col" />
                  </Panel>
                  <SubAgentDetailsResizeHandle label={t('chat.subagent.resizeSession')} />
                  <Panel id="subagent-details" defaultSize="48%" minSize="300px" maxSize="68%" className="min-w-[300px]">
                    <AgentSubAgentSessionPanel projectId={projectId} messages={messages} sessionKey={activeSubAgentSessionKey} onClose={() => setActiveSubAgentSessionKey('')} onResolveAsk={resolveAsk} />
                  </Panel>
                </Group>
                <div className="absolute inset-0 z-30 lg:hidden">
                  <AgentSubAgentSessionPanel projectId={projectId} messages={messages} sessionKey={activeSubAgentSessionKey} onClose={() => setActiveSubAgentSessionKey('')} onResolveAsk={resolveAsk} />
                </div>
              </>
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
      ) : view === 'sessions' ? (
        <SessionManagementPanel
          sessions={sessions}
          activeSessionId={activeSessionId}
          disabled={isStreaming || sessionTransitionPending}
          onCreate={onCreateSession}
          onSwitch={onSwitchSession}
          onRename={onRenameSession}
          onDelete={onDeleteSession}
          onEnterChat={() => setView('chat')}
        />
      ) : (
        <AgentTracePanel projectId={projectId} disabled={isStreaming} selectedRunId={selectedTraceRunId} />
      )}
      {chatPanePortal}
    </aside>
  )
}

export const AgentPanel = memo(AgentPanelComponent)

function isGlobalStyleSceneName(scene: string) {
  const normalized = scene.trim().toLowerCase()
  return normalized === '全局' || normalized === 'global'
}

function SubAgentDetailsResizeHandle({ label }: { label: string }) {
  return <Separator aria-label={label} className="nova-resize-handle z-10 -mx-1 hidden w-2 cursor-col-resize bg-transparent transition-colors lg:block" />
}

function AgentQuickActions({
  chapter,
  selectedFile,
  disabled,
  onSend,
}: {
  chapter?: ChapterSummary
  selectedFile: string | null
  disabled?: boolean
  onSend: (message: string) => void
}) {
  const { t } = useTranslation()
  const target = chapter
    ? t('chat.quick.targetChapter', { title: chapter.display_title })
    : selectedFile
      ? t('chat.quick.targetFile', { file: selectedFile })
      : t('chat.quick.targetWork')
  const actions = useMemo(
    () => [
      {
        label: t('chat.quick.nextGroup'),
        icon: FileText,
        prompt:
          '请基于当前大纲、已有章节正文、setting/progress.md、setting/character-states.md 和资料库长期设定，生成接下来一个短期情节单元的章节组细纲。只规划下一组，不要批量生成很多组；细纲要短而可维护，方便阅读、评论和后续更新，每章只写关键点，不写长篇背景解释；如实际正文已经偏离大纲，请先指出偏差并让我确认是调整大纲还是拉回主线。',
      },
      {
        label: t('chat.quick.writeNextChapter'),
        icon: PenLine,
        prompt:
          '请读取当前章节组细纲、长期大纲、setting/progress.md、setting/character-states.md、资料库长期设定和最近至少两章实际正文，按细纲安排创作下一章。写作前以已有章节路径和非空正文判断下一章编号、标题与所属分卷，setting/progress.md 只作为摘要参考；若属于某一卷，请写入 chapters/<分卷名>/ 下符合章节文件名模板的文件。完成正文自检和本轮最后修订后，在同一轮同步更新 setting/progress.md 与 setting/character-states.md；章节是否标记成章不影响同步。',
      },
      {
        label: t('chat.quick.continueParagraph'),
        icon: PenLine,
        prompt: `请基于${target}的上下文，续写下一段正文，保持原有叙事节奏和人物状态。`,
      },
      {
        label: t('chat.quick.polishChapter'),
        icon: WandSparkles,
        prompt: `请检查并润色${target}，重点优化语句节奏、动作描写和情绪推进，不改变核心剧情。`,
      },
      {
        label: t('chat.quick.finalizeState'),
        icon: FileText,
        prompt: `请检查${target}与前后文和当前章节组细纲的连续性，并根据当前实际正文重新同步 setting/progress.md 和 setting/character-states.md；只有角色身份、人设、长期关系、能力体系或世界规则等稳定设定发生明确变化时，才更新资料库。章节状态只作为 UI 编辑标记，除非我明确要求，否则不要修改长期大纲。`,
      },
      {
        label: t('chat.quick.consistencyCheck'),
        icon: SearchCheck,
        prompt: `请对${target}做一致性检查，重点关注人物动机、时间线、道具、地点和前后文冲突。`,
      },
    ],
    [target, t],
  )

  return (
    <div className="border-b border-[var(--nova-border)] bg-[var(--nova-bg)] p-3">
      <div className="mb-2 flex items-center gap-2 text-xs font-medium text-[var(--nova-text-muted)]">
        <Sparkles className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
        {t('chat.quickActions')}
      </div>
      <div className="grid grid-cols-2 gap-2">
        {actions.map((action) => {
          const Icon = action.icon
          return (
            <button
              key={action.label}
              type="button"
              disabled={disabled}
              className="nova-nav-item flex items-center gap-2 border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 text-left text-xs"
              onClick={() => onSend(action.prompt)}
            >
              <Icon className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
              <span className="truncate">{action.label}</span>
            </button>
          )
        })}
      </div>
    </div>
  )
}
