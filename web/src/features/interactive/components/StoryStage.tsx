import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import { Activity, Archive, BarChart3, Check, ChevronDown, ChevronUp, Command as CommandIcon, Compass, ImagePlus, List, Loader2, PanelRight, Pencil, Plus, RefreshCw, ScrollText, Send, SlidersHorizontal, Sparkles, Square, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Command, CommandEmpty, CommandGroup, CommandItem, CommandList } from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuSub, DropdownMenuSubContent, DropdownMenuSubTrigger, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { FileReferencePicker } from '@/components/Chat/FileReferencePicker'
import { CONTEXT_ANALYSIS_SIMULATED_MESSAGE, ContextAnalysisDialog } from '@/components/Chat/ContextAnalysisDialog'
import { MessageList, type TurnScrollRequest } from '@/components/Chat/MessageList'
import { AgentComposerShell } from '@/components/Chat/AgentComposerShell'
import { ModelProfileSwitcher } from '@/components/Chat/ModelProfileSwitcher'
import { TokenUsageDialog } from '@/components/Chat/TokenUsagePanel'
import { AgentTracePanel } from '@/components/Chat/AgentTracePanel'
import { AgentSubAgentSessionPanel } from '@/components/Chat/AgentSubAgentSessionPanel'
import { ComposerTokenInput, type ComposerTokenInputHandle, type ComposerTokenSpec, type ComposerTrigger } from '@/components/Chat/composer-token-input'
import { buildContextCompactionMessage, createContextCompactionMessageId, upsertContextCompactionMessage } from '@/components/Chat/context-compaction-message'
import { MOBILE_NAVIGATION_OPEN_EVENT } from '@/components/layout/workspace-mobile-layout'
import type { ChatMessage, ContextAnalysis, InteractiveImage, InteractiveImageError, PublicRuleRoll } from '@/lib/api'
import { chatMessagesToAgentUIMessages } from '@/lib/agent-legacy-message'
import { agentSubAgentSessionKey, agentViewToRenderMessage, type AgentMessageView } from '@/lib/agent-message-view'
import { fetchSettings } from '@/features/settings/api'
import { useSkillCommands } from '@/hooks/useSkillCommands'
import { abortInteractiveChat, analyzeInteractiveContext, compactInteractiveContext, generateInteractiveImage, removeInteractiveContextCompaction, runInteractiveDirector, sendInteractiveMessage, streamActiveInteractiveChat, switchInteractiveTurnVersion, updateInteractiveTurnNarrative } from '../api'
import type { ActiveInteractiveChat } from '../api'
import { createInteractiveNarrativeFilter, sanitizeStoredNarrative } from '../stream-parser'
import { emptyStoryStageRun, useInteractiveStore } from '../stores/interactive-store'
import type { StoryStageRunState } from '../stores/interactive-store'
import { buildOpeningPrompt, truncateStoryOpeningText, type BookOpeningPreset, type StoryCreateInput } from '../opening'
import type { ImagePreset, InteractiveSSEEvent, InteractiveTurnPersistedEvent, RuleResolution, Snapshot, StoryDirector, StoryImageSettings, StorySummary, Teller, TokenUsageEvent, TurnEvent } from '../types'
import { abortStoryRunStream, clearStoryRunAbortController, registerStoryRunAbortController, useActiveStoryRunRecovery } from '../use-active-story-run'
import { StoryPicker } from './StoryPicker'
import { NewStorySetupPanel } from './NewStorySetupPanel'
import { StoryOpeningPanel } from './StoryOpeningPanel'
import { StoryDirectorPicker } from './StoryDirectorPicker'
import { ReplyTargetCharsControl } from './ReplyTargetCharsControl'
import { TurnNavigator, type TurnNavigationItem } from './TurnNavigator'
import { isDirectorDisplayEvent } from './director-console/utils'
import { DEFAULT_STORY_STATE_DISPLAY, type StoryStateDisplayPreference } from './story-state/display-preference'
import { StoryStateLedger } from './story-state/StoryStateLedger'
import { buildStoryStateModel } from './story-state/model'
import { EditInteractiveReplyDialog } from './EditInteractiveReplyDialog'
import { appendBufferedLiveMessage, bindLiveToolEventKeys, findMappedLiveToolId, findToolMessageIndexForPayload, liveToolEventKeys, promoteMessageTarget, promoteMessageTargets, streamMetadataFromPayload, type BufferedLiveMessage } from './story-stage/live-stream-messages'
import { useIsMobile } from '@/hooks/useIsMobile'
import { useKeyboardInset } from '@/hooks/useKeyboardInset'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'

interface StoryStageProps {
  workspace?: string
  styleSceneSuggestions?: string[]
  stories?: StorySummary[]
  story?: StorySummary
  tellers?: Teller[]
  storyDirectors?: StoryDirector[]
  imagePresets?: ImagePreset[]
  storyId: string
  branchId: string
  snapshot: Snapshot | null
  snapshotLoading?: boolean
  loreEmpty?: boolean
  bookOpeningPresets?: BookOpeningPreset[]
  directorPanelVisible?: boolean
  stateDisplayPreference?: StoryStateDisplayPreference
  onStorySelect?: (storyId: string) => void
  onStoryCreate?: (input: StoryCreateInput) => void | Promise<void>
  onStorySetupUpdate?: (input: StoryCreateInput) => void | Promise<void>
  onStoryDelete?: (storyIds: string[]) => void | Promise<void>
  onDirectorChange?: (directorId: string) => void
  onReplyTargetCharsChange?: (replyTargetChars: number) => void | Promise<void>
  onImageSettingsChange?: (settings: StoryImageSettings) => void | Promise<void>
  onRequestLoreInit?: () => void
  onOpenDirectorConfig?: () => void
  onToggleDirectorPanel?: () => void
  onOpenDirectorState?: () => void
  onStateDisplayPreferenceChange?: (value: StoryStateDisplayPreference) => void
  onTurnPersisted?: (event: InteractiveTurnPersistedEvent) => Snapshot | void
  onDone: (options?: { silent?: boolean }) => void | Promise<Snapshot | void>
}

const DEFAULT_READING_FONT_SIZE = 18
const DEFAULT_STAGE_LINE_HEIGHT = 1.78
const EMPTY_STAGE_RUN = emptyStoryStageRun()
const DEFAULT_IMAGE_INTERVAL_TURNS = 3

type LiveTurnRenderKeys = {
  user: string
  assistant: string
}

type InteractiveStreamOutcome = {
  finishedNormally: boolean
  receivedPersistedTurn: boolean
  persistedSnapshot?: Snapshot
}

export function StoryStage({ workspace, styleSceneSuggestions = [], stories = [], story, tellers = [], storyDirectors = [], imagePresets = [], storyId, branchId, snapshot, snapshotLoading = false, loreEmpty = false, bookOpeningPresets = [], directorPanelVisible = true, stateDisplayPreference = DEFAULT_STORY_STATE_DISPLAY, onStorySelect = noop, onStoryCreate = noop, onStorySetupUpdate = noop, onStoryDelete = noop, onDirectorChange = noop, onReplyTargetCharsChange, onImageSettingsChange, onRequestLoreInit, onOpenDirectorConfig, onToggleDirectorPanel, onOpenDirectorState, onStateDisplayPreferenceChange = noopStateDisplayPreferenceChange, onTurnPersisted = noopTurnPersisted, onDone }: StoryStageProps) {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const keyboardInset = useKeyboardInset()
  const storyStateModel = useMemo(() => buildStoryStateModel(snapshot), [snapshot])
  const [input, setInput] = useState('')
  const [stageControlsOpen, setStageControlsOpen] = useState(false)
  const [styleScenes, setStyleScenes] = useState<string[]>([])
  const [styleSceneQuery, setStyleSceneQuery] = useState<string | null>(null)
  const [showSkillCommands, setShowSkillCommands] = useState(false)
  const [skillCommandQuery, setSkillCommandQuery] = useState<string | null>(null)
  const [activeSkillCommandIndex, setActiveSkillCommandIndex] = useState(0)
  const [inputFloatHeight, setInputFloatHeight] = useState(0)
  const [optimisticInteractiveImages, setOptimisticInteractiveImages] = useState<Record<string, InteractiveImage[]>>({})
  const inputRef = useRef<ComposerTokenInputHandle | null>(null)
  const inputFloatRef = useRef<HTMLDivElement | null>(null)
  const skillCommandRefs = useRef<Array<HTMLDivElement | null>>([])
  const skillCommands = useSkillCommands({
    agentKey: 'interactive_story',
    workspace,
    fallbackEnabled: true,
  })
  const snapshotKey = storyStageSnapshotKey(storyId, branchId, snapshot)
  const stageKey = `${workspace || 'current'}:${storyId || 'none'}:${branchId || snapshot?.branch_id || 'main'}`
  const { storyStageRuns, setStoryStageRun, clearStoryStageRun } = useInteractiveStore()
  const stageRun = storyStageRuns[stageKey] || EMPTY_STAGE_RUN
  const streaming = stageRun.streaming
  const activityContent = stageRun.activityContent
  const liveMessages = stageRun.liveMessages
  const rewindTurnId = stageRun.rewindTurnId
  const branchTerminal = snapshot?.current_turn?.terminal_outcome?.terminal === true

  useEffect(() => {
    setOptimisticInteractiveImages({})
  }, [stageKey])
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
  const [generatingImageTurnId, setGeneratingImageTurnId] = useState<string | null>(null)
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
  const currentCompactionMessageIdRef = useRef<string | null>(null)
  const compactionIdCounterRef = useRef(0)

  useEffect(() => {
    setReplyEditTarget(null)
  }, [stageKey])
  const liveMessageBufferRef = useRef<BufferedLiveMessage[]>([])
  const liveMessageRafRef = useRef<number | null>(null)
  const liveMessagePromoteRafRef = useRef<number | null>(null)
  const liveToolKeyToMessageIdRef = useRef<Record<string, string>>({})
  const nonNarrativeLiveMessageStreamingRef = useRef(false)
  const liveStageKeyRef = useRef(stageKey)
  const currentLiveTurnRenderKeysRef = useRef<LiveTurnRenderKeys | null>(null)
  const turnRenderKeysRef = useRef<Record<string, LiveTurnRenderKeys>>({})
  const previousSnapshotKeyRef = useRef(snapshotKey)
  const liveTurnNavigationAnchorId = useMemo(() => `live:${stageKey}`, [stageKey])
  const stagePreferences = useStagePreferences()
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
    (updater: ChatMessage[] | ((current: ChatMessage[]) => ChatMessage[])) => {
      updateStageRun((current) => ({
        ...current,
        liveMessages: typeof updater === 'function' ? updater(current.liveMessages) : updater,
      }))
    },
    [updateStageRun],
  )

  const latestLiveTurn = useMemo(() => {
    if (liveMessages.length === 0) return null
    const user = liveMessages.find((msg) => msg.role === 'user')?.content || ''
    const narrative = liveMessages
      .filter((msg) => msg.role === 'assistant' && !msg.subagent)
      .map((msg) => msg.streaming_target_content || msg.content || '')
      .join('')
    if (!user && !narrative) return null
    return { user, narrative }
  }, [liveMessages])

  const hasPersistedLiveTurn = useMemo(() => {
    const lastTurn = snapshot?.turns?.[snapshot.turns.length - 1]
    if (!lastTurn || !latestLiveTurn) return false
    if (liveStageKeyRef.current !== stageKey) return false
    return normalizeMessageContent(lastTurn.user) === normalizeMessageContent(latestLiveTurn.user) && normalizeMessageContent(lastTurn.narrative) === normalizeMessageContent(latestLiveTurn.narrative)
  }, [latestLiveTurn, snapshot?.turns, stageKey])
  const filteredSkillCommands = useMemo(() => {
    if (skillCommandQuery === null) return []
    const query = skillCommandQuery.toLowerCase()
    const seen = new Set(['compact'])
    const commands = [
      {
        name: 'compact',
        description: t('chat.command.compact.desc'),
        hint: t('chat.command.compact.hint'),
        builtIn: true,
      },
      ...skillCommands
        .filter((skill) => {
          if (seen.has(skill.name)) return false
          seen.add(skill.name)
          return true
        })
        .map((skill) => ({
          name: skill.name,
          description: skill.description || skill.name,
          hint: t('chat.command.skill.hint'),
          builtIn: false,
        })),
    ]
    return commands.filter((skill) => skill.name.toLowerCase().startsWith(query))
  }, [skillCommandQuery, skillCommands, t])
  const filteredBuiltInCommandItems = useMemo(() => filteredSkillCommands
    .map((command, index) => ({ command, index }))
    .filter(({ command }) => command.builtIn), [filteredSkillCommands])
  const filteredSkillCommandItems = useMemo(() => filteredSkillCommands
    .map((command, index) => ({ command, index }))
    .filter(({ command }) => !command.builtIn), [filteredSkillCommands])

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
    return () => {
      if (liveMessageRafRef.current !== null) {
        window.cancelAnimationFrame(liveMessageRafRef.current)
        liveMessageRafRef.current = null
      }
      if (liveMessagePromoteRafRef.current !== null) {
        window.cancelAnimationFrame(liveMessagePromoteRafRef.current)
        liveMessagePromoteRafRef.current = null
      }
      liveMessageBufferRef.current = []
    }
  }, [])

  useEffect(() => {
    if (activeSkillCommandIndex >= filteredSkillCommands.length) setActiveSkillCommandIndex(0)
  }, [activeSkillCommandIndex, filteredSkillCommands.length])

  useEffect(() => {
    if (!showSkillCommands || filteredSkillCommands.length === 0) return
    skillCommandRefs.current[activeSkillCommandIndex]?.scrollIntoView({
      block: 'nearest',
    })
  }, [activeSkillCommandIndex, filteredSkillCommands.length, showSkillCommands])

  const storyPathTurns = useMemo(() => {
    const turns = snapshot?.turns || []
    const rewindIndex = rewindTurnId ? turns.findIndex((turn) => turn.id === rewindTurnId) : -1
    return rewindIndex >= 0 ? turns.slice(0, rewindIndex) : turns
  }, [rewindTurnId, snapshot?.turns])
  const publicRuleRollVisible = useMemo(
    () => storyRuleVisibilityMode(story, storyDirectors) === 'public_roll',
    [story, storyDirectors],
  )

  const historyMessages = useMemo<ChatMessage[]>(() => {
    return storyPathTurns.flatMap((turn) => {
      const messages: ChatMessage[] = [
        {
          id: `${turn.id}-user`,
          render_key: turnRenderKeysRef.current[turn.id]?.user,
          turn_id: turn.id,
          navigation_turn_id: turn.id,
          role: 'user',
          content: turn.user,
        },
      ]
      const displayEvents = (turn.display_events || []).filter((event) => !isDirectorDisplayEvent(event))
      const hasDisplayTimelineThinking = displayEvents.some((event) => event.role === 'thinking')
      if (!hasDisplayTimelineThinking && turn.thinking?.trim()) {
        messages.push({
          id: `${turn.id}-thinking`,
          role: 'thinking',
          content: turn.thinking,
          streaming: false,
        })
      }
      const deferredImageMessages: ChatMessage[] = []
      // narrative 锚点标记正文在事件流中的真实位置：锚点前的思考/工具留在正文之前，
      // 提交结果等锚点后事件渲染在正文之后；旧回合没有锚点，正文兜底排在最后。
      const preNarrativeMessages: ChatMessage[] = []
      const postNarrativeMessages: ChatMessage[] = []
      let narrativeAnchored = false
      for (const [index, event] of displayEvents.entries()) {
        if (event.role === 'narrative') {
          narrativeAnchored = true
          continue
        }
        const timeline = narrativeAnchored ? postNarrativeMessages : preNarrativeMessages
        if (event.role === 'thinking') {
          timeline.push({
            id: event.id || `${turn.id}-thinking-${index}`,
            role: 'thinking',
            content: event.content || '',
            streaming: false,
            created_at: event.created_at,
            run_id: event.run_id,
            agent_kind: event.agent_kind,
            agent_name: event.agent_name,
            root_agent_name: event.root_agent_name,
            run_path: event.run_path,
            subagent: event.subagent,
            subagent_session_id: event.subagent_session_id,
            subagent_type: event.subagent_type,
          })
          continue
        }
        if (event.role === 'tool_call') {
          const toolMessage: ChatMessage = {
            id: event.id || `${turn.id}-tool-${index}`,
            turn_id: event.name === 'generate_interactive_image' ? turn.id : undefined,
            role: 'tool_call',
            content: event.content || event.name || 'unknown_tool',
            name: event.name || event.content,
            args: event.args || '',
            status: event.status || 'success',
            result: event.result || '',
            interactive_image: readInteractiveImage(event.result),
            interactive_image_error: readInteractiveImageError(event.result),
            streaming: false,
            created_at: event.created_at,
            run_id: event.run_id,
            agent_kind: event.agent_kind,
            agent_name: event.agent_name,
            root_agent_name: event.root_agent_name,
            run_path: event.run_path,
            subagent: event.subagent,
            subagent_session_id: event.subagent_session_id,
            subagent_type: event.subagent_type,
            sse_hidden_fields: event.sse_hidden_fields,
            sse_hidden_reason: event.sse_hidden_reason,
            sse_display_notice: event.sse_display_notice,
            sse_generated_chars: event.sse_generated_chars,
          }
          if (event.name === 'generate_interactive_image') {
            deferredImageMessages.push(toolMessage)
          } else {
            timeline.push(toolMessage)
          }
          continue
        }
        if (event.role === 'assistant') {
          timeline.push({
            id: event.id || `${turn.id}-subagent-${index}`,
            role: 'assistant',
            content: event.content || '',
            streaming: false,
            created_at: event.created_at,
            run_id: event.run_id,
            agent_kind: event.agent_kind,
            agent_name: event.agent_name,
            root_agent_name: event.root_agent_name,
            run_path: event.run_path,
            subagent: event.subagent,
            subagent_session_id: event.subagent_session_id,
            subagent_type: event.subagent_type,
          })
        }
      }
      messages.push(...preNarrativeMessages)
      const ruleRoll = publicRuleRollVisible ? publicRuleRollFromResolution(turn.rule_resolution) : null
      if (ruleRoll) {
        messages.push({
          id: `${turn.id}-rule-roll`,
          turn_id: turn.id,
          navigation_turn_id: turn.id,
          role: 'rule_roll',
          rule_roll: ruleRoll,
        })
      }
      const mergedImages = mergeInteractiveImages(interactiveImages(deferredImageMessages), optimisticInteractiveImages[turn.id])
      messages.push({
        id: `${turn.id}-assistant`,
        render_key: turnRenderKeysRef.current[turn.id]?.assistant,
        turn_id: turn.id,
        navigation_turn_id: turn.id,
        role: 'assistant',
        content: sanitizeStoredNarrative(turn.narrative),
        run_id: turn.run_id,
        agent_kind: turn.agent_kind,
        turn_versions: turn.versions,
        turn_version_index: turn.version_idx,
        interactive_image: latestMergedInteractiveImage(mergedImages),
        interactive_images: mergedImages,
        interactive_image_error: latestInteractiveImageError(deferredImageMessages),
        interactive_image_status: mergedImages?.length ? 'success' : latestInteractiveImageStatus(deferredImageMessages),
      })
      messages.push(...postNarrativeMessages)
      return messages
    })
  }, [optimisticInteractiveImages, publicRuleRollVisible, storyPathTurns])

  const displayLiveMessages = hasPersistedLiveTurn ? [] : liveMessages.filter((message) => message.role !== 'token_usage')
  const messages = useMemo(() => [...historyMessages, ...displayLiveMessages], [displayLiveMessages, historyMessages])
  const agentMessages = useMemo(() => chatMessagesToAgentUIMessages(messages), [messages])
  const turnNavigationItems = useMemo<TurnNavigationItem[]>(() => {
    const items: TurnNavigationItem[] = storyPathTurns.map((turn) => ({
      anchorId: turn.id,
      user: turn.user,
      narrative: sanitizeStoredNarrative(turn.narrative),
    }))
    if (!hasPersistedLiveTurn && latestLiveTurn) {
      items.push({
        anchorId: liveTurnNavigationAnchorId,
        user: latestLiveTurn.user,
        narrative: latestLiveTurn.narrative,
        pending: streaming || !latestLiveTurn.narrative.trim(),
      })
    }
    return items
  }, [hasPersistedLiveTurn, latestLiveTurn, liveTurnNavigationAnchorId, storyPathTurns, streaming])
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
  const persistedTokenUsageMessages = useMemo(
    () => (snapshot?.token_usage_events || []).map((event, index) => buildTokenUsageMessage(event, event.id || `token-usage-${index + 1}`)),
    [snapshot?.token_usage_events],
  )
  const liveTokenUsageMessages = useMemo(
    () => liveMessages.filter((message) => message.role === 'token_usage'),
    [liveMessages],
  )
  const tokenUsageMessages = useMemo(
    () => mergeTokenUsageMessages(persistedTokenUsageMessages, liveTokenUsageMessages),
    [liveTokenUsageMessages, persistedTokenUsageMessages],
  )
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
  const availableBookOpeningPresets = useMemo(() => bookOpeningPresets.filter((preset) => preset.content.trim()), [bookOpeningPresets])
  const selectedBookOpeningPreset = useMemo(
    () => availableBookOpeningPresets.find((preset) => preset.id === selectedBookOpeningPresetId) || availableBookOpeningPresets[0] || null,
    [availableBookOpeningPresets, selectedBookOpeningPresetId],
  )
  const turnsById = useMemo(() => {
    const result = new Map<string, TurnEvent>()
    for (const turn of snapshot?.turns || []) {
      result.set(turn.id, turn)
    }
    return result
  }, [snapshot?.turns])

  const syncInputFloatHeight = useCallback(() => {
    const element = inputFloatRef.current
    if (!element) return
    const nextHeight = Math.ceil(element.getBoundingClientRect().height)
    setInputFloatHeight((current) => (current === nextHeight ? current : nextHeight))
  }, [])

  useLayoutEffect(() => {
    syncInputFloatHeight()
  }, [directorRetryError, directorRetrying, directorStatusVisible, editingTurn, hotChoices.length, input, showHotChoices, syncInputFloatHeight])

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

  useActiveStoryRunRecovery({
    stageKey,
    storyId,
    branchId,
    isStreaming: () => Boolean(useInteractiveStore.getState().storyStageRuns[stageKey]?.streaming),
    onResume: resumeActiveStoryRun,
    onDetach: () => updateStageRun({ streaming: false, activityContent: '' }),
  })

  const send = async (override?: { message?: string; rewindTurnId?: string }) => {
    const sourceMessage = override?.message ?? input
    const message = sourceMessage.trim()
    if (!message || !storyId || streaming || branchTerminal || directorBlocking) return
    if (message === '/compact') {
      await compactCurrentContext()
      return
    }
    const nextRewindTurnId = override?.rewindTurnId ?? editingTurn?.id
    const inlineStyleScenes = parseInlineStyleScenes(message)
    const mergedStyleScenes = Array.from(new Set([...styleScenes, ...inlineStyleScenes]))
    setInput('')
    setEditingTurn(null)
    setStyleScenes([])
    setStyleSceneQuery(null)
    setShowSkillCommands(false)
    setSkillCommandQuery(null)
    setActiveSkillCommandIndex(0)
    prepareLiveStoryRun(message, nextRewindTurnId)
    const abortController = new AbortController()
    registerStoryRunAbortController(stageKey, abortController)
    try {
      const stream = await sendInteractiveMessage({
        mode: 'story',
        story_id: storyId,
        branch: branchId,
        message,
        style_scenes: mergedStyleScenes,
        regenerate_from_turn_id: nextRewindTurnId || undefined,
        signal: abortController.signal,
      })
      await completeInteractiveStream(await consumeInteractiveStream(stream))
    } catch (error) {
      handleInteractiveStreamError(error)
    } finally {
      finishLiveStoryRun(abortController)
    }
  }

  async function resumeActiveStoryRun(active: ActiveInteractiveChat, abortController: AbortController, isDisposed: () => boolean) {
    const message = active.message?.trim() || ''
    if (!message) return
    prepareLiveStoryRun(message, active.regenerate_from_turn_id)
    try {
      const stream = await streamActiveInteractiveChat({
        storyId,
        branchId,
        taskId: active.task_id,
        signal: abortController.signal,
      })
      if (isDisposed()) return
      await completeInteractiveStream(await consumeInteractiveStream(stream))
    } catch (error) {
      if (!isDisposed()) handleInteractiveStreamError(error)
    } finally {
      finishLiveStoryRun(abortController)
    }
  }

  function prepareLiveStoryRun(message: string, nextRewindTurnId?: string) {
    setStageActivityContent(t('storyStage.activity.thinking'))
    flushLiveMessageBuffer()
    liveToolKeyToMessageIdRef.current = {}
    nonNarrativeLiveMessageStreamingRef.current = false
    const liveTurnRenderKeys = createLiveTurnRenderKeys()
    currentLiveTurnRenderKeysRef.current = liveTurnRenderKeys
    setStageLiveMessages([{ role: 'user', content: message, render_key: liveTurnRenderKeys.user, navigation_turn_id: liveTurnNavigationAnchorId }])
    currentCompactionMessageIdRef.current = null
    updateStageRun({ rewindTurnId: nextRewindTurnId || undefined, retryMessage: message })
    liveStageKeyRef.current = stageKey
    setStageStreaming(true)
  }

  async function consumeInteractiveStream(stream: ReadableStream<InteractiveSSEEvent>): Promise<InteractiveStreamOutcome> {
    const narrativeFilter = createInteractiveNarrativeFilter()
    let finishedNormally = false
    let streamFailed = false
    let receivedPersistedTurn = false
    let persistedSnapshot: Snapshot | undefined
    const reader = stream.getReader()
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      switch (value.event) {
        case 'chunk': {
          const data = JSON.parse(value.data)
          if (data.subagent) {
            appendAssistantMessage(data.content || '', streamMetadataFromPayload(data))
            setStageActivityContent('')
            break
          }
          const { text, reset } = narrativeFilter.push(data.content || '')
          if (reset) resetAssistantMessage()
          if (text) {
            collapseNonNarrativeMessages()
            appendAssistantMessage(text)
          }
          setStageActivityContent('')
          break
        }
        case 'thinking': {
          const data = JSON.parse(value.data)
          appendThinkingMessage(data.content || '', streamMetadataFromPayload(data))
          setStageActivityContent(t('storyStage.activity.thinking'))
          break
        }
        case 'interactive_content_reclassified': {
          const data = JSON.parse(value.data)
          resetAssistantMessage()
          appendThinkingMessage(data.content || '', streamMetadataFromPayload(data))
          setStageActivityContent(t('storyStage.activity.thinking'))
          break
        }
        case 'tool_call': {
          const data = JSON.parse(value.data)
          flushLiveMessageBuffer()
          appendToolCallMessage(data)
          setStageActivityContent(t('storyStage.activity.processingTool', {
            name: data.name || t('storyStage.activity.toolCall'),
          }))
          break
        }
        case 'tool_args_delta': {
          const data = JSON.parse(value.data)
          flushLiveMessageBuffer()
          appendToolArgsDelta(data)
          break
        }
        case 'tool_result': {
          const data = JSON.parse(value.data)
          flushLiveMessageBuffer()
          updateToolCallMessage(data, 'success', data.content || '')
          appendLiveRuleRollMessage(data)
          setStageActivityContent('')
          break
        }
        case 'context_compaction': {
          const data = JSON.parse(value.data)
          flushLiveMessageBuffer()
          appendContextCompactionMessage(data)
          setStageActivityContent('')
          if (data.status === 'completed' || data.status === 'failed') currentCompactionMessageIdRef.current = null
          break
        }
        case 'token_usage': {
          const data = JSON.parse(value.data)
          flushLiveMessageBuffer()
          setStageLiveMessages((prev) => upsertTokenUsageMessage(prev, buildTokenUsageMessage(data)))
          break
        }
        case 'interactive_turn_persisted': {
          const data = JSON.parse(value.data) as InteractiveTurnPersistedEvent
          flushLiveMessageBuffer()
          receivedPersistedTurn = true
          if (data.turn?.id && currentLiveTurnRenderKeysRef.current) {
            turnRenderKeysRef.current[data.turn.id] = currentLiveTurnRenderKeysRef.current
          }
          persistedSnapshot = onTurnPersisted(data) || persistedSnapshot
          setStageActivityContent('')
          break
        }
        case 'error': {
          const data = JSON.parse(value.data)
          flushLiveMessageBuffer()
          finishLiveMessages()
          setStageActivityContent('')
          streamFailed = true
          setStageLiveMessages((prev) => [
            ...prev,
            { role: 'error', content: data.message || data.error || t('storyStage.activity.unknownError') },
          ])
          break
        }
        case 'done': {
          const { text, reset } = narrativeFilter.flush()
          if (reset) resetAssistantMessage()
          collapseNonNarrativeMessages()
          if (text) appendAssistantMessage(text)
          finishLiveMessages()
          if (!receivedPersistedTurn && !streamFailed) {
            streamFailed = true
            setStageLiveMessages([{ role: 'error', content: t('storyStage.activity.persistenceMissing') }])
          } else if (!streamFailed) {
            finishedNormally = true
          }
          setStageActivityContent('')
          break
        }
        case 'aborted': {
          const { text, reset } = narrativeFilter.flush()
          if (reset) resetAssistantMessage()
          collapseNonNarrativeMessages()
          if (text) appendAssistantMessage(text)
          finishLiveMessages()
          setStageLiveMessages((prev) => [
            ...prev,
            { role: 'error', content: t('storyStage.activity.aborted') },
          ])
          setStageActivityContent('')
          break
        }
      }
    }
    return { finishedNormally, receivedPersistedTurn, persistedSnapshot }
  }

  async function completeInteractiveStream({ finishedNormally, receivedPersistedTurn, persistedSnapshot }: InteractiveStreamOutcome) {
    let nextSnapshot: Snapshot | void = persistedSnapshot
    if (persistedSnapshot) {
      void Promise.resolve(onDone({ silent: true })).catch((error) => {
        console.warn('[interactive-stage] 静默刷新互动快照失败', error)
      })
    } else {
      nextSnapshot = await onDone(receivedPersistedTurn ? { silent: true } : undefined)
    }
    if (finishedNormally) await maybeGenerateAutoImage(nextSnapshot)
  }

  function handleInteractiveStreamError(error: unknown) {
    flushLiveMessageBuffer()
    finishLiveMessages()
    setStageActivityContent('')
    setStageLiveMessages((prev) => [
      ...prev,
      {
        role: 'error',
        content: isAbortError(error)
          ? t('storyStage.activity.aborted')
          : error instanceof Error ? error.message : t('storyStage.activity.runFailed'),
      },
    ])
  }

  function finishLiveStoryRun(abortController: AbortController) {
    if (!clearStoryRunAbortController(stageKey, abortController)) return
    setStageStreaming(false)
    liveToolKeyToMessageIdRef.current = {}
    currentCompactionMessageIdRef.current = null
    currentLiveTurnRenderKeysRef.current = null
    setStageActivityContent('')
  }

  const compactCurrentContext = async () => {
    if (!storyId || streaming) return
    setInput('')
    setEditingTurn(null)
    setStyleScenes([])
    setStyleSceneQuery(null)
    setShowSkillCommands(false)
    setActiveSkillCommandIndex(0)
    setStageStreaming(true)
    setStageActivityContent('')
    currentCompactionMessageIdRef.current = null
    setStageLiveMessages([{
      role: 'context_compaction',
      id: createContextCompactionMessageId(compactionIdCounterRef),
      status: 'running',
      content: '',
      phase: 'pre_run',
      streaming: true,
    }])
    try {
      await compactInteractiveContext(storyId, branchId)
      setStageLiveMessages((prev) => [
        ...prev.map((msg) => msg.role === 'context_compaction' ? { ...msg, status: 'success' as const, streaming: false } : msg),
        { role: 'system', content: t('storyStage.contextCompaction.done') },
      ])
      await onDone()
    } catch (error) {
      setStageLiveMessages((prev) => [
        ...prev.map((msg) => msg.role === 'context_compaction' ? { ...msg, status: 'error' as const, streaming: false } : msg),
        { role: 'error', content: error instanceof Error ? error.message : t('storyStage.contextCompaction.failed') },
      ])
    } finally {
      setStageStreaming(false)
      currentCompactionMessageIdRef.current = null
      setStageActivityContent('')
    }
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

  const stop = () => {
    void abortInteractiveChat()
    abortStoryRunStream(stageKey)
    setStageActivityContent(t('storyStage.activity.aborting'))
  }

  const startEditingMessage = (message: ChatMessage) => {
    if (!message.turn_id || streaming) return
    setEditingTurn({ id: message.turn_id, content: message.content || '' })
    setInput(message.content || '')
    setShowSkillCommands(false)
    setActiveSkillCommandIndex(0)
    window.requestAnimationFrame(() => {
      const length = message.content?.length || 0
      inputRef.current?.focus()
      inputRef.current?.setSelectionRange(length, length)
    })
  }

  const regenerateMessage = (message: ChatMessage) => {
    if (streaming) return
    if (!message.turn_id) {
      const source = stageRun.retryMessage || [...liveMessages].reverse().find((item) => item.role === 'user')?.content || ''
      if (source.trim()) void send({ message: source })
      return
    }
    const source = turnsById.get(message.turn_id)?.user || message.content || ''
    void send({ message: source, rewindTurnId: message.turn_id })
  }

  const rememberGeneratedInteractiveImage = useCallback((image?: InteractiveImage) => {
    if (!image?.turn_id || !image.image_path) return
    setOptimisticInteractiveImages((prev) => {
      const images = prev[image.turn_id] || []
      if (images.some((item) => item.image_path === image.image_path)) return prev
      return { ...prev, [image.turn_id]: [...images, image] }
    })
  }, [])

  const generateImageForMessage = useCallback(
    async (message: ChatMessage, source: 'manual' | 'auto' = 'manual', force = true) => {
      if (!message.turn_id || !storyId || generatingImageTurnId) return null
      setGeneratingImageTurnId(message.turn_id)
      setStageActivityContent(t('storyStage.interactiveImage.generating'))
      try {
        const result = await generateInteractiveImage(storyId, {
          branch_id: branchId || snapshot?.branch_id,
          turn_id: message.turn_id,
          source,
          force,
        })
        rememberGeneratedInteractiveImage(result.image)
        await onDone({ silent: true })
        return result
      } catch (error) {
        setStageLiveMessages((prev) => [
          ...prev,
          {
            role: 'error',
            content: error instanceof Error ? error.message : t('storyStage.interactiveImage.generateFailed'),
          },
        ])
        return null
      } finally {
        setGeneratingImageTurnId(null)
        setStageActivityContent('')
      }
    },
    [branchId, generatingImageTurnId, onDone, rememberGeneratedInteractiveImage, setStageActivityContent, setStageLiveMessages, snapshot?.branch_id, storyId, t],
  )

  const maybeGenerateAutoImage = useCallback(
    async (nextSnapshot: Snapshot | void) => {
      const targetSnapshot = nextSnapshot || snapshot
      const turn = targetSnapshot?.turns?.[targetSnapshot.turns.length - 1]
      if (!turn || !storyId) return
      const result = await generateInteractiveImage(storyId, {
        branch_id: targetSnapshot.branch_id || branchId,
        turn_id: turn.id,
        source: 'auto',
        force: false,
      }).catch((error) => {
        console.warn('[interactive-stage] 自动生成互动图像失败', error)
        return null
      })
      if (result && !result.skipped) {
        rememberGeneratedInteractiveImage(result.image)
        await onDone({ silent: true })
      }
    },
    [branchId, onDone, rememberGeneratedInteractiveImage, snapshot, storyId],
  )

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

  const switchMessageVersion = async (message: ChatMessage, direction: -1 | 1) => {
    if (!message.turn_id || !storyId || streaming || switchingVersionTurnId) return
    const versions = message.turn_versions || []
    const currentIndex = message.turn_version_index ?? versions.findIndex((version) => version.current)
    const nextVersion = versions[currentIndex + direction]
    if (!nextVersion) return
    setSwitchingVersionTurnId(message.turn_id)
    setStageActivityContent(direction > 0 ? t('storyStage.activity.switchNewer') : t('storyStage.activity.switchOlder'))
    try {
      await switchInteractiveTurnVersion(storyId, {
        branch_id: branchId,
        turn_id: message.turn_id,
        version_turn_id: nextVersion.turn_id,
      })
      clearStoryStageRun(stageKey)
      await onDone()
    } catch (error) {
      setStageLiveMessages((prev) => [
        ...prev,
        {
          role: 'error',
          content: error instanceof Error ? error.message : t('storyStage.activity.switchFailed'),
        },
      ])
    } finally {
      setSwitchingVersionTurnId(null)
      setStageActivityContent('')
    }
  }

  const startEditingView = (view: AgentMessageView) => {
    const message = agentViewToRenderMessage(view)
    if (message) startEditingMessage(message)
  }

  const startEditingAssistantReply = (view: AgentMessageView) => {
    if (streaming || generatingImageTurnId || switchingVersionTurnId) return
    const message = agentViewToRenderMessage(view)
    if (!message?.turn_id) return
    const turn = turnsById.get(message.turn_id)
    if (!turn) return
    setReplyEditTarget({
      turnId: turn.id,
      branchId: turn.branch_id || branchId,
      initialContent: sanitizeStoredNarrative(turn.narrative),
      expectedNarrative: turn.narrative,
    })
  }

  const regenerateView = (view: AgentMessageView) => {
    const message = agentViewToRenderMessage(view)
    if (message) regenerateMessage(message)
  }

  const switchViewVersion = (view: AgentMessageView, direction: -1 | 1) => {
    const message = agentViewToRenderMessage(view)
    if (message) void switchMessageVersion(message, direction)
  }

  const generateImageForView = (view: AgentMessageView) => {
    const message = agentViewToRenderMessage(view)
    if (message) void generateImageForMessage(message, 'manual', true)
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
    if (command?.builtIn) {
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

  const stageControls = (
    <>
      <StoryPicker stories={stories} currentStoryId={storyId} onSelect={(id) => { setStageControlsOpen(false); setCreatingStory(false); setEditingStorySetup(false); onStorySelect(id) }} onCreate={() => { setStageControlsOpen(false); setEditingStorySetup(false); setCreatingStory(true) }} onDeleteStories={onStoryDelete} />
      {isMobile ? <StoryDirectorPicker story={story} storyDirectors={storyDirectors} onChange={onDirectorChange} /> : null}
      {isMobile ? <ReplyTargetCharsControl story={story} onChange={onReplyTargetCharsChange} /> : null}
      {onToggleDirectorPanel && (
        <Button type="button" variant="outline" size="sm" className={`h-7 gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-[11px] hover:bg-[var(--nova-hover)] ${directorPanelVisible ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'}`} onClick={onToggleDirectorPanel} aria-label={directorPanelVisible ? t('storyStage.hideDirectorPanel') : t('storyStage.showDirectorPanel')} title={directorPanelVisible ? t('storyStage.hideDirectorPanel') : t('storyStage.showDirectorPanel')}>
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
        {isMobile ? (
          <div className="pointer-events-none absolute inset-x-0 top-3 z-10 px-3">
            <div className={`pointer-events-auto ml-auto overflow-hidden rounded-[14px] border border-[var(--nova-border)] bg-[var(--nova-surface)]/85 text-[var(--nova-text)] shadow-[0_12px_36px_rgba(0,0,0,0.28)] backdrop-blur-xl transition-[max-height,width,background-color] duration-200 ease-[var(--nova-ease)] ${stageControlsOpen ? 'w-[min(calc(100vw-1.5rem),390px)] max-h-[48dvh]' : 'w-8 max-h-8'}`}>
              <button type="button" className="flex h-8 w-full items-center gap-2 px-2 text-left text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('storyStage.mobile.controls')} aria-expanded={stageControlsOpen} title={t('storyStage.mobile.controls')} onClick={() => setStageControlsOpen((open) => !open)}>
                <span className="flex h-4 w-4 shrink-0 items-center justify-center">
                  <SlidersHorizontal className="h-3.5 w-3.5" />
                </span>
                {stageControlsOpen ? <span className="min-w-0 flex-1 truncate text-xs font-semibold text-[var(--nova-text)]">{t('storyStage.mobile.controls')}</span> : null}
                {stageControlsOpen ? <X className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" /> : null}
              </button>
              {stageControlsOpen ? (
                <div className="border-t border-[var(--nova-border)] px-3 pb-3 pt-2">
                  <div className="flex max-h-[calc(48dvh-3rem)] flex-col gap-2 overflow-y-auto pr-1">
                    {stageControls}
                  </div>
                </div>
              ) : null}
            </div>
          </div>
        ) : (
          <div className="nova-story-stage-header nova-topbar flex min-h-12 flex-wrap items-center justify-start gap-3 border-b px-4 py-2">
            <div className="nova-story-stage-controls flex min-w-0 flex-wrap items-center justify-start gap-2">
              {stageControls}
            </div>
          </div>
        )}

        <div className="nova-story-stage-content flex min-h-0 flex-1 overflow-hidden bg-[var(--nova-surface-2)]">
          <TurnNavigator items={turnNavigationItems} activeAnchorId={activeTurnAnchorId} onSelect={handleTurnNavigationSelect} />
          <section className="relative flex min-h-0 min-w-0 flex-1 flex-col bg-[var(--nova-surface-2)]">
            {creatingStory ? (
              <NewStorySetupPanel
                stories={stories}
                tellers={tellers}
                directors={storyDirectors}
                imagePresets={imagePresets}
                story={editingStorySetup ? story : undefined}
                onCancel={() => { setCreatingStory(false); setEditingStorySetup(false) }}
                onCreate={async (input) => {
                  if (editingStorySetup) await onStorySetupUpdate(input)
                  else await onStoryCreate(input)
                  setCreatingStory(false)
                  setEditingStorySetup(false)
                }}
              />
            ) : snapshotLoading && messages.length === 0 && !streaming ? (
              <div className="m-5 flex min-h-0 flex-1 items-center justify-center rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-6 text-center text-sm text-[var(--nova-text-faint)] shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]">
                <div className="flex max-w-md flex-col items-center gap-3">
                  <RefreshCw className="h-4 w-4 animate-spin text-[var(--nova-text-muted)]" />
                  <div className="text-xs leading-5 text-[var(--nova-text-faint)]">{t('common.loading')}</div>
                </div>
              </div>
            ) : messages.length === 0 && !streaming ? (
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
                messages={agentMessages}
                isStreaming={streaming}
                activityContent={activityContent}
                highlightDialogue
                scrollResetKey={scrollResetKey}
                bottomPaddingClassName="pb-36"
                bottomPaddingPx={messageListBottomPadding}
                afterContent={!streaming && storyStateModel.hasState && stateDisplayPreference !== 'director-only' ? (
                  <StoryStateLedger
                    snapshot={snapshot}
                    displayPreference={stateDisplayPreference}
                    onDisplayPreferenceChange={onStateDisplayPreferenceChange}
                    onOpenDirectorState={onOpenDirectorState}
                  />
                ) : undefined}
                afterContentKey={`${snapshot?.current_turn?.id || ''}:${snapshot?.current_turn?.state_status || ''}:${stateDisplayPreference}`}
                messageStyle={stageTextStyle}
                collapseTraceGroups
                turnScrollRequest={turnScrollRequest}
                onVisibleTurnAnchorChange={handleVisibleTurnAnchorChange}
                onEditMessage={startEditingView}
                onEditAssistantReply={generatingImageTurnId || switchingVersionTurnId ? undefined : startEditingAssistantReply}
                onRegenerateMessage={regenerateView}
                onSwitchMessageVersion={switchViewVersion}
                onGenerateInteractiveImage={generateImageForView}
                generatingInteractiveImageTurnId={generatingImageTurnId || undefined}
                onOpenSubAgentSession={openSubAgentSession}
                activeSubAgentSessionKey={activeSubAgentSessionKey}
                onOpenTrace={openTraceRun}
              />
            )}
            {activeSubAgentSessionKey && (
              <div className="absolute inset-y-0 right-0 z-30 w-[min(420px,92vw)] border-l border-[var(--nova-border)] shadow-[var(--nova-shadow)]">
                <AgentSubAgentSessionPanel
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
      {!creatingStory ? <div ref={inputFloatRef} style={{ bottom: keyboardInset }} className="nova-story-input-float pointer-events-none absolute inset-x-0 bottom-0 z-20 p-3">
        <div className="pointer-events-auto mx-auto max-w-5xl">
          {editingTurn && !streaming ? (
            <div className="mb-3 flex min-w-0 items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-xs text-[var(--nova-text-muted)]">
              <Pencil className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
              <span className="min-w-0 flex-1 truncate">{t('storyStage.editingNotice')}</span>
              <Button type="button" variant="ghost" size="icon-xs" className="h-7 w-7 shrink-0 text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]" onClick={cancelEditing} aria-label={t('storyStage.cancelEdit')}>
                <X className="h-3.5 w-3.5" />
              </Button>
            </div>
          ) : null}
          {directorStatusVisible && directorPlanStatus ? (
            <div className="mb-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)]/90 px-3 py-2 text-xs text-[var(--nova-text-muted)] shadow-[var(--nova-shadow)] backdrop-blur-xl">
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                {directorPlanStatus.status === 'running' ? <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin text-[var(--nova-text-faint)]" /> : <Sparkles className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />}
                <span className="min-w-0 flex-1 font-medium text-[var(--nova-text)]">{t('storyStage.director.title')}</span>
                <span className="shrink-0 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-0.5 text-[11px] text-[var(--nova-text-faint)]">
                  {t('storyStage.director.progress', { completed: directorPlanStatus.completed_docs, planned: directorPlanStatus.planned_docs })}
                </span>
                {directorPlanStatus.status === 'failed' ? (
                  <Button type="button" variant="outline" size="xs" className="h-7 gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface-2)]" disabled={directorRetrying || !storyId} onClick={() => void retryDirectorPlanning()}>
                    {directorRetrying ? <Loader2 className="h-3 w-3 animate-spin" /> : <RefreshCw className="h-3 w-3" />}
                    {t('storyStage.director.retry')}
                  </Button>
                ) : null}
              </div>
              <div className="mt-1 leading-5 text-[var(--nova-text-faint)]">
                {directorRetryError || directorPlanStatus.error || directorPlanStatus.summary || t('storyStage.director.description')}
              </div>
            </div>
          ) : null}
          {showHotChoices ? (
            <div className="mb-2 overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
              <div className="flex min-h-8 items-center gap-1.5 px-2 py-1 text-[11px] text-[var(--nova-text-muted)]">
                <button type="button" className="nova-nav-item flex min-w-0 flex-1 items-center gap-1.5 rounded-[var(--nova-radius)] px-1.5 py-1 text-left hover:bg-[var(--nova-hover)]" onMouseDown={(event) => event.preventDefault()} onClick={() => setHotChoicesExpanded((value) => !value)} aria-expanded={hotChoicesExpanded}>
                  <Compass className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                  <span className="shrink-0 font-medium text-[var(--nova-text-muted)]">{t('storyStage.hotChoices.title')}</span>
                  <span className="min-w-0 flex-1 truncate text-[var(--nova-text-faint)]">{t('storyStage.hotChoices.count', { count: hotChoices.length })}</span>
                  {hotChoicesExpanded ? <ChevronUp className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" /> : <ChevronDown className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />}
                </button>
              </div>
              {hotChoicesExpanded ? (
                <div className="border-t border-[var(--nova-border)] px-2 py-2">
                  <div data-testid="story-stage-hot-choices-list" className="flex max-h-48 flex-wrap content-start gap-1.5 overflow-y-auto overscroll-contain pr-1">
                    {hotChoices.map((choice, index) => (
                      <button
                        key={`${index}-${choice}`}
                        type="button"
                        className="min-w-0 max-w-full flex-none rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-1.5 text-left text-xs leading-5 text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
                        onMouseDown={(event) => event.preventDefault()}
                        onClick={() => {
                          setInput(choice)
                          setShowSkillCommands(false)
                          setSkillCommandQuery(null)
                          setActiveSkillCommandIndex(0)
                          setHotChoicesExpanded(false)
                          window.requestAnimationFrame(() => {
                            inputRef.current?.focus()
                            inputRef.current?.setSelectionRange(choice.length, choice.length)
                          })
                        }}
                      >
                        <span className="block max-w-full break-words">{choice}</span>
                      </button>
                    ))}
                  </div>
                </div>
              ) : null}
            </div>
          ) : null}
          <div className="relative min-w-0">
              <FileReferencePicker open={styleSceneQuery !== null && styleSceneSuggestions.length > 0} query={styleSceneQuery || ''} files={styleSceneSuggestions} onSelect={selectStyleScene} trigger="#" placeholder={t('chat.styleReference.placeholder')} emptyText={t('chat.styleReference.empty')} heading={t('chat.styleReference.heading')} />
              <Popover open={showSkillCommands && filteredSkillCommands.length > 0}>
                <PopoverTrigger asChild>
                  <span className="absolute bottom-full left-0 h-0 w-0" />
                </PopoverTrigger>
                <PopoverContent align="start" side="top" className="nova-command-menu mb-2 w-[384px] overflow-hidden rounded-lg border border-[var(--nova-border)] p-0 text-[var(--nova-text)]" onOpenAutoFocus={(event) => event.preventDefault()}>
                  <Command shouldFilter={false} className="bg-transparent">
                    <div className="border-b border-[var(--nova-border-soft)] px-3 py-2">
                      <div className="flex items-center gap-2">
                        <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]">
                          <CommandIcon className="h-3.5 w-3.5" />
                        </span>
                        <div className="min-w-0">
                          <div className="text-xs font-medium text-[var(--nova-text)]">{t('chat.commands.title')}</div>
                          <div className="text-[11px] text-[var(--nova-text-faint)]">{t('chat.commands.description')}</div>
                        </div>
                      </div>
                    </div>
                    <CommandList className="max-h-[312px] p-1.5">
                      <CommandEmpty className="py-5 text-center text-xs text-[var(--nova-text-faint)]">{t('chat.commands.empty')}</CommandEmpty>
                      {filteredBuiltInCommandItems.length > 0 ? (
                      <CommandGroup heading={t('chat.commands.group')} className="[&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:pb-1 [&_[cmdk-group-heading]]:pt-1 [&_[cmdk-group-heading]]:text-[11px] [&_[cmdk-group-heading]]:text-[var(--nova-text-faint)]">
                        {filteredBuiltInCommandItems.map(({ command: skill, index }) => {
                          const active = index === activeSkillCommandIndex
                          return (
                            <CommandItem
                              key={skill.name}
                              ref={(element) => {
                                skillCommandRefs.current[index] = element
                              }}
                              value={skill.name}
                              onMouseEnter={() => setActiveSkillCommandIndex(index)}
                              onSelect={() => selectSkillCommand(skill.name)}
                              className={`group min-h-12 cursor-pointer rounded-md border px-2.5 py-2 text-[var(--nova-text-muted)] ${active ? 'border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)]' : 'border-transparent hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)]'}`}
                            >
                              <span className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-md border bg-[var(--nova-surface-2)] ${active ? 'border-[var(--nova-border)] text-[var(--nova-text)]' : 'border-[var(--nova-border)] text-[var(--nova-text-faint)]'}`}>
                                {skill.builtIn ? <Archive className="h-3.5 w-3.5" /> : <Sparkles className="h-3.5 w-3.5" />}
                              </span>
                              <span className="min-w-0 flex-1">
                                <span className="flex items-center gap-2">
                                  <span className="font-mono text-xs text-[var(--nova-text)]">/{skill.name}</span>
                                  <span className="truncate text-xs text-[var(--nova-text-muted)]">{skill.description || skill.name}</span>
                                </span>
                                <span className="mt-0.5 block text-[11px] text-[var(--nova-text-faint)]">{skill.hint}</span>
                              </span>
                            </CommandItem>
                          )
                        })}
                      </CommandGroup>
                      ) : null}
                      {filteredSkillCommandItems.length > 0 ? (
                      <CommandGroup heading={t('chat.commands.skillsGroup')} className="[&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:pb-1 [&_[cmdk-group-heading]]:pt-2 [&_[cmdk-group-heading]]:text-[11px] [&_[cmdk-group-heading]]:text-[var(--nova-text-faint)]">
                        {filteredSkillCommandItems.map(({ command: skill, index }) => {
                          const active = index === activeSkillCommandIndex
                          return (
                            <CommandItem
                              key={skill.name}
                              ref={(element) => {
                                skillCommandRefs.current[index] = element
                              }}
                              value={skill.name}
                              onMouseEnter={() => setActiveSkillCommandIndex(index)}
                              onSelect={() => selectSkillCommand(skill.name)}
                              className={`group min-h-12 cursor-pointer rounded-md border px-2.5 py-2 text-[var(--nova-text-muted)] ${active ? 'border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text)]' : 'border-transparent hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)]'}`}
                            >
                              <span className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-md border bg-[var(--nova-surface-2)] ${active ? 'border-[var(--nova-border)] text-[var(--nova-text)]' : 'border-[var(--nova-border)] text-[var(--nova-text-faint)]'}`}>
                                <Sparkles className="h-3.5 w-3.5" />
                              </span>
                              <span className="min-w-0 flex-1">
                                <span className="flex items-center gap-2">
                                  <span className="font-mono text-xs text-[var(--nova-text)]">/{skill.name}</span>
                                  <span className="truncate text-xs text-[var(--nova-text-muted)]">{skill.description || skill.name}</span>
                                </span>
                                <span className="mt-0.5 block text-[11px] text-[var(--nova-text-faint)]">{skill.hint}</span>
                              </span>
                            </CommandItem>
                          )
                        })}
                      </CommandGroup>
                      ) : null}
                    </CommandList>
                  </Command>
                </PopoverContent>
              </Popover>
            <AgentComposerShell
              className="nova-story-stage-composer"
              input={
                <ComposerTokenInput
                  ref={inputRef}
                  value={input}
                  onChange={handleInputChange}
                  onTriggerChange={handleInputTriggerChange}
                  onTokenRemove={handleTokenRemove}
                  onEditorKeyDown={(event) => {
                    const canPickSkill = showSkillCommands && filteredSkillCommands.length > 0
                    if (canPickSkill && (event.key === 'ArrowDown' || event.key === 'ArrowUp')) {
                      event.preventDefault()
                      setActiveSkillCommandIndex((current) => {
                        const direction = event.key === 'ArrowDown' ? 1 : -1
                        return (current + direction + filteredSkillCommands.length) % filteredSkillCommands.length
                      })
                      return true
                    }
                    if (event.key === 'Escape') {
                      setStyleSceneQuery(null)
                      setShowSkillCommands(false)
                      setSkillCommandQuery(null)
                      setActiveSkillCommandIndex(0)
                      return true
                    }
                    if (canPickSkill && event.key === 'Tab') {
                      event.preventDefault()
                      selectSkillCommand(filteredSkillCommands[activeSkillCommandIndex]?.name || filteredSkillCommands[0].name)
                      return true
                    }
                    if (event.key === 'Enter' && !event.shiftKey) {
                      if (isNativeComposingKeyboardEvent(event)) return false
                      event.preventDefault()
                      if (canPickSkill) {
                        selectSkillCommand(filteredSkillCommands[activeSkillCommandIndex]?.name || filteredSkillCommands[0].name)
                        return true
                      }
                      void send()
                      return true
                    }
                    return false
                  }}
                  knownSkills={skillCommands.map((skill) => skill.name)}
                  knownStyleScenes={Array.from(new Set([...styleSceneSuggestions, ...styleScenes]))}
                  externalTokens={styleScenes.map((scene) => ({ kind: 'style', value: scene, label: scene }))}
                  rows={1}
                  minRows={1}
                  maxRows={isMobile ? 5 : 10}
                  className="nova-agent-composer-textarea nova-agent-token-input min-h-[42px] resize-none border-0 bg-transparent px-1 py-[9px] text-sm leading-6 text-[var(--nova-text)] shadow-none placeholder:text-[var(--nova-text-faint)] focus-visible:border-transparent focus-visible:ring-0"
                  style={inputTextStyle}
                  disabled={branchTerminal || directorBlocking}
                  inputMode="text"
                  enterKeyHint="send"
                  autoCapitalize="sentences"
                  placeholder={branchTerminal ? t('storyStage.inputPlaceholderTerminal') : directorBlocking ? t('storyStage.director.inputBlocked') : !isMobile && skillCommands.length > 0 ? t('storyStage.inputPlaceholderWithSkills') : t('storyStage.inputPlaceholder')}
                />
              }
              toolbarStart={
                <>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        type="button"
                        variant="outline"
                        size="icon-sm"
                        className="nova-agent-composer-icon h-8 w-8 shrink-0 rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:opacity-45"
                        disabled={streaming || branchTerminal || directorBlocking || (!storyId && tokenUsageMessages.length === 0)}
                        aria-label={t('chat.input.actions')}
                        title={t('chat.input.actions')}
                      >
                        <List className="h-3.5 w-3.5" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="start" side="top" className="w-80 border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2 text-[var(--nova-text)]">
                      <InteractiveImageSettingsMenu story={story} disabled={!storyId || streaming || directorBlocking || !onImageSettingsChange} onChange={onImageSettingsChange} />
                      <StoryImagePresetMenu story={story} presets={imagePresets} disabled={!storyId || streaming || directorBlocking || !onImageSettingsChange} onChange={onImageSettingsChange} />
                      <DropdownMenuItem
                        onSelect={() => setTokenUsageOpen(true)}
                        className="cursor-pointer text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
                      >
                        <BarChart3 className="h-3.5 w-3.5" />
                        <span className="min-w-0 flex-1">{t('chat.tokenUsage.action')}</span>
                        <span className="text-[10px] text-[var(--nova-text-faint)]">{t('chat.tokenUsage.subtitle', { count: tokenUsageMessages.length })}</span>
                      </DropdownMenuItem>
                      <DropdownMenuSeparator className="bg-[var(--nova-border-soft)]" />
                      <DropdownMenuItem
                        disabled={!storyId || streaming || branchTerminal || directorBlocking}
                        onSelect={openContextAnalysis}
                        className="cursor-pointer text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
                      >
                        <ScrollText className="h-3.5 w-3.5" />
                        {t('chat.contextAnalysis.action')}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </>
              }
              toolbarEnd={
                <>
                  <Button type="button" variant="outline" className={`nova-agent-composer-pill h-8 shrink-0 rounded-[10px] border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 text-[11px] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] ${hotChoicesExpanded ? 'text-[var(--nova-text)]' : ''}`} disabled={!canUseHotChoices} onMouseDown={(event) => event.preventDefault()} onClick={toggleHotChoices} aria-label={hotChoicesExpanded ? t('storyStage.hotChoices.collapse') : t('storyStage.hotChoices.get')} title={hotChoicesExpanded ? t('storyStage.hotChoices.collapse') : t('storyStage.hotChoices.get')}>
                    <Compass className="h-3.5 w-3.5" />
                    {!isMobile ? t('storyStage.hotChoices.button') : null}
                  </Button>
                  {isMobile ? (
                    <Button type="button" variant="outline" className="nova-agent-composer-icon h-8 w-8 shrink-0 rounded-[10px] border-[var(--nova-border)] bg-[var(--nova-surface)] px-0 text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" onMouseDown={(event) => event.preventDefault()} onClick={openMobileNavigation} aria-label={t('workbench.mobile.navigationMenu')} title={t('workbench.mobile.navigationMenu')}>
                      <Plus className="h-3.5 w-3.5" />
                    </Button>
                  ) : null}
                  <ModelProfileSwitcher agentKey="interactive_story" workspace={workspace} disabled={streaming || directorBlocking} />
                </>
              }
              submitControl={
                <Button
                  className={`nova-agent-composer-submit h-9 w-9 shrink-0 rounded-[10px] px-0 text-[var(--nova-text)] shadow-[inset_0_1px_0_rgba(255,255,255,0.08)] ${streaming ? 'bg-[var(--nova-danger-bg)] hover:bg-[var(--nova-danger-bg)]' : 'bg-[var(--nova-active)] hover:bg-[var(--nova-hover)]'}`}
                  disabled={streaming ? false : !storyId || branchTerminal || directorBlocking || !input.trim()}
                  onClick={() => {
                    streaming ? stop() : void send()
                  }}
                  aria-label={streaming ? t('chat.input.stop') : editingTurn ? t('storyStage.sendRegenerate') : t('chat.input.send')}
                >
                  {streaming ? <Square className="h-3.5 w-3.5 fill-current" /> : editingTurn ? <RefreshCw className="h-3.5 w-3.5" /> : <Send className="h-3.5 w-3.5" />}
                </Button>
              }
            />
          </div>
          <ContextAnalysisDialog
            open={contextAnalysisOpen}
            loading={contextAnalysisLoading}
            error={contextAnalysisError}
            analysis={contextAnalysis}
            onOpenChange={setContextAnalysisOpen}
            onRemoveCompaction={removeContextCompaction}
          />
          <TokenUsageDialog open={tokenUsageOpen} messages={tokenUsageMessages} onOpenChange={setTokenUsageOpen} onOpenTrace={openTraceRun} />
          <Dialog open={traceOpen} onOpenChange={setTraceOpen}>
            <DialogContent className="flex h-[min(88vh,760px)] max-w-[min(96vw,1120px)] flex-col gap-0 overflow-hidden border-[var(--nova-border)] bg-[var(--nova-bg)] p-0 text-[var(--nova-text)]">
              <DialogHeader className="border-b border-[var(--nova-border)] px-4 py-3">
                <DialogTitle className="flex items-center gap-2 text-sm">
                  <Activity className="h-4 w-4 text-[var(--nova-text-muted)]" />
                  {t('chat.tracePanel.title')}
                </DialogTitle>
              </DialogHeader>
              <div className="min-h-0 flex-1">
                <AgentTracePanel selectedRunId={selectedTraceRunId} />
              </div>
            </DialogContent>
          </Dialog>
          {replyEditTarget ? (
            <EditInteractiveReplyDialog
              key={replyEditTarget.turnId}
              turnId={replyEditTarget.turnId}
              initialContent={replyEditTarget.initialContent}
              onClose={() => setReplyEditTarget(null)}
              onSave={async (narrative) => {
                await updateInteractiveTurnNarrative(storyId, replyEditTarget.turnId, {
                  branch_id: replyEditTarget.branchId,
                  narrative,
                  expected_narrative: replyEditTarget.expectedNarrative,
                })
                await onDone({ silent: true })
              }}
            />
          ) : null}
        </div>
      </div> : null}
    </main>
  )

  function appendAssistantMessage(content: string, metadata: Partial<ChatMessage> = {}) {
    if (!content) return
    const renderKey = metadata.render_key || (metadata.subagent ? undefined : currentLiveTurnRenderKeysRef.current?.assistant)
    const navigationMetadata = metadata.subagent ? metadata : { ...metadata, navigation_turn_id: liveTurnNavigationAnchorId }
    queueLiveMessage({ role: 'assistant', content, metadata: renderKey ? { ...navigationMetadata, render_key: renderKey } : navigationMetadata })
  }

  // 思考前言被误当正文显示时（孤立 </think>），丢弃这条流式 assistant 消息，正文随后另起。
  function resetAssistantMessage() {
    flushLiveMessageBuffer()
    setStageLiveMessages((prev) => {
      const last = prev[prev.length - 1]
      if (last?.role === 'assistant' && last.streaming) {
        return prev.slice(0, -1)
      }
      return prev
    })
  }

  function appendThinkingMessage(content: string, metadata: Partial<ChatMessage> = {}) {
    if (!content) return
    nonNarrativeLiveMessageStreamingRef.current = true
    queueLiveMessage({ role: 'thinking', content, metadata })
  }

  function queueLiveMessage(message: BufferedLiveMessage) {
    liveMessageBufferRef.current.push(message)
    if (liveMessageRafRef.current !== null) return
    liveMessageRafRef.current = window.requestAnimationFrame(() => {
      liveMessageRafRef.current = null
      flushLiveMessageBuffer()
    })
  }

  function flushLiveMessageBuffer() {
    if (liveMessageRafRef.current !== null) {
      window.cancelAnimationFrame(liveMessageRafRef.current)
      liveMessageRafRef.current = null
    }
    const buffered = liveMessageBufferRef.current
    if (buffered.length === 0) return
    liveMessageBufferRef.current = []
    setStageLiveMessages((prev) => buffered.reduce(appendBufferedLiveMessage, prev))
    if (buffered.some((message) => message.role === 'assistant' || message.role === 'thinking')) {
      scheduleLiveMessagePromotion()
    }
  }

  function scheduleLiveMessagePromotion() {
    if (liveMessagePromoteRafRef.current !== null) return
    liveMessagePromoteRafRef.current = window.requestAnimationFrame(() => {
      liveMessagePromoteRafRef.current = null
      promoteLiveMessageTargets()
    })
  }

  function promoteLiveMessageTargets() {
    setStageLiveMessages((prev) => promoteMessageTargets(prev))
  }

  function appendToolCallMessage(payload: Record<string, unknown> & { id?: string; name?: string; args?: string }) {
    const toolKeys = liveToolEventKeys(payload)
    const mappedId = findMappedLiveToolId(toolKeys, liveToolKeyToMessageIdRef.current)
    const id = payload.id || mappedId || `tool-${Date.now()}-${Math.random().toString(16).slice(2)}`
    const name = payload.name || 'unknown_tool'
    const metadata = streamMetadataFromPayload(payload)
    if (toolKeys.length > 0) {
      liveToolKeyToMessageIdRef.current = bindLiveToolEventKeys(toolKeys, liveToolKeyToMessageIdRef.current, id)
    }
    nonNarrativeLiveMessageStreamingRef.current = true
    setStageLiveMessages((prev) => [
      ...prev,
      {
        id,
        role: 'tool_call',
        content: name,
        name,
        args: payload.args || '',
        status: 'running',
        streaming: true,
        ...metadata,
      },
    ])
  }

  function appendToolArgsDelta(payload: Record<string, unknown> & { id?: string; name?: string; args?: string; delta?: string }) {
    if (!payload.id && !payload.name && liveToolEventKeys(payload).length === 0) return
    setStageLiveMessages((prev) => {
      const targetIndex = findToolMessageIndexForPayload(prev, payload, liveToolKeyToMessageIdRef.current)
      if (targetIndex < 0) return prev
      const matchedId = prev[targetIndex].id
      if (matchedId) {
        liveToolKeyToMessageIdRef.current = bindLiveToolEventKeys(liveToolEventKeys(payload), liveToolKeyToMessageIdRef.current, matchedId)
      }
      return prev.map((msg, index) =>
        index === targetIndex
          ? {
              ...msg,
              args: payload.args !== undefined ? payload.args : `${msg.args || ''}${payload.delta || ''}`,
            }
          : msg,
      )
    })
  }

  function updateToolCallMessage(payload: Record<string, unknown> & { id?: string; name?: string }, status: 'success' | 'error', result = '') {
    setStageLiveMessages((prev) => {
      const targetIndex = findToolMessageIndexForPayload(prev, payload, liveToolKeyToMessageIdRef.current)
      if (targetIndex < 0) return prev
      const matchedId = prev[targetIndex].id
      if (matchedId) {
        liveToolKeyToMessageIdRef.current = bindLiveToolEventKeys(liveToolEventKeys(payload), liveToolKeyToMessageIdRef.current, matchedId)
      }
      return prev.map((msg, index) => (
        index === targetIndex ? { ...msg, status, result, streaming: false } : msg
      ))
    })
  }

  function appendLiveRuleRollMessage(payload: Record<string, unknown> & { id?: string; name?: string; content?: string }) {
    if (!publicRuleRollVisible || payload.name !== 'prepare_interactive_turn') return
    const ruleRoll = publicRuleRollFromToolOutput(payload.content || '')
    if (!ruleRoll) return
    setStageLiveMessages((prev) => {
      const id = ruleRoll.resolution_id ? `live-rule-roll-${ruleRoll.resolution_id}` : `live-rule-roll-${Date.now()}`
      if (prev.some((message) => message.role === 'rule_roll' && message.id === id)) return prev
      return [
        ...prev,
        {
          id,
          role: 'rule_roll',
          rule_roll: ruleRoll,
          streaming: false,
        },
      ]
    })
  }

  function appendContextCompactionMessage(data: Record<string, unknown>) {
    const compactionId = currentCompactionMessageIdRef.current || createContextCompactionMessageId(compactionIdCounterRef)
    currentCompactionMessageIdRef.current = compactionId
    nonNarrativeLiveMessageStreamingRef.current = true
    setStageLiveMessages((prev) => upsertContextCompactionMessage(prev, buildContextCompactionMessage(data, compactionId)))
  }

  function collapseNonNarrativeMessages() {
    if (!nonNarrativeLiveMessageStreamingRef.current) return
    flushLiveMessageBuffer()
    nonNarrativeLiveMessageStreamingRef.current = false
    setStageLiveMessages((prev) =>
      prev.map((msg) =>
        msg.role === 'tool_call' || msg.role === 'context_compaction'
          ? {
              ...msg,
              status: msg.status === 'running' ? 'success' : msg.status,
            }
          : msg,
      ),
    )
  }

  function finishLiveMessages() {
    flushLiveMessageBuffer()
    if (liveMessagePromoteRafRef.current !== null) {
      window.cancelAnimationFrame(liveMessagePromoteRafRef.current)
      liveMessagePromoteRafRef.current = null
    }
    nonNarrativeLiveMessageStreamingRef.current = false
    setStageLiveMessages((prev) =>
      prev.map((msg) =>
        msg.role === 'assistant' || msg.role === 'thinking' || msg.role === 'tool_call' || msg.role === 'context_compaction'
          ? {
              ...promoteMessageTarget(msg),
              streaming: false,
              status: msg.role === 'tool_call' || msg.role === 'context_compaction' ? (msg.status === 'running' ? 'success' : msg.status) : msg.status,
            }
          : msg,
      ),
    )
  }
}

function InteractiveImageSettingsMenu({ story, disabled, onChange }: { story?: StorySummary; disabled?: boolean; onChange?: (settings: StoryImageSettings) => void | Promise<void> }) {
  const { t } = useTranslation()
  const current = normalizeStoryImageSettings(story?.image_settings)
  const [intervalDraft, setIntervalDraft] = useState(String(current.interval_turns))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    setIntervalDraft(String(current.interval_turns))
    setError('')
  }, [current.interval_turns, current.mode])

  const save = async (patch: Partial<StoryImageSettings>) => {
    if (disabled || !onChange) return
    const next = normalizeStoryImageSettings({ ...current, ...patch })
    setSaving(true)
    setError('')
    try {
      await onChange(next)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('storyStage.interactiveImage.saveFailed'))
    } finally {
      setSaving(false)
    }
  }

  const saveInterval = () => {
    const intervalTurns = normalizeIntervalTurns(intervalDraft)
    setIntervalDraft(String(intervalTurns))
    void save({ mode: 'interval', interval_turns: intervalTurns })
  }

  return (
    <>
      <DropdownMenuSeparator className="bg-[var(--nova-border-soft)]" />
      <DropdownMenuSub>
        <DropdownMenuSubTrigger
          disabled={disabled}
          className="flex cursor-pointer items-center gap-2 text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
        >
          <span className="flex h-3.5 w-3.5 items-center justify-center">
            {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin text-[var(--nova-text-faint)]" /> : <ImagePlus className="h-3.5 w-3.5" />}
          </span>
          <span className="min-w-0 flex-1 truncate">{t('storyStage.interactiveImage.menuTitle')}</span>
          <span className="max-w-36 shrink-0 truncate text-right text-[10px] text-[var(--nova-text-faint)]">{imageSettingsSummary(current, t)}</span>
        </DropdownMenuSubTrigger>
        <DropdownMenuSubContent className="w-72 border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2 text-[var(--nova-text)]">
          <DropdownMenuItem
            disabled={disabled || saving}
            onSelect={(event) => {
              event.preventDefault()
              void save({ mode: 'manual', interval_turns: current.interval_turns })
            }}
            onClick={() => void save({ mode: 'manual', interval_turns: current.interval_turns })}
            className="grid cursor-pointer grid-cols-[1rem_minmax(0,1fr)] items-center gap-2 text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
          >
            <Check className={`h-3.5 w-3.5 ${current.mode === 'manual' ? 'opacity-100' : 'opacity-0'}`} />
            <span className="min-w-0 flex-1 truncate">{t('storyStage.interactiveImage.modeManual')}</span>
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={disabled || saving}
            onSelect={(event) => {
              event.preventDefault()
              saveInterval()
            }}
            onClick={saveInterval}
            className="grid cursor-pointer grid-cols-[1rem_minmax(0,1fr)] items-center gap-2 text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
          >
            <Check className={`h-3.5 w-3.5 ${current.mode === 'interval' ? 'opacity-100' : 'opacity-0'}`} />
            <span className="min-w-0 flex-1 truncate">{t('storyStage.interactiveImage.modeInterval', { count: normalizeIntervalTurns(intervalDraft) })}</span>
          </DropdownMenuItem>
          <div className="mt-1 grid grid-cols-[1rem_minmax(0,1fr)_auto] items-center gap-2 px-2 py-1.5">
            <span />
            <div className="text-[11px] text-[var(--nova-text-faint)]">{t('storyStage.interactiveImage.intervalLabel')}</div>
            <div className="flex items-center justify-end gap-2">
              <Input
                aria-label={t('storyStage.interactiveImage.intervalInputLabel')}
                className="nova-field h-7 w-16 text-center text-xs"
                type="number"
                min={1}
                max={50}
                disabled={disabled || saving}
                value={intervalDraft}
                onPointerDown={(event) => event.stopPropagation()}
                onChange={(event) => {
                  setIntervalDraft(event.target.value)
                  setError('')
                }}
                onKeyDown={(event) => {
                  event.stopPropagation()
                  if (event.key === 'Enter') {
                    event.preventDefault()
                    saveInterval()
                  }
                }}
                onBlur={saveInterval}
              />
              <span className="text-[11px] text-[var(--nova-text-faint)]">{t('storyStage.interactiveImage.intervalSuffix')}</span>
            </div>
            {error ? <div className="col-span-3 text-[11px] leading-4 text-[var(--nova-danger)]">{error}</div> : null}
          </div>
        </DropdownMenuSubContent>
      </DropdownMenuSub>
    </>
  )
}

function StoryImagePresetMenu({ story, presets, disabled, onChange }: { story?: StorySummary; presets: ImagePreset[]; disabled?: boolean; onChange?: (settings: StoryImageSettings) => void | Promise<void> }) {
  const { t } = useTranslation()
  const current = normalizeStoryImageSettings(story?.image_settings)
  const [saving, setSaving] = useState(false)
  const normalizedPresets = useMemo(() => {
    if (presets.some((preset) => preset.id === current.preset_id)) return presets
    return [{ id: current.preset_id || 'game-cg', name: current.preset_id || 'game-cg', description: '', prompt: '', custom: true, version: 1 }, ...presets]
  }, [current.preset_id, presets])
  const selected = normalizedPresets.find((preset) => preset.id === current.preset_id) || normalizedPresets.find((preset) => preset.id === 'game-cg') || normalizedPresets[0]

  const save = async (presetId: string) => {
    if (disabled || !onChange || saving || presetId === current.preset_id) return
    setSaving(true)
    try {
      await onChange(normalizeStoryImageSettings({ ...current, preset_id: presetId }))
    } catch (err) {
      console.warn('[interactive-stage] 保存图像方案失败', err)
    } finally {
      setSaving(false)
    }
  }

  if (normalizedPresets.length === 0) return null

  return (
    <DropdownMenuSub>
      <DropdownMenuSubTrigger
        disabled={disabled || saving}
        className="flex cursor-pointer items-center gap-2 text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
      >
        <span className="flex h-3.5 w-3.5 items-center justify-center">
          {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin text-[var(--nova-text-faint)]" /> : <Sparkles className="h-3.5 w-3.5" />}
        </span>
        <span className="min-w-0 flex-1 truncate">{t('storyStage.imagePreset.menuTitle')}</span>
        <span className="max-w-36 shrink-0 truncate text-right text-[10px] text-[var(--nova-text-faint)]">{selected?.name || current.preset_id}</span>
      </DropdownMenuSubTrigger>
      <DropdownMenuSubContent className="w-72 border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2 text-[var(--nova-text)]">
        {normalizedPresets.map((preset) => {
          const selectedPreset = preset.id === selected?.id
          return (
            <DropdownMenuItem
              key={preset.id}
              disabled={disabled || saving}
              onSelect={(event) => {
                event.preventDefault()
                void save(preset.id)
              }}
              onClick={() => void save(preset.id)}
              className="cursor-pointer text-xs focus:bg-[var(--nova-active)] focus:text-[var(--nova-text)]"
            >
              <Check className={`h-3.5 w-3.5 ${selectedPreset ? 'opacity-100' : 'opacity-0'}`} />
              <span className="min-w-0 flex-1 truncate">{preset.name || preset.id}</span>
            </DropdownMenuItem>
          )
        })}
      </DropdownMenuSubContent>
    </DropdownMenuSub>
  )
}

function normalizeStoryImageSettings(value?: Partial<StoryImageSettings> | null): StoryImageSettings {
  const rawMode = typeof value?.mode === 'string' ? String(value.mode) : ''
  const mode = rawMode === 'interval' || rawMode === 'every_turn' ? 'interval' : 'manual'
  return {
    mode,
    interval_turns: rawMode === 'every_turn' ? 1 : normalizeIntervalTurns(value?.interval_turns),
    preset_id: typeof value?.preset_id === 'string' && value.preset_id.trim() ? value.preset_id.trim() : 'game-cg',
  }
}

function normalizeIntervalTurns(value: unknown) {
  const numberValue = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(numberValue) || numberValue <= 0) return DEFAULT_IMAGE_INTERVAL_TURNS
  return Math.min(50, Math.max(1, Math.floor(numberValue)))
}

function imageSettingsSummary(settings: StoryImageSettings, t: (key: string, options?: Record<string, unknown>) => string) {
  if (settings.mode === 'interval') return t('storyStage.interactiveImage.currentInterval', { count: settings.interval_turns })
  return t('storyStage.interactiveImage.currentManual')
}

function noop() {}

function noopStateDisplayPreferenceChange(_value: StoryStateDisplayPreference) {}

function noopTurnPersisted() {
  return undefined
}

function storyRuleVisibilityMode(story: StorySummary | undefined, directors: StoryDirector[]) {
  const directorID = story?.story_director_id || 'default'
  const director = directors.find((item) => item.id === directorID) || directors.find((item) => item.id === 'default')
  return director?.strategy?.rule_visibility_mode || 'audit_only'
}

function publicRuleRollFromResolution(resolution?: RuleResolution): PublicRuleRoll | null {
  if (!resolution?.result) return null
  const result = resolution.result
  return {
    resolution_id: resolution.id,
    label: result.label || resolution.request?.rule?.label || resolution.request?.challenge || resolution.request?.action,
    difficulty: resolution.request?.difficulty,
    dice: result.dice,
    roll_mode: result.roll_mode || resolution.request?.rule?.roll_mode,
    rolls: result.rolls,
    kept_roll: result.kept_roll,
    base_target: result.base_target,
    target: result.target,
    bonus_total: result.bonus_total,
    total: result.total,
    outcome: result.outcome,
    result: result.result,
    cost: resolution.request?.cost,
    stakes: resolution.request?.adjudication?.stakes,
    state_changes: result.state_changes,
  }
}

function publicRuleRollFromToolOutput(content: string): PublicRuleRoll | null {
  const parsed = parseJSONRecord(content)
  if (!parsed) return null
  const rolls = Array.isArray(parsed.rolls) ? parsed.rolls.map(Number).filter(Number.isFinite) : undefined
  const stateChanges = Array.isArray(parsed.state_changes)
    ? parsed.state_changes
      .map((item) => isPlainRecord(item) ? {
        actor_id: String(item.actor_id || '').trim(),
        field_id: String(item.field_id || '').trim(),
        change: Number(item.change),
        reason: typeof item.reason === 'string' ? item.reason : undefined,
      } : null)
      .filter((item): item is NonNullable<typeof item> => Boolean(item && item.actor_id && item.field_id && Number.isFinite(item.change)))
    : undefined
  return {
    resolution_id: stringFromRecord(parsed, 'resolution_id'),
    label: stringFromRecord(parsed, 'label') || stringFromRecord(parsed, 'challenge'),
    difficulty: stringFromRecord(parsed, 'difficulty'),
    dice: stringFromRecord(parsed, 'dice'),
    roll_mode: stringFromRecord(parsed, 'roll_mode'),
    rolls,
    kept_roll: numberFromRecord(parsed, 'kept_roll'),
    base_target: numberFromRecord(parsed, 'base_target'),
    target: numberFromRecord(parsed, 'target'),
    bonus_total: numberFromRecord(parsed, 'bonus_total'),
    total: numberFromRecord(parsed, 'total'),
    outcome: stringFromRecord(parsed, 'outcome'),
    result: stringFromRecord(parsed, 'result'),
    cost: stringFromRecord(parsed, 'cost'),
    stakes: stringFromRecord(parsed, 'stakes'),
    state_changes: stateChanges,
  }
}

function parseJSONRecord(content: string): Record<string, unknown> | null {
  try {
    const parsed = JSON.parse(content)
    return isPlainRecord(parsed) ? parsed : null
  } catch {
    return null
  }
}

function stringFromRecord(record: Record<string, unknown>, key: string) {
  const value = record[key]
  return typeof value === 'string' && value.trim() ? value.trim() : undefined
}

function numberFromRecord(record: Record<string, unknown>, key: string) {
  const value = Number(record[key])
  return Number.isFinite(value) ? value : undefined
}

function isPlainRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value)
}

function createLiveTurnRenderKeys(): LiveTurnRenderKeys {
  const id = `${Date.now()}-${Math.random().toString(16).slice(2)}`
  return {
    user: `interactive-live-user-${id}`,
    assistant: `interactive-live-assistant-${id}`,
  }
}

function storyStageSnapshotKey(storyId: string, branchId: string, snapshot?: Snapshot | null) {
  const turns = snapshot?.turns || []
  return `${storyId || snapshot?.story_id || 'none'}:${snapshot?.branch_id || branchId || 'main'}:${turns[turns.length - 1]?.id || 'empty'}`
}

function useStagePreferences() {
  const [preferences, setPreferences] = useState({
    lineHeight: DEFAULT_STAGE_LINE_HEIGHT,
  })

  const load = useCallback(async () => {
    try {
      const settings = await fetchSettings()
      const effective = settings.effective || {}
      setPreferences({
        lineHeight: clampNumber(effective.interactive_stage_line_height, 1.35, 2.4, DEFAULT_STAGE_LINE_HEIGHT),
      })
    } catch (error) {
      console.warn('[interactive-stage] 加载故事舞台显示设置失败', error)
      setPreferences({
        lineHeight: DEFAULT_STAGE_LINE_HEIGHT,
      })
    }
  }, [])

  useEffect(() => {
    void load()
    window.addEventListener('nova:settings-updated', load)
    return () => window.removeEventListener('nova:settings-updated', load)
  }, [load])

  return preferences
}

function clampNumber(value: unknown, min: number, max: number, fallback: number) {
  const numberValue = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(numberValue)) return fallback
  return Math.min(max, Math.max(min, numberValue))
}

function isAbortError(error: unknown) {
  return error instanceof DOMException && error.name === 'AbortError'
}

function normalizeMessageContent(value: string) {
  return value.replace(/\r\n/g, '\n').trim()
}

function buildTokenUsageMessage(data: Record<string, unknown> | TokenUsageEvent, fallbackId?: string): ChatMessage {
  const runId = readString(data.run_id)
  return {
    role: 'token_usage',
    id: runId || fallbackId || `token-usage-${Date.now()}`,
    content: '',
    run_id: runId,
    agent_kind: readString(data.agent_kind),
    prompt_tokens: readNumber(data.prompt_tokens),
    cached_prompt_tokens: readNumber(data.cached_prompt_tokens),
    uncached_prompt_tokens: readNumber(data.uncached_prompt_tokens),
    cache_hit_rate: readNumber(data.cache_hit_rate),
    completion_tokens: readNumber(data.completion_tokens),
    reasoning_tokens: readNumber(data.reasoning_tokens),
    total_tokens: readNumber(data.total_tokens),
    model_calls: readNumber(data.model_calls),
    generated_bytes: readNumber(data.generated_bytes),
    usage_calls: readUsageCalls(data.usage_calls),
    created_at: readString(data.created_at) || new Date().toISOString(),
  }
}

function readUsageCalls(value: unknown) {
  if (!Array.isArray(value)) return undefined
  return value
    .map((item) => {
      if (!item || typeof item !== 'object') return null
      const call = item as Record<string, unknown>
      return {
        index: readNumber(call.index),
        created_at: readString(call.created_at),
        finish_reason: readString(call.finish_reason),
        requested_tools: readStringArray(call.requested_tools),
        after_tools: readStringArray(call.after_tools),
        prompt_tokens: readNumber(call.prompt_tokens),
        cached_prompt_tokens: readNumber(call.cached_prompt_tokens),
        uncached_prompt_tokens: readNumber(call.uncached_prompt_tokens),
        cache_hit_rate: readNumber(call.cache_hit_rate),
        completion_tokens: readNumber(call.completion_tokens),
        reasoning_tokens: readNumber(call.reasoning_tokens),
        total_tokens: readNumber(call.total_tokens),
      }
    })
    .filter((call): call is NonNullable<typeof call> => Boolean(call))
}

function readStringArray(value: unknown) {
  if (!Array.isArray(value)) return undefined
  const result = value.map((item) => readString(item)).filter(Boolean)
  return result.length > 0 ? result : undefined
}

function upsertTokenUsageMessage(messages: ChatMessage[], next: ChatMessage) {
  if (!next.run_id) return [...messages, next]
  let found = false
  const updated = messages.map((message) => {
    if (message.role === 'token_usage' && message.run_id === next.run_id) {
      found = true
      return { ...message, ...next }
    }
    return message
  })
  return found ? updated : [...updated, next]
}

function mergeTokenUsageMessages(persisted: ChatMessage[], live: ChatMessage[]) {
  return live.reduce((messages, message) => upsertTokenUsageMessage(messages, message), [...persisted])
}

function readString(value: unknown) {
  return typeof value === 'string' ? value : ''
}

function readNumber(value: unknown) {
  const numberValue = typeof value === 'number' ? value : Number(value)
  return Number.isFinite(numberValue) ? numberValue : 0
}

function readInteractiveImage(result?: string): InteractiveImage | undefined {
  const data = parseEventResult(result)
  if (!data || typeof data !== 'object') return undefined
  const record = data as Record<string, unknown>
  if (record.schema !== 'interactive_image.v1' || typeof record.image_path !== 'string' || !record.image_path) return undefined
  return record as unknown as InteractiveImage
}

function readInteractiveImageError(result?: string): InteractiveImageError | undefined {
  const data = parseEventResult(result)
  if (!data || typeof data !== 'object') return undefined
  const record = data as Record<string, unknown>
  if (record.schema !== 'interactive_image_error.v1') return undefined
  return record as unknown as InteractiveImageError
}

function parseEventResult(result?: string): unknown {
  if (!result) return null
  try {
    return JSON.parse(result)
  } catch {
    return null
  }
}

function interactiveImages(messages: ChatMessage[]): InteractiveImage[] | undefined {
  const images = messages
    .map((message) => message.interactive_image)
    .filter((image): image is InteractiveImage => Boolean(image?.image_path))
  return images.length > 0 ? images : undefined
}

function mergeInteractiveImages(persisted?: InteractiveImage[], optimistic?: InteractiveImage[]) {
  const merged: InteractiveImage[] = []
  for (const image of [...(persisted || []), ...(optimistic || [])]) {
    if (!image.image_path || merged.some((item) => item.image_path === image.image_path)) continue
    merged.push(image)
  }
  return merged.length > 0 ? merged : undefined
}

function latestMergedInteractiveImage(images?: InteractiveImage[]) {
  return images?.[images.length - 1]
}

function latestInteractiveImageError(messages: ChatMessage[]): InteractiveImageError | undefined {
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const error = messages[index].interactive_image_error
    if (error) return error
  }
  return undefined
}

function latestInteractiveImageStatus(messages: ChatMessage[]): 'running' | 'success' | 'error' | undefined {
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const status = messages[index].status
    if (status === 'running' || status === 'success' || status === 'error') return status
  }
  return undefined
}

function parseInlineStyleScenes(input: string): string[] {
  const result = new Set<string>()
  const regex = /(?:^|\s)#([^\s#]+)/g
  let match: RegExpExecArray | null
  while ((match = regex.exec(input)) !== null) {
    result.add(match[1])
  }
  return Array.from(result)
}

function isNativeComposingKeyboardEvent(event: KeyboardEvent) {
  return event.isComposing || event.key === 'Process' || event.keyCode === 229
}
