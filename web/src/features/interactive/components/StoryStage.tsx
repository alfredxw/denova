import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import { PanelRight } from 'lucide-react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import { LoadingState } from '@/components/common/LoadingState'
import { Button } from '@/components/ui/button'
import { CONTEXT_ANALYSIS_SIMULATED_MESSAGE } from '@/components/Chat/ContextAnalysisDialog'
import { MessageList, type TurnScrollRequest } from '@/components/Chat/MessageList'
import { AgentSubAgentSessionPanel } from '@/components/Chat/AgentSubAgentSessionPanel'
import type { ComposerTokenInputHandle, ComposerTokenSpec, ComposerTrigger } from '@/components/Chat/composer-token-input'
import { MOBILE_NAVIGATION_OPEN_EVENT } from '@/components/layout/workspace-mobile-layout'
import type { ContextAnalysis } from '@/lib/api'
import type { AgentUIMessage } from '@/lib/agent-ui'
import { agentMessageDisplayText, createAgentDataMessage } from '@/lib/agent-ui-message'
import { agentSubAgentSessionKey, agentViewContent, type AgentMessageView } from '@/lib/agent-message-view'
import { useSkillCommands } from '@/hooks/useSkillCommands'
import { useConversationConfig } from '@/features/conversation-config/use-conversation-config'
import type { ConversationConfigBinding } from '@/features/conversation-config/types'
import { useConversationGoal } from '@/features/agent-goal/use-conversation-goal'
import { analyzeInteractiveContext, getInteractiveHistoryPage, removeInteractiveContextCompaction, runInteractiveDirector, switchInteractiveTurnVersion, updateInteractiveTurnNarrative } from '../api'
import { sanitizeStoredNarrative } from '../stream-parser'
import { emptyStoryStageRun, useInteractiveStore } from '../stores/interactive-store'
import type { StoryStageRunState } from '../stores/interactive-store'
import { useInteractiveAgentCommands, type StoryStageRuntimeUpdater } from '../use-interactive-agent-commands'
import { buildOpeningPrompt, truncateStoryOpeningText } from '../opening'
import { DEFAULT_NARRATIVE_STYLE_ID } from '../narrative-style'
import { StoryPicker } from './StoryPicker'
import { NewStorySetupPanel } from './NewStorySetupPanel'
import { StoryOpeningPanel } from './StoryOpeningPanel'
import { StoryDirectorPicker } from './StoryDirectorPicker'
import { ReplyTargetCharsControl } from './ReplyTargetCharsControl'
import { TurnNavigator } from './TurnNavigator'
import { DEFAULT_STORY_STATE_DISPLAY, type StoryStateDisplayPreference } from './story-state/display-preference'
import { StoryStateLedger } from './story-state/StoryStateLedger'
import { buildStoryStateModel } from './story-state/model'
import { storyRuleVisibilityMode } from './story-stage/rule-roll'
import { useStagePreferences } from './story-stage/use-stage-preferences'
import { parseInlineStyleScenes, storyStageSnapshotKey } from './story-stage/utils'
import { useLiveMessageAccumulator } from './story-stage/use-live-message-accumulator'
import { useStoryStageMessages } from './story-stage/use-story-stage-messages'
import { useStoryHistoryWindow } from './story-stage/use-story-history-window'
import { useStoryImages } from './story-stage/use-story-images'
import { useStoryStageRuntime } from './story-stage/use-story-stage-runtime'
import { StoryStageComposer } from './story-stage/StoryStageComposer'
import { StoryStageHeader } from './story-stage/StoryStageHeader'
import { buildStoryStageCommandMenu } from './story-stage/story-stage-commands'
import { branchCreationSourceFromMessage, branchCreationSourceFromTurn } from './branching/model'
import type { StoryStageProps } from './story-stage/story-stage-props'
import { useIsMobile } from '@/hooks/useIsMobile'
import { useKeyboardInset } from '@/hooks/useKeyboardInset'
import type { InputAreaSendOptions } from '@/components/Chat/InputArea'

const DEFAULT_READING_FONT_SIZE = 18
const EMPTY_STAGE_RUN = emptyStoryStageRun()

export function StoryStage({ projectId, workspace, styleSceneSuggestions = [], stories = [], story, tellers = [], storyDirectors = [], imagePresets = [], recentNarrativeStyleID = DEFAULT_NARRATIVE_STYLE_ID, narrativeStyleLoading = false, storyId, branchId, snapshot, snapshotLoading = false, loreEmpty = false, bookOpeningPresets = [], directorPanelVisible = true, stateDisplayPreference = DEFAULT_STORY_STATE_DISPLAY, onStorySelect = noop, onStoryCreate = noop, onStorySetupUpdate = noop, onNarrativeStyleChange, onStoryDelete = noop, onDirectorChange = noop, onReplyTargetCharsChange, onImageSettingsChange, onRequestLoreInit, onOpenDirectorConfig, onToggleDirectorPanel, onOpenDirectorState, onRequestCreateBranch, onStateDisplayPreferenceChange = noopStateDisplayPreferenceChange, onTurnPersisted = noopTurnPersisted, onDone }: StoryStageProps) {
  const { t } = useTranslation()
  const conversationBinding = useMemo<ConversationConfigBinding | undefined>(() => storyId ? {
    mode: 'interactive', project_id: projectId, story_id: storyId,
    branch_id: branchId || snapshot?.branch_id || 'main',
  } : undefined, [branchId, projectId, snapshot?.branch_id, storyId])
  const conversationConfig = useConversationConfig(conversationBinding)
  const approvalReady = conversationConfig.initialized && !conversationConfig.saving
  const isMobile = useIsMobile()
  const keyboardInset = useKeyboardInset()
  const storyStateModel = useMemo(() => buildStoryStateModel(snapshot), [snapshot])
  const [input, setInput] = useState('')
  const [goalMode, setGoalMode] = useState(false)
  const [stageControlsOpen, setStageControlsOpen] = useState(false)
  const [styleScenes, setStyleScenes] = useState<string[]>([])
  const [styleSceneQuery, setStyleSceneQuery] = useState<string | null>(null)
  const [showSkillCommands, setShowSkillCommands] = useState(false)
  const [skillCommandQuery, setSkillCommandQuery] = useState<string | null>(null)
  const [activeSkillCommandIndex, setActiveSkillCommandIndex] = useState(0)
  const [inputFloatHeight, setInputFloatHeight] = useState(0)
  const inputRef = useRef<ComposerTokenInputHandle | null>(null)
  const inputFloatRef = useRef<HTMLDivElement | null>(null)
  const skillCommands = useSkillCommands({
    agentKey: 'interactive_story',
    projectId,
  })
  const snapshotKey = storyStageSnapshotKey(storyId, branchId, snapshot)
  const stageKey = `${workspace || 'current'}:${storyId || 'none'}:${branchId || snapshot?.branch_id || 'main'}`
  const { displaySnapshot, historyWindow, prependPage: prependHistoryPage, resetToLatest: resetHistoryToLatest } = useStoryHistoryWindow(stageKey, snapshot)
  const [historyLoading, setHistoryLoading] = useState(false)
  const stageRun = useInteractiveStore((state) => state.storyStageRuns[stageKey] || EMPTY_STAGE_RUN)
  const setStoryStageRun = useInteractiveStore((state) => state.setStoryStageRun)
  const clearStoryStageRun = useInteractiveStore((state) => state.clearStoryStageRun)
  const streaming = stageRun.streaming
  const conversationGoal = useConversationGoal(conversationBinding, streaming)
  const activityContent = stageRun.activityContent
  const liveMessages = stageRun.liveMessages
  const rewindTurnId = stageRun.rewindTurnId
  const branchTerminal = snapshot?.current_turn?.terminal_outcome?.terminal === true
  const publicRuleRollVisible = useMemo(
    () => storyRuleVisibilityMode(story, storyDirectors) === 'public_roll',
    [story, storyDirectors],
  )
  const [replyEditTarget, setReplyEditTarget] = useState<{
    turnId: string
    branchId: string
    initialContent: string
    expectedNarrative: string
  } | null>(null)
  const [editingTurn, setEditingTurn] = useState<{
    id: string
    content: string
  } | null>(null)
  const [switchingVersionTurnId, setSwitchingVersionTurnId] = useState<string | null>(null)
  const [hotChoicesExpanded, setHotChoicesExpanded] = useState(false)
  const [customOpeningText, setCustomOpeningText] = useState('')
  const [selectedBookOpeningPresetId, setSelectedBookOpeningPresetId] = useState('')
  const [creatingStory, setCreatingStory] = useState(false)
  const [editingStorySetup, setEditingStorySetup] = useState(false)
  const [directorRetrying, setDirectorRetrying] = useState(false)
  const [directorRetryError, setDirectorRetryError] = useState('')
  const [contextAnalysisOpen, setContextAnalysisOpen] = useState(false)
  const [tokenUsageOpen, setTokenUsageOpen] = useState(false)
  const [traceOpen, setTraceOpen] = useState(false)
  const [selectedTraceRunId, setSelectedTraceRunId] = useState('')
  const [contextAnalysisLoading, setContextAnalysisLoading] = useState(false)
  const [contextAnalysisError, setContextAnalysisError] = useState<string | null>(null)
  const [contextAnalysis, setContextAnalysis] = useState<ContextAnalysis | null>(null)
  const [activeSubAgentSessionKey, setActiveSubAgentSessionKey] = useState('')
  const [activeTurnAnchorId, setActiveTurnAnchorId] = useState('')
  const [turnScrollRequest, setTurnScrollRequest] = useState<TurnScrollRequest>()

  useEffect(() => {
    setReplyEditTarget(null)
    setGoalMode(false)
  }, [stageKey])
  useEffect(() => {
    if (streaming && !historyWindow.followLatest) resetHistoryToLatest()
  }, [historyWindow.followLatest, resetHistoryToLatest, streaming])
  const previousSnapshotKeyRef = useRef(snapshotKey)
  const stagePreferences = useStagePreferences(projectId)
  const stageTextStyle = useMemo<CSSProperties>(
    () => ({
      fontSize: `var(--nova-reading-font-size, ${DEFAULT_READING_FONT_SIZE}px)`,
      lineHeight: stagePreferences.lineHeight,
      fontFamily: 'var(--nova-reading-font-family)',
    }),
    [stagePreferences.lineHeight],
  )
  const inputTextStyle = useMemo<CSSProperties>(
    () => ({
      fontSize: `min(var(--nova-reading-font-size, ${DEFAULT_READING_FONT_SIZE}px), 16px)`,
      lineHeight: 1.35,
      fontFamily: 'var(--nova-reading-font-family)',
    }),
    [],
  )

  const updateStageRun = useCallback(
    (updater: Partial<StoryStageRunState> | ((current: StoryStageRunState) => StoryStageRunState)) => {
      setStoryStageRun(stageKey, updater)
    },
    [setStoryStageRun, stageKey],
  )

  const setStageRuntime = useCallback(
    (runtime: StoryStageRuntimeUpdater) => updateStageRun((current) => ({
      ...current,
      runtime: typeof runtime === 'function' ? runtime(current.runtime) : runtime,
    })),
    [updateStageRun],
  )
  const readStageRuntime = useCallback(
    () => useInteractiveStore.getState().storyStageRuns[stageKey]?.runtime || EMPTY_STAGE_RUN.runtime,
    [stageKey],
  )
  const interactiveAgentCommands = useInteractiveAgentCommands({
    storyId,
    branchId,
    readRuntime: readStageRuntime,
    onRuntimeChange: setStageRuntime,
  })
  const setStageStreaming = useCallback(
    (value: boolean) => {
      updateStageRun({ streaming: value })
    },
    [updateStageRun],
  )

  const setStageActivityContent = useCallback(
    (value: string) => {
      updateStageRun({ activityContent: value })
    },
    [updateStageRun],
  )

  const openTraceRun = useCallback((runID: string) => {
    if (!runID) return
    setSelectedTraceRunId(runID)
    setTokenUsageOpen(false)
    setTraceOpen(true)
  }, [])

  const setStageLiveMessages = useCallback(
    (updater: AgentUIMessage[] | ((current: AgentUIMessage[]) => AgentUIMessage[])) => {
      updateStageRun((current) => ({
        ...current,
        liveMessages: typeof updater === 'function' ? updater(current.liveMessages) : updater,
      }))
    },
    [updateStageRun],
  )

  const liveAccumulator = useLiveMessageAccumulator({
    publicRuleRollVisible,
    setMessages: setStageLiveMessages,
  })
  const storyImages = useStoryImages({
    stageKey,
    storyId,
    branchId,
    snapshot,
    t,
    onDone,
    setActivity: setStageActivityContent,
  })
  const liveTurnNavigationAnchorId = useMemo(() => `live:${stageKey}`, [stageKey])
  const {
    agentMessages,
    tokenUsageMessages,
    turnNavigationItems,
    turnsById,
  } = useStoryStageMessages({
    snapshot: displaySnapshot,
    rewindTurnId,
    liveMessages,
    streaming,
    stageKey,
    liveTurnNavigationAnchorId,
    publicRuleRollVisible,
    optimisticInteractiveImages: storyImages.optimisticImages,
    belongsToStage: liveAccumulator.belongsToStage,
    renderKeyFor: liveAccumulator.renderKeyFor,
  })
  const {
    commands: filteredSkillCommands,
    builtInItems: filteredBuiltInCommandItems,
    skillItems: filteredSkillCommandItems,
  } = useMemo(() => buildStoryStageCommandMenu(skillCommandQuery, skillCommands, {
    compactDescription: t('chat.command.compact.desc'),
    compactHint: t('chat.command.compact.hint'),
    goalDescription: t('chat.command.goal.desc'),
    goalHint: t('chat.command.goal.hint'),
    skillHint: t('chat.command.skill.hint'),
  }), [skillCommandQuery, skillCommands, t])

  useEffect(() => {
    if (previousSnapshotKeyRef.current === snapshotKey) return
    if (streaming) return
    previousSnapshotKeyRef.current = snapshotKey
    setStageActivityContent('')
    if (liveMessages.length > 0) {
      clearStoryStageRun(stageKey)
    }
  }, [clearStoryStageRun, liveMessages.length, setStageActivityContent, snapshotKey, stageKey, streaming])

  useEffect(() => {
    if (activeSkillCommandIndex >= filteredSkillCommands.length) setActiveSkillCommandIndex(0)
  }, [activeSkillCommandIndex, filteredSkillCommands.length])

  const handleTurnNavigationSelect = useCallback((anchorId: string) => {
    setActiveTurnAnchorId(anchorId)
    setTurnScrollRequest((current) => ({
      anchorId,
      requestId: (current?.requestId || 0) + 1,
    }))
  }, [])
  const handleVisibleTurnAnchorChange = useCallback((anchorId: string) => {
    setActiveTurnAnchorId(anchorId)
  }, [])
  useEffect(() => {
    const fallbackAnchorId = turnNavigationItems[turnNavigationItems.length - 1]?.anchorId || ''
    setActiveTurnAnchorId((current) => {
      if (!current) return fallbackAnchorId
      return turnNavigationItems.some((item) => item.anchorId === current) ? current : fallbackAnchorId
    })
  }, [turnNavigationItems])
  const openSubAgentSession = useCallback((view: AgentMessageView) => {
    const key = agentSubAgentSessionKey(view)
    if (key) setActiveSubAgentSessionKey(key)
  }, [])
  const scrollResetKey = `${storyId || 'none'}:${branchId || snapshot?.branch_id || 'main'}`
  const hotChoices = useMemo(
    () =>
      (snapshot?.current_turn?.turn_result?.choices || snapshot?.current_turn?.hot_state?.choices || [])
        .map((choice) => choice.trim())
        .filter(Boolean),
    [snapshot?.current_turn?.hot_state?.choices, snapshot?.current_turn?.turn_result?.choices],
  )
  const directorPlanStatus = snapshot?.director_plan_status
  const directorBlocking = false
  const directorStatusVisible = Boolean(directorPlanStatus && directorBlocking)
  const canUseHotChoices = hotChoices.length > 0 && !branchTerminal && !streaming && !editingTurn && !directorBlocking && Boolean(storyId)
  const showHotChoices = canUseHotChoices && hotChoicesExpanded
  const messageListBottomPadding = inputFloatHeight > 0 ? inputFloatHeight + keyboardInset + 20 : undefined
  const loadEarlierMessages = useCallback(async () => {
    if (!storyId || !historyWindow.beforeCursor || historyLoading) return
    setHistoryLoading(true)
    try {
      const page = await getInteractiveHistoryPage(storyId, branchId || snapshot?.branch_id || 'main', historyWindow.beforeCursor)
      prependHistoryPage(page)
    } catch (error) {
      console.error('[interactive-stage] load earlier story history failed', error)
      setStageLiveMessages((current) => [...current, errorMessage(error instanceof Error ? error.message : t('chat.history.loadEarlierFailed'))])
    } finally {
      setHistoryLoading(false)
    }
  }, [branchId, historyLoading, historyWindow.beforeCursor, prependHistoryPage, setStageLiveMessages, snapshot?.branch_id, storyId, t])
  const latestTurnID = snapshot?.current_turn?.id || snapshot?.turns?.at(-1)?.id || ''
  const canMutateStoryView = useCallback((view: AgentMessageView) => {
    const turnID = view.metadata.turn_id
    return !turnID || !latestTurnID || turnID === latestTurnID
  }, [latestTurnID])
  const availableBookOpeningPresets = useMemo(() => bookOpeningPresets.filter((preset) => preset.content.trim()), [bookOpeningPresets])
  const selectedBookOpeningPreset = useMemo(
    () => availableBookOpeningPresets.find((preset) => preset.id === selectedBookOpeningPresetId) || availableBookOpeningPresets[0] || null,
    [availableBookOpeningPresets, selectedBookOpeningPresetId],
  )
  const syncInputFloatHeight = useCallback(() => {
    const element = inputFloatRef.current
    if (!element) return
    const nextHeight = Math.ceil(element.getBoundingClientRect().height)
    setInputFloatHeight((current) => (current === nextHeight ? current : nextHeight))
  }, [])

  useLayoutEffect(() => {
    syncInputFloatHeight()
  }, [conversationGoal.goal?.revision, directorRetryError, directorRetrying, directorStatusVisible, editingTurn, goalMode, hotChoices.length, input, showHotChoices, stageRun.runtime.queue.length, syncInputFloatHeight])

  useEffect(() => {
    setSelectedBookOpeningPresetId((current) => {
      if (current && availableBookOpeningPresets.some((preset) => preset.id === current)) return current
      return availableBookOpeningPresets[0]?.id || ''
    })
  }, [availableBookOpeningPresets])

  useEffect(() => {
    if (directorPlanStatus?.status !== 'failed') setDirectorRetryError('')
  }, [directorPlanStatus?.status])

  useEffect(() => {
    const element = inputFloatRef.current
    if (!element || typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(syncInputFloatHeight)
    observer.observe(element)
    return () => observer.disconnect()
  }, [syncInputFloatHeight])

  const toggleHotChoices = () => {
    if (!canUseHotChoices) return
    setHotChoicesExpanded((value) => !value)
  }

  useEffect(() => {
    setHotChoicesExpanded(false)
  }, [snapshotKey])

  function clearSubmittedComposer() {
    setInput('')
    setGoalMode(false)
    setEditingTurn(null)
    setStyleScenes([])
    setStyleSceneQuery(null)
    setShowSkillCommands(false)
    setSkillCommandQuery(null)
    setActiveSkillCommandIndex(0)
  }

  const { commandSubmitting, deleteQueuedCommand, queueActionPendingCommandID, send, steerQueuedCommand, stop } = useStoryStageRuntime({
    stageKey,
    storyId,
    branchId,
    input,
    editingTurnId: editingTurn?.id,
    styleScenes,
    streaming,
    branchTerminal,
    blocked: directorBlocking || !approvalReady,
    stageRun,
    liveTurnNavigationAnchorId,
    t,
    interactiveAgentCommands,
    liveAccumulator,
    storyImages,
    readStageRuntime,
    setStageRuntime,
    updateStageRun,
    setStreaming: setStageStreaming,
    setActivity: setStageActivityContent,
    setMessages: setStageLiveMessages,
    clearComposer: clearSubmittedComposer,
    onTurnPersisted,
    onDone,
  })

  const enterGoalMode = () => {
    setEditingTurn(null)
    setGoalMode(true)
    setShowSkillCommands(false)
    setSkillCommandQuery(null)
    setActiveSkillCommandIndex(0)
    window.requestAnimationFrame(() => inputRef.current?.focus())
  }

  const submitComposer = async (options?: InputAreaSendOptions) => {
    if (!goalMode) return send(options)
    const objective = input.trim()
    if (!objective || conversationGoal.saving) return false
    const next = await conversationGoal.set(objective)
    if (!next) {
      toast.error(t('chat.goal.updateFailed'))
      return false
    }
    return send(options)
  }

  const editGoal = () => {
    if (!conversationGoal.goal) return
    setInput(conversationGoal.goal.objective)
    enterGoalMode()
  }

  const pauseGoal = async () => {
    const next = await conversationGoal.pause()
    if (!next) {
      toast.error(t('chat.goal.updateFailed'))
      return
    }
    if (streaming) await stop()
  }

  const clearGoal = async () => {
    const next = await conversationGoal.clear()
    if (!next) {
      toast.error(t('chat.goal.updateFailed'))
      return
    }
    if (streaming) await stop()
  }

  const analyzeCurrentContext = async (rawMessage: string) => {
    const message = rawMessage.trim()
    if (!message || !storyId || streaming) return
    const inlineStyleScenes = parseInlineStyleScenes(message)
    const mergedStyleScenes = Array.from(new Set([...styleScenes, ...inlineStyleScenes]))
    setContextAnalysisLoading(true)
    setContextAnalysisError(null)
    setContextAnalysis(null)
    try {
      setContextAnalysis(await analyzeInteractiveContext({
        mode: 'story',
        story_id: storyId,
        branch: branchId,
        message,
        style_scenes: mergedStyleScenes,
      }))
    } catch (e) {
      setContextAnalysis(null)
      setContextAnalysisError((e as Error).message)
    } finally {
      setContextAnalysisLoading(false)
    }
  }

  const openContextAnalysis = () => {
    setContextAnalysisOpen(true)
    void analyzeCurrentContext(CONTEXT_ANALYSIS_SIMULATED_MESSAGE)
  }

  const removeContextCompaction = async () => {
    await removeInteractiveContextCompaction(storyId, branchId)
    await onDone()
    await analyzeCurrentContext(CONTEXT_ANALYSIS_SIMULATED_MESSAGE)
  }

  const startBookPresetOpening = () => {
    if (!storyId || streaming) return
    if (!selectedBookOpeningPreset?.content.trim()) return
    void send({
      message: buildOpeningPrompt(
        story,
        t,
        {
          mode: 'preset',
          preset_id: selectedBookOpeningPreset.id,
          preset_text: truncateStoryOpeningText(selectedBookOpeningPreset.content),
        },
        'book_preset',
      ),
    })
  }

  const startAIOpening = () => {
    if (!storyId || streaming) return
    void send({ message: buildOpeningPrompt(story, t, { mode: 'ai' }) })
  }

  const startOpening = () => {
    if (!storyId || streaming) return
    const customText = truncateStoryOpeningText(customOpeningText)
    if (!customText) return
    void send({ message: buildOpeningPrompt(story, t, { mode: 'custom', custom_text: customText }) })
    setCustomOpeningText('')
  }

  const retryDirectorPlanning = async () => {
    if (!storyId || directorRetrying) return
    setDirectorRetrying(true)
    setDirectorRetryError('')
    try {
      await runInteractiveDirector(storyId, branchId || snapshot?.branch_id)
      await onDone({ silent: true })
    } catch (error) {
      console.warn('[interactive-stage] retry director planning failed', error)
      setDirectorRetryError(error instanceof Error ? error.message : t('storyStage.director.retryFailed'))
    } finally {
      setDirectorRetrying(false)
    }
  }

  const switchMessageVersion = async (view: AgentMessageView, direction: -1 | 1) => {
    const turnId = view.metadata.turn_id
    if (!turnId || !storyId || streaming || switchingVersionTurnId) return
    const versions = view.metadata.turn_versions || []
    const currentIndex = view.metadata.turn_version_index ?? versions.findIndex((version) => version.current)
    const nextVersion = versions[currentIndex + direction]
    if (!nextVersion) return
    setSwitchingVersionTurnId(turnId)
    setStageActivityContent(direction > 0 ? t('storyStage.activity.switchNewer') : t('storyStage.activity.switchOlder'))
    try {
      await switchInteractiveTurnVersion(storyId, {
        branch_id: branchId,
        turn_id: turnId,
        version_turn_id: nextVersion.turn_id,
      })
      clearStoryStageRun(stageKey)
      await onDone()
    } catch (error) {
      setStageLiveMessages((prev) => [
        ...prev,
        errorMessage(error instanceof Error ? error.message : t('storyStage.activity.switchFailed')),
      ])
    } finally {
      setSwitchingVersionTurnId(null)
      setStageActivityContent('')
    }
  }

  const startEditingView = (view: AgentMessageView) => {
    const turnId = view.metadata.turn_id
    if (!turnId || streaming) return
    const content = agentViewContent(view)
    setEditingTurn({ id: turnId, content })
    setInput(content)
    setShowSkillCommands(false)
    setActiveSkillCommandIndex(0)
    window.requestAnimationFrame(() => {
      inputRef.current?.focus()
      inputRef.current?.setSelectionRange(content.length, content.length)
    })
  }

  const startEditingAssistantReply = (view: AgentMessageView) => {
    if (streaming || storyImages.generatingTurnId || switchingVersionTurnId) return
    const turnId = view.metadata.turn_id
    if (!turnId) return
    const turn = turnsById.get(turnId)
    if (!turn) return
    setReplyEditTarget({
      turnId: turn.id,
      branchId: turn.branch_id || branchId,
      initialContent: sanitizeStoredNarrative(turn.narrative),
      expectedNarrative: turn.narrative,
    })
  }

  const startCreatingBranchFromView = (view: AgentMessageView) => {
    const turnId = view.metadata.turn_id
    if (!turnId || !onRequestCreateBranch) return
    const turn = turnsById.get(turnId)
    onRequestCreateBranch(turn
      ? branchCreationSourceFromTurn(turn, t('branchTimeline.nodeFallback'))
      : branchCreationSourceFromMessage(turnId, agentViewContent(view), t('branchTimeline.nodeFallback')))
  }

  const regenerateView = (view: AgentMessageView) => {
    if (streaming) return
    const turnId = view.metadata.turn_id
    if (!turnId) {
      const liveUserMessage = [...liveMessages].reverse().find((item) => item.role === 'user')
      const source = stageRun.retryMessage || (liveUserMessage ? agentMessageDisplayText(liveUserMessage) : '')
      if (source.trim()) void send({ message: source })
      return
    }
    const source = turnsById.get(turnId)?.user || agentViewContent(view)
    void send({ message: source, rewindTurnId: turnId })
  }

  const switchViewVersion = (view: AgentMessageView, direction: -1 | 1) => {
    void switchMessageVersion(view, direction)
  }

  const generateImageForView = (view: AgentMessageView) => {
    void storyImages.generateForTurn(view.metadata.turn_id || '', 'manual', true)
  }

  const cancelEditing = () => {
    setEditingTurn(null)
    setInput('')
    setStyleSceneQuery(null)
    setShowSkillCommands(false)
    setSkillCommandQuery(null)
    setActiveSkillCommandIndex(0)
  }

  const handleInputChange = (nextValue: string) => {
    setInput(nextValue)
  }

  const handleInputTriggerChange = (trigger: ComposerTrigger | null) => {
    if (trigger?.kind === 'slash') {
      setSkillCommandQuery(trigger.query)
      setShowSkillCommands(true)
      setActiveSkillCommandIndex(0)
    } else {
      setSkillCommandQuery(null)
      setShowSkillCommands(false)
      setActiveSkillCommandIndex(0)
    }
    setStyleSceneQuery(trigger?.kind === 'style' ? trigger.query : null)
  }

  const selectSkillCommand = (name: string) => {
    const command = filteredSkillCommands.find((item) => item.name === name)
    if (command?.builtIn && name === 'goal') {
      inputRef.current?.replaceActiveTriggerText('')
      enterGoalMode()
    } else if (command?.builtIn) {
      inputRef.current?.replaceActiveTriggerText(`/${name} `)
    } else {
      inputRef.current?.replaceActiveTriggerWithToken({ kind: 'skill', value: name, label: name })
    }
    setShowSkillCommands(false)
    setSkillCommandQuery(null)
    setActiveSkillCommandIndex(0)
    inputRef.current?.focus()
  }

  const selectStyleScene = (scene: string) => {
    inputRef.current?.replaceActiveTriggerWithToken({ kind: 'style', value: scene, label: scene })
    setStyleScenes((current) => Array.from(new Set([...current, scene])))
    setStyleSceneQuery(null)
    inputRef.current?.focus()
  }

  const removeStyleScene = (scene: string) => {
    setStyleScenes((current) => current.filter((item) => item !== scene))
  }

  const handleTokenRemove = (token: ComposerTokenSpec) => {
    if (token.kind === 'style' && styleScenes.includes(token.value)) removeStyleScene(token.value)
  }

  const selectHotChoice = (choice: string) => {
    setInput(choice)
    setShowSkillCommands(false)
    setSkillCommandQuery(null)
    setActiveSkillCommandIndex(0)
    setHotChoicesExpanded(false)
    window.requestAnimationFrame(() => {
      inputRef.current?.focus()
      inputRef.current?.setSelectionRange(choice.length, choice.length)
    })
  }

  const saveEditedReply = async (narrative: string) => {
    if (!replyEditTarget) return
    await updateInteractiveTurnNarrative(storyId, replyEditTarget.turnId, {
      branch_id: replyEditTarget.branchId,
      narrative,
      expected_narrative: replyEditTarget.expectedNarrative,
    })
    await onDone({ silent: true })
  }

  const stageControls = (
    <>
      <StoryPicker stories={stories} currentStoryId={storyId} onSelect={(id) => { setStageControlsOpen(false); setCreatingStory(false); setEditingStorySetup(false); onStorySelect(id) }} onCreate={() => { setStageControlsOpen(false); setEditingStorySetup(false); setCreatingStory(true) }} onDeleteStories={onStoryDelete} />
      {isMobile ? <StoryDirectorPicker story={story} storyDirectors={storyDirectors} onChange={onDirectorChange} /> : null}
      {isMobile ? <ReplyTargetCharsControl story={story} onChange={onReplyTargetCharsChange} /> : null}
      {onToggleDirectorPanel && (
        <Button type="button" variant="outline" size="sm" className={`h-7 gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-[11px] hover:bg-[var(--nova-hover)] ${directorPanelVisible ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'}`} onClick={onToggleDirectorPanel} aria-label={directorPanelVisible ? t('storyStage.hideDirectorPanel') : t('storyStage.showDirectorPanel')}>
          <PanelRight className="h-3.5 w-3.5" />
          {t('storyStage.directorPanel')}
        </Button>
      )}
    </>
  )
  const openMobileNavigation = () => {
    window.dispatchEvent(new Event(MOBILE_NAVIGATION_OPEN_EVENT))
  }

  return (
    <main className="relative flex h-full min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-[var(--nova-surface-2)]">
      <div data-testid="story-stage-card" className="flex min-h-0 flex-1 flex-col overflow-hidden bg-[var(--nova-surface-2)]">
        <StoryStageHeader isMobile={isMobile} controlsOpen={stageControlsOpen} onControlsOpenChange={setStageControlsOpen} controls={stageControls} t={t} />

        <div className="nova-story-stage-content flex min-h-0 flex-1 overflow-hidden bg-[var(--nova-surface-2)]">
          <TurnNavigator items={turnNavigationItems} activeAnchorId={activeTurnAnchorId} onSelect={handleTurnNavigationSelect} />
          <section className="relative flex min-h-0 min-w-0 flex-1 flex-col bg-[var(--nova-surface-2)]">
            {historyWindow.stageKey === stageKey && !historyWindow.followLatest ? (
              <Button type="button" variant="secondary" size="sm" className="absolute right-4 top-3 z-30 shadow-md" onClick={resetHistoryToLatest}>
                {t('storyStage.history.backToLatest')}
              </Button>
            ) : null}
            {creatingStory ? (
              <NewStorySetupPanel
                stories={stories}
                tellers={tellers}
                directors={storyDirectors}
                imagePresets={imagePresets}
                recentNarrativeStyleID={recentNarrativeStyleID}
                narrativeStyleLoading={narrativeStyleLoading}
                story={editingStorySetup ? story : undefined}
                onNarrativeStyleChange={onNarrativeStyleChange}
                onCancel={() => { setCreatingStory(false); setEditingStorySetup(false) }}
                onCreate={async (input) => {
                  if (editingStorySetup) await onStorySetupUpdate(input)
                  else await onStoryCreate(input)
                  setCreatingStory(false)
                  setEditingStorySetup(false)
                }}
              />
            ) : snapshotLoading && agentMessages.length === 0 && !streaming ? (
              <LoadingState
                label={t('common.loading')}
                layout="conversation"
                className="h-full min-h-0 flex-1 bg-[var(--nova-surface-2)]"
              />
            ) : agentMessages.length === 0 && !streaming ? (
              <StoryOpeningPanel
                story={story}
                storyId={storyId}
                streaming={streaming}
                presets={availableBookOpeningPresets}
                selectedPreset={selectedBookOpeningPreset}
                customText={customOpeningText}
                bottomInset={inputFloatHeight}
                loreEmpty={loreEmpty}
                onSelectPreset={setSelectedBookOpeningPresetId}
                onCustomTextChange={setCustomOpeningText}
                onStartAI={startAIOpening}
                onStartPreset={startBookPresetOpening}
                onStartCustom={startOpening}
                onConfigureDirector={onOpenDirectorConfig}
                onRequestLoreInit={onRequestLoreInit}
                onBackToSetup={() => { setEditingStorySetup(true); setCreatingStory(true) }}
              />
            ) : (
              <MessageList
                projectId={projectId}
                messages={agentMessages}
                isStreaming={streaming}
                activityContent={stageRun.runtime.recoveryPaused ? t('storyStage.activity.recoveryPaused') : activityContent}
                highlightDialogue
                scrollResetKey={scrollResetKey}
                bottomPaddingClassName="pb-36"
                bottomPaddingPx={messageListBottomPadding}
                hasEarlierMessages={historyWindow.stageKey === stageKey && historyWindow.hasMore}
                isLoadingEarlierMessages={historyLoading}
                onLoadEarlierMessages={loadEarlierMessages}
                afterContent={historyWindow.followLatest && !streaming && storyStateModel.hasState && stateDisplayPreference !== 'director-only' ? (
                  <StoryStateLedger
                    snapshot={snapshot}
                    displayPreference={stateDisplayPreference}
                    onDisplayPreferenceChange={onStateDisplayPreferenceChange}
                    onOpenDirectorState={onOpenDirectorState}
                  />
                ) : undefined}
                afterContentKey={snapshot?.current_turn?.id || ''}
                messageStyle={stageTextStyle}
                collapseTraceGroups
                turnScrollRequest={turnScrollRequest}
                onVisibleTurnAnchorChange={handleVisibleTurnAnchorChange}
                canMutateMessage={canMutateStoryView}
                onEditMessage={startEditingView}
                onEditAssistantReply={storyImages.generatingTurnId || switchingVersionTurnId ? undefined : startEditingAssistantReply}
                onCreateBranch={onRequestCreateBranch ? startCreatingBranchFromView : undefined}
                onRegenerateMessage={regenerateView}
                onSwitchMessageVersion={switchViewVersion}
                onGenerateInteractiveImage={generateImageForView}
                generatingInteractiveImageTurnId={storyImages.generatingTurnId || undefined}
                onOpenSubAgentSession={openSubAgentSession}
                activeSubAgentSessionKey={activeSubAgentSessionKey}
                onOpenTrace={openTraceRun}
              />
            )}
            {activeSubAgentSessionKey && (
              <div className="absolute inset-y-0 right-0 z-30 w-[min(420px,92vw)] border-l border-[var(--nova-border)] shadow-[var(--nova-shadow)]">
                <AgentSubAgentSessionPanel
                  projectId={projectId}
                  messages={agentMessages}
                  sessionKey={activeSubAgentSessionKey}
                  onClose={() => setActiveSubAgentSessionKey('')}
                  highlightDialogue
                  messageStyle={stageTextStyle}
                />
              </div>
            )}
          </section>
        </div>
      </div>
      <StoryStageComposer
        layout={{ projectId, creatingStory, isMobile, keyboardInset, inputTextStyle, workspace, inputFloatRef, inputRef, t, attachmentDraftKey: stageKey }}
        editor={{ input, editingTurn, styleScenes, styleSceneQuery, styleSceneSuggestions, showSkillCommands, activeSkillCommandIndex, skillCommands, filteredSkillCommands, filteredBuiltInCommandItems, filteredSkillCommandItems, setStyleSceneQuery, setShowSkillCommands, setSkillCommandQuery, setActiveSkillCommandIndex }}
        story={{ storyId, story, imagePresets, onImageSettingsChange, branchTerminal, directorBlocking, directorPlanStatus, directorStatusVisible, directorRetrying, directorRetryError, hotChoices, hotChoicesExpanded, showHotChoices, canUseHotChoices, setHotChoicesExpanded }}
        runtime={{ streaming, approvalReady, conversationConfig, abortPending: stageRun.runtime.abortPending, recoveryPaused: stageRun.runtime.recoveryPaused, recoveryAbortAvailable: stageRun.runtime.recoveryAbortAvailable, operationId: stageRun.runtime.operationId, connection: stageRun.runtime.connection, commandSubmitting, queue: stageRun.runtime.queue, queueActionPendingCommandID }}
        goal={{ value: conversationGoal.goal, mode: goalMode, pending: conversationGoal.saving, enter: enterGoalMode, exit: () => setGoalMode(false), edit: editGoal, pause: pauseGoal, clear: clearGoal }}
        dialogs={{ contextAnalysisOpen, contextAnalysisLoading, contextAnalysisError, contextAnalysis, tokenUsageOpen, tokenUsageMessages, traceOpen, selectedTraceRunId, replyEditTarget, setContextAnalysisOpen, setTokenUsageOpen, setTraceOpen, closeReplyEditor: () => setReplyEditTarget(null), saveReply: saveEditedReply }}
        actions={{ cancelEditing, retryDirectorPlanning, selectHotChoice, selectStyleScene, selectSkillCommand, handleInputChange, handleInputTriggerChange, handleTokenRemove, toggleHotChoices, openMobileNavigation, openContextAnalysis, removeContextCompaction, openTraceRun, send: submitComposer, steerQueuedCommand, deleteQueuedCommand, stop }}
      />
    </main>
  )

}
function noop() {}
function noopStateDisplayPreferenceChange(_value: StoryStateDisplayPreference) {}

function noopTurnPersisted() {
  return undefined
}

function errorMessage(content: string) {
  return createAgentDataMessage({ type: 'agent-error', data: { role: 'error', content } })
}
