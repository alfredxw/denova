import { useEffect, useMemo, useState } from 'react'
import { Activity, Bot, FileText, PenLine, Plus, SearchCheck, Sparkles, WandSparkles, X } from 'lucide-react'
import { Group, Panel, Separator } from 'react-resizable-panels'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { createStablePortalHost, StablePortalSlot } from '@/components/layout/stable-portal-slot'
import type { ImagePreset, Teller } from '@/features/interactive/types'
import { removeChatContextCompaction } from '@/lib/api'
import type { ChapterIllustration, ChapterSummary, ContextAnalysis, IDEContext, SessionSummary, TextSelection } from '@/lib/api'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { agentSubAgentSessionKey, agentViewContent, buildAgentMessageViews, selectAgentTokenUsageRecords, type AgentMessageView, type AgentPartRef } from '@/lib/agent-message-view'
import { useSkillCommands } from '@/hooks/useSkillCommands'
import { DEFAULT_WRITING_SKILL, useWritingSkillOptions } from '@/hooks/useWritingSkillOptions'
import type { PersistedUserSettingsController } from '@/hooks/usePersistedUserSettings'
import { AgentChatPane } from './AgentChatPane'
import { SessionManagementPanel } from './SessionManagementPanel'
import { AgentTracePanel } from './AgentTracePanel'
import { AgentSubAgentSessionPanel } from './AgentSubAgentSessionPanel'
import { CONTEXT_ANALYSIS_SIMULATED_MESSAGE, ContextAnalysisDialog } from './ContextAnalysisDialog'
import type { ReferencePickerItem } from './FileReferencePicker'
import { WritingComposerSettingsMenu } from './WritingComposerSettingsMenu'
import { formatPlanDiscussionMessage } from '@/lib/plan-mode'
import { useWorkspaceChangeGroups } from '@/features/changes/use-change-review'
import { AgentChangeSummaryCard } from '@/features/changes/agent/AgentChangeSummaryCard'
import { MAX_REVIEW_FEEDBACK_COMMENT_COUNT, MAX_REVIEW_FEEDBACK_CONTEXT_BYTES, reviewFeedbackCommentCount, reviewFeedbackContextBytes, type ReviewFeedbackBatch, type ReviewFeedbackComment, type ReviewFeedbackSelection } from '@/features/changes/agent/ReviewFeedbackTray'
import { toast } from 'sonner'
import type { ChatSendOptions } from '@/hooks/useAgentChat'

type AgentPanelView = 'chat' | 'sessions' | 'traces'

const WRITING_AGENT_INIT_EVENT = 'nova:writing-agent-init'
export const WRITING_COMPOSER_SETTING_DEFAULTS = {
  ide_story_teller_id: 'classic',
  ide_image_preset_id: 'game-cg',
  writing_skill_default: DEFAULT_WRITING_SKILL,
} as const

export type WritingComposerSettingsController = PersistedUserSettingsController<typeof WRITING_COMPOSER_SETTING_DEFAULTS>

interface AgentPanelProps {
  workspace: string
  /** Owned above the conditional panel so closing the panel cannot discard delayed saves. */
  composerSettings: WritingComposerSettingsController
  currentChapter?: ChapterSummary
  selectedFile: string | null
  tellers: Teller[]
  imagePresets?: ImagePreset[]
  messages: AgentUIMessage[]
  sessions: SessionSummary[]
  activeSessionId: string
  isStreaming: boolean
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
  onSend: (message: string, options?: ChatSendOptions) => boolean | Promise<boolean>
  onAnalyzeContext: (message: string, options?: { writingSkill?: string; ideContext?: IDEContext; imagePresetId?: string; tellerId?: string }) => Promise<ContextAnalysis>
  onStop: () => void
  onReferenceRemove: (path: string) => void
  onLoreReferenceAdd: (id: string) => void
  onLoreReferenceRemove: (id: string) => void
  onStyleSceneAdd: (scene: string) => void
  onStyleSceneRemove: (scene: string) => void
  onTextSelectionRemove: (index: number) => void
  onInsertIllustration?: (illustration: ChapterIllustration) => void
  onPlanModeChange: (value: boolean) => void
  onPlanModeToggle: () => void
  onSubmitPlanQuestion: (ref: AgentPartRef, content: string, preview: string) => void
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

/** IDE 右侧创作 Agent 面板，内部支持在对话与完整会话管理之间切换。 */
export function AgentPanel({
  workspace,
  composerSettings: persistedSettings,
  currentChapter,
  selectedFile,
  tellers,
  imagePresets = [],
  messages,
  sessions,
  activeSessionId,
  isStreaming,
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
  onSend,
  onAnalyzeContext,
  onStop,
  onReferenceRemove,
  onLoreReferenceAdd,
  onLoreReferenceRemove,
  onStyleSceneAdd,
  onStyleSceneRemove,
  onTextSelectionRemove,
  onInsertIllustration,
  onPlanModeChange,
  onPlanModeToggle,
  onSubmitPlanQuestion,
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
  const [view, setView] = useState<AgentPanelView>('chat')
  const [inputPrefill, setInputPrefill] = useState<{ prompt: string; nonce: number } | null>(null)
  const [contextAnalysisOpen, setContextAnalysisOpen] = useState(false)
  const [contextAnalysisLoading, setContextAnalysisLoading] = useState(false)
  const [contextAnalysisError, setContextAnalysisError] = useState<string | null>(null)
  const [contextAnalysis, setContextAnalysis] = useState<ContextAnalysis | null>(null)
  const [activeSubAgentSessionKey, setActiveSubAgentSessionKey] = useState('')
  const [selectedTraceRunId, setSelectedTraceRunId] = useState('')
  const [inputAreaHeight, setInputAreaHeight] = useState(0)
  const [chatPaneHost] = useState(() => createStablePortalHost('relative flex h-full min-h-0 w-full min-w-0 flex-col'))
  const ideTellerId = persistedSettings.values.ide_story_teller_id
  const imagePresetId = persistedSettings.values.ide_image_preset_id
  const writingSkill = persistedSettings.values.writing_skill_default
  const skillCommands = useSkillCommands({ agentKey: 'ide', workspace, fallbackEnabled: true })
  const writingSkillOptions = useWritingSkillOptions(workspace)
  const changeGroupsQuery = useWorkspaceChangeGroups(activeSessionId ? workspace : '', { sessionID: activeSessionId })
  const tokenUsageMessages = useMemo(
    () => selectAgentTokenUsageRecords(messages),
    [messages],
  )
  const activeRunID = useMemo(() => {
    if (!isStreaming) return ''
    const views = buildAgentMessageViews(messages)
    for (let index = views.length - 1; index >= 0; index -= 1) {
      if (!views[index].metadata.subagent && views[index].metadata.run_id) return views[index].metadata.run_id || ''
    }
    return ''
  }, [isStreaming, messages])
  const messageListBottomPadding = inputAreaHeight > 0 ? inputAreaHeight + 20 : undefined
  const styleSceneSuggestions = useMemo(() => {
    const teller = tellers.find((item) => item.id === ideTellerId) || tellers.find((item) => item.id === 'classic') || tellers[0]
    return Array.from(new Set((teller?.style_rules || []).map((rule) => rule.scene.trim()).filter((scene) => scene && !isGlobalStyleSceneName(scene))))
  }, [ideTellerId, tellers])

  useEffect(() => {
    const handleWritingInitRequest = (event: Event) => {
      const detail = (event as CustomEvent<{ prompt?: string; autoSend?: boolean }>).detail
      const prompt = detail?.prompt || t('writingAgent.initPrompt')
      setView('chat')
      if (detail?.autoSend && !isStreaming) {
        onSend(prompt, { writingSkill, ideContext, imagePresetId, tellerId: ideTellerId })
        return
      }
      setInputPrefill((current) => ({ prompt, nonce: (current?.nonce || 0) + 1 }))
    }
    window.addEventListener(WRITING_AGENT_INIT_EVENT, handleWritingInitRequest)
    return () => window.removeEventListener(WRITING_AGENT_INIT_EVENT, handleWritingInitRequest)
  }, [ideContext, ideTellerId, imagePresetId, isStreaming, onSend, t, writingSkill])

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
      setContextAnalysis(await onAnalyzeContext(message, { writingSkill, ideContext, imagePresetId, tellerId: ideTellerId }))
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

  const openSubAgentSession = (message: AgentMessageView) => {
    const key = agentSubAgentSessionKey(message)
    if (key) setActiveSubAgentSessionKey(key)
  }

  const openTraceRun = (runID: string) => {
    if (!runID) return
    setSelectedTraceRunId(runID)
    setView('traces')
  }

  const continuePlanDiscussion = (message: AgentMessageView) => {
    setView('chat')
    onPlanModeChange(true)
    setInputPrefill((current) => ({
      prompt: formatPlanDiscussionMessage(agentViewContent(message)),
      nonce: (current?.nonce || 0) + 1,
    }))
  }

  const removeContextCompaction = async () => {
    await removeChatContextCompaction()
    await handleAnalyzeContext(CONTEXT_ANALYSIS_SIMULATED_MESSAGE)
  }

  const timelineAttachments = useMemo(() => (
    (changeGroupsQuery.data ?? [])
      .filter((summary) => Boolean(summary.run_id) && summary.run_id !== activeRunID)
      .map((summary, index) => ({
        id: summary.id,
        runId: summary.run_id || '',
        content: (
          <AgentChangeSummaryCard
            workspace={workspace}
            summary={summary}
            disabled={isStreaming}
            eagerPreload={!isStreaming && index === 0}
            onReview={(reviewThreadID, groupID) => onOpenChangeReview?.(reviewThreadID, groupID)}
            onWorkspaceChanged={onWorkspaceChanged}
          />
        ),
      }))
  ), [activeRunID, changeGroupsQuery.data, isStreaming, onOpenChangeReview, onWorkspaceChanged, workspace])

  const sendWithWritingSkill = async (message: string) => {
    const feedbackSelection = reviewFeedback?.filter((selection) => selection.comments.length) ?? []
    const feedback = feedbackSelection.length ? feedbackSelection.map((selection) => ({
      source: selection.source || 'workspace_change' as const,
      reviewThreadId: selection.reviewThreadId,
      commentIds: selection.comments.map((comment) => comment.id),
    })) : undefined
    const feedbackCount = reviewFeedbackCommentCount(feedbackSelection)
    const effectiveMessage = message.trim() || (feedback
      ? t('changes.feedback.defaultMessage', { count: feedbackCount })
      : message)
    if (feedbackCount > MAX_REVIEW_FEEDBACK_COMMENT_COUNT) {
      toast.error(t('changes.feedback.tooMany', { maximum: MAX_REVIEW_FEEDBACK_COMMENT_COUNT }))
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
      writingSkill,
      ideContext,
      imagePresetId,
      tellerId: ideTellerId,
      reviewFeedback: feedback,
      reviewFeedbackDisplay: feedbackSelection.length ? { comments: feedbackSelection.flatMap((selection) => selection.comments) } : undefined,
      loreReferenceLabels,
      onSubmissionStart: handleSubmissionStart,
      onSubmissionError: handleSubmissionError,
    })
    if (feedbackSelection.length && accepted && !submissionStarted) handleSubmissionStart()
    if (!accepted) handleSubmissionError()
    return accepted
  }

  const emptyChatContent = messages.length === 0 && !isStreaming ? (
    <AgentQuickActions chapter={currentChapter} selectedFile={selectedFile} onSend={sendWithWritingSkill} />
  ) : null
  const messageListProps = {
    messages,
    isStreaming,
    activityContent,
    scrollResetKey: `${workspace || 'none'}:${activeSessionId || 'current'}`,
    bottomPaddingClassName: 'pb-36',
    bottomPaddingPx: messageListBottomPadding,
    collapseTraceGroups: true,
    activeTraceDisplay: 'collapsed' as const,
    hasEarlierMessages,
    isLoadingEarlierMessages: isLoadingEarlierHistory,
    onLoadEarlierMessages: onLoadEarlierHistory,
    timelineAttachments,
    onOpenSubAgentSession: openSubAgentSession,
    onInsertIllustration,
    activeSubAgentSessionKey,
    onSubmitPlanQuestion,
    onApprovePlan: onApproveProposedPlan,
    onContinuePlan: continuePlanDiscussion,
    onExitPlanMode,
    onOpenTrace: openTraceRun,
  }
  const inputAreaProps = {
    onSend: sendWithWritingSkill,
    onStop,
    disabled: isStreaming,
    planMode,
    onTogglePlanMode: onPlanModeToggle,
    draftKey: `ide-agent:${workspace || 'global'}`,
    inputPrefill,
    onInputPrefillConsumed: () => setInputPrefill(null),
    referencedFiles: references,
    onReferenceRemove,
    fileSuggestions,
    loreReferences,
    loreReferenceLabels,
    onLoreReferenceAdd,
    onLoreReferenceRemove,
    loreSuggestions,
    styleScenes,
    onStyleSceneAdd,
    onStyleSceneRemove,
    styleSceneSuggestions,
    textSelections,
    onTextSelectionRemove,
    reviewFeedback,
    onReviewFeedbackOpen,
    onReviewFeedbackRemove,
    skills: skillCommands,
    onContextAnalyze: openContextAnalysis,
    tokenUsageMessages,
    onOpenTrace: openTraceRun,
    agentKey: 'ide' as const,
    workspace,
    writingSkillControl: (
      <WritingComposerSettingsMenu
        enabled={Boolean(workspace) && !persistedSettings.loading}
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
      emptyContent={emptyChatContent}
      messageListProps={messageListProps}
      inputAreaProps={inputAreaProps}
    />
  )
  const chatPanePortal = view === 'chat' && chatPaneHost
    ? createPortal(chatPane, chatPaneHost, 'agent-chat-pane')
    : null

  return (
    <aside className="nova-sidebar relative flex h-full min-h-0 flex-col overflow-hidden border-l border-[var(--nova-border)] bg-[var(--nova-surface)] shadow-[-14px_0_30px_-28px_rgba(15,23,42,0.72)]">
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-3">
        <div className="flex min-w-0 shrink-0 items-center gap-2 text-xs font-medium text-[var(--nova-text)]">
          <Bot className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
          {t('chat.agent')}
        </div>
        <div className="flex h-7 min-w-0 shrink-0 items-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0.5" aria-label={t('chat.panelSwitch')}>
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
            title={t('chat.view.traces')}
          >
            <Activity className="h-3 w-3" />
          </button>
        </div>
        <button
          type="button"
          disabled={isStreaming}
          onClick={() => void onCreateSession()}
          className="nova-nav-item flex h-7 w-7 shrink-0 items-center justify-center rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] disabled:cursor-not-allowed disabled:opacity-45"
          aria-label={t('chat.newSession')}
          title={t('chat.newSession')}
        >
          <Plus className="h-3.5 w-3.5" />
        </button>
        <div className="min-w-0 flex-1" />
        <button
          type="button"
          onClick={onClose}
          className="nova-nav-item rounded p-1"
          aria-label={t('chat.closeAgent')}
          title={t('common.close')}
        >
          <X className="h-3.5 w-3.5" />
        </button>
      </div>

      {view === 'chat' ? (
        <>
          <div className="relative flex min-h-0 flex-1">
            {!activeSubAgentSessionKey ? (
              <StablePortalSlot
                host={chatPaneHost}
                fallback={chatPane}
                wrapFallback={false}
                className="relative flex min-h-0 min-w-0 flex-1 flex-col"
              />
            ) : (
              <>
                <Group
                  id="nova-agent-subagent-details"
                  orientation="horizontal"
                  resizeTargetMinimumSize={{ coarse: 16, fine: 1 }}
                  className="absolute inset-0 hidden lg:flex"
                >
                  <Panel id="agent-chat" defaultSize="52%" minSize="300px" className="min-w-[300px]">
                    <StablePortalSlot
                      host={chatPaneHost}
                      fallback={chatPane}
                      wrapFallback={false}
                      className="relative flex h-full min-h-0 min-w-0 flex-col"
                    />
                  </Panel>
                  <SubAgentDetailsResizeHandle label={t('chat.subagent.resizeSession')} />
                  <Panel id="subagent-details" defaultSize="48%" minSize="300px" maxSize="68%" className="min-w-[300px]">
                    <AgentSubAgentSessionPanel
                      messages={messages}
                      sessionKey={activeSubAgentSessionKey}
                      onClose={() => setActiveSubAgentSessionKey('')}
                    />
                  </Panel>
                </Group>
                <div className="absolute inset-0 z-30 lg:hidden">
                  <AgentSubAgentSessionPanel
                    messages={messages}
                    sessionKey={activeSubAgentSessionKey}
                    onClose={() => setActiveSubAgentSessionKey('')}
                  />
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
          disabled={isStreaming}
          onCreate={onCreateSession}
          onSwitch={onSwitchSession}
          onRename={onRenameSession}
          onDelete={onDeleteSession}
          onEnterChat={() => setView('chat')}
        />
      ) : (
        <AgentTracePanel disabled={isStreaming} selectedRunId={selectedTraceRunId} />
      )}
      {chatPanePortal}
    </aside>
  )
}

function isGlobalStyleSceneName(scene: string) {
  const normalized = scene.trim().toLowerCase()
  return normalized === '全局' || normalized === 'global'
}

function SubAgentDetailsResizeHandle({ label }: { label: string }) {
  return (
    <Separator
      aria-label={label}
      className="nova-resize-handle z-10 -mx-1 hidden w-2 cursor-col-resize bg-transparent transition-colors lg:block"
    />
  )
}

function AgentQuickActions({
  chapter,
  selectedFile,
  onSend,
}: {
  chapter?: ChapterSummary
  selectedFile: string | null
  onSend: (message: string) => void
}) {
  const { t } = useTranslation()
  const target = chapter ? t('chat.quick.targetChapter', { title: chapter.display_title }) : (selectedFile ? t('chat.quick.targetFile', { file: selectedFile }) : t('chat.quick.targetWork'))
  const actions = useMemo(() => [
    { label: t('chat.quick.nextGroup'), icon: FileText, prompt: '请基于当前大纲、已定稿章节、progress.md、character-states.md 和资料库长期设定，生成接下来一个短期情节单元的章节组细纲。只规划下一组，不要批量生成很多组；细纲要短而可维护，方便阅读、评论和后续更新，每章只写关键点，不写长篇背景解释；如实际定稿已经偏离大纲，请先指出偏差并让我确认是调整大纲还是拉回主线。' },
    { label: t('chat.quick.writeNextChapter'), icon: PenLine, prompt: '请读取当前章节组细纲、长期大纲、progress.md、character-states.md、资料库长期设定和前面至少两章成章正文，按细纲安排创作下一章。写作前请先按长期大纲的卷章安排和已有章节路径判断下一章所属分卷；若属于某一卷，请写入 chapters/<分卷名>/ 下符合章节文件名模板的文件。新写入的非空章节先作为初稿，由我在章节列表确认后再标记为成章。' },
    { label: t('chat.quick.continueParagraph'), icon: PenLine, prompt: `请基于${target}的上下文，续写下一段正文，保持原有叙事节奏和人物状态。` },
    { label: t('chat.quick.polishChapter'), icon: WandSparkles, prompt: `请检查并润色${target}，重点优化语句节奏、动作描写和情绪推进，不改变核心剧情。` },
    { label: t('chat.quick.finalizeState'), icon: FileText, prompt: `请将${target}视为章节定稿，检查其与前后文和当前章节组细纲的连续性，然后同步更新 progress.md 和 character-states.md；只有角色身份、人设、长期关系、能力体系或世界规则等稳定设定发生明确变化时，才更新资料库。除非我明确要求，不要修改长期大纲。` },
    { label: t('chat.quick.consistencyCheck'), icon: SearchCheck, prompt: `请对${target}做一致性检查，重点关注人物动机、时间线、道具、地点和前后文冲突。` },
  ], [target, t])

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
