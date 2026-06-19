import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, ChangeEvent } from 'react'
import { Archive, BarChart3, BookOpen, ChevronDown, ChevronUp, Command as CommandIcon, Compass, List, Loader2, PanelRight, Pencil, Plus, RefreshCw, ScrollText, Send, SlidersHorizontal, Sparkles, Square, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Command, CommandEmpty, CommandGroup, CommandItem, CommandList } from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { FileReferencePicker } from '@/components/Chat/FileReferencePicker'
import { CONTEXT_ANALYSIS_SIMULATED_MESSAGE, ContextAnalysisDialog } from '@/components/Chat/ContextAnalysisDialog'
import { MessageList } from '@/components/Chat/MessageList'
import { AgentComposerShell } from '@/components/Chat/AgentComposerShell'
import { ModelProfileSwitcher } from '@/components/Chat/ModelProfileSwitcher'
import { ReferenceChips } from '@/components/Chat/ReferenceChips'
import { TokenUsageDialog } from '@/components/Chat/TokenUsagePanel'
import { SubAgentSessionPanel } from '@/components/Chat/SubAgentSessionPanel'
import { buildContextCompactionMessage, createContextCompactionMessageId, upsertContextCompactionMessage } from '@/components/Chat/context-compaction-message'
import { subAgentSessionKey } from '@/components/Chat/subagent-session'
import { MOBILE_NAVIGATION_OPEN_EVENT } from '@/components/layout/workspace-mobile-layout'
import type { ChatMessage, ContextAnalysis } from '@/lib/api'
import { isComposingKeyboardEvent } from '@/lib/keyboard'
import { fetchSettings } from '@/features/settings/api'
import { useSkillCommands } from '@/hooks/useSkillCommands'
import { abortInteractiveChat, analyzeInteractiveContext, compactInteractiveContext, generateInteractiveHotChoices, removeInteractiveContextCompaction, sendInteractiveMessage, switchInteractiveTurnVersion } from '../api'
import { createInteractiveNarrativeFilter } from '../stream-parser'
import { emptyStoryStageRun, useInteractiveStore } from '../stores/interactive-store'
import type { StoryStageRunState } from '../stores/interactive-store'
import { DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS, buildOpeningPrompt, truncateStoryOpeningText, type BookOpeningPreset, type StoryCreateInput } from '../opening'
import type { Snapshot, StorySummary, Teller, TokenUsageEvent } from '../types'
import { StoryPicker } from './StoryPicker'
import { TellerPicker } from './TellerPicker'
import { useIsMobile } from '@/hooks/useIsMobile'

interface StoryStageProps {
  workspace?: string
  styleSceneSuggestions?: string[]
  stories?: StorySummary[]
  story?: StorySummary
  tellers?: Teller[]
  storyId: string
  branchId: string
  snapshot: Snapshot | null
  snapshotLoading?: boolean
  loreEmpty?: boolean
  bookOpeningPresets?: BookOpeningPreset[]
  sceneMemoryVisible?: boolean
  onStorySelect?: (storyId: string) => void
  onStoryCreate?: (input: StoryCreateInput) => void
  onStoryDelete?: (storyId: string) => void
  onTellerChange?: (tellerId: string) => void
  onReplyTargetCharsChange?: (replyTargetChars: number) => void | Promise<void>
  onRequestLoreInit?: () => void
  onToggleSceneMemory?: () => void
  onDone: () => void | Promise<Snapshot | void>
}

const DEFAULT_READING_FONT_SIZE = 18
const DEFAULT_STAGE_LINE_HEIGHT = 1.78
const EMPTY_STAGE_RUN = emptyStoryStageRun()
const stageAbortControllers = new Map<string, AbortController>()

export function StoryStage({ workspace, styleSceneSuggestions = [], stories = [], story, tellers = [], storyId, branchId, snapshot, snapshotLoading = false, loreEmpty = false, bookOpeningPresets = [], sceneMemoryVisible = true, onStorySelect = noop, onStoryCreate = noop, onStoryDelete = noop, onTellerChange = noop, onReplyTargetCharsChange, onRequestLoreInit, onToggleSceneMemory, onDone }: StoryStageProps) {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const [input, setInput] = useState('')
  const [stageControlsOpen, setStageControlsOpen] = useState(false)
  const [styleScenes, setStyleScenes] = useState<string[]>([])
  const [styleSceneQuery, setStyleSceneQuery] = useState<string | null>(null)
  const [showSkillCommands, setShowSkillCommands] = useState(false)
  const [activeSkillCommandIndex, setActiveSkillCommandIndex] = useState(0)
  const [inputFloatHeight, setInputFloatHeight] = useState(0)
  const inputRef = useRef<HTMLTextAreaElement | null>(null)
  const inputFloatRef = useRef<HTMLDivElement | null>(null)
  const skillCommandRefs = useRef<Array<HTMLDivElement | null>>([])
  const skillCommands = useSkillCommands({
    agentKey: 'interactive_story',
    workspace,
    fallbackEnabled: true,
  })
  const snapshotKey = `${storyId || 'none'}:${snapshot?.branch_id || branchId || 'main'}:${snapshot?.turns?.[snapshot.turns.length - 1]?.id || 'empty'}`
  const stageKey = `${workspace || 'current'}:${storyId || 'none'}:${branchId || snapshot?.branch_id || 'main'}`
  const { storyStageRuns, setStoryStageRun, clearStoryStageRun } = useInteractiveStore()
  const stageRun = storyStageRuns[stageKey] || EMPTY_STAGE_RUN
  const streaming = stageRun.streaming
  const activityContent = stageRun.activityContent
  const liveMessages = stageRun.liveMessages
  const rewindTurnId = stageRun.rewindTurnId
  const [editingTurn, setEditingTurn] = useState<{
    id: string
    content: string
  } | null>(null)
  const [switchingVersionTurnId, setSwitchingVersionTurnId] = useState<string | null>(null)
  const [generatedHotChoices, setGeneratedHotChoices] = useState<string[]>([])
  const [hotChoicesExpanded, setHotChoicesExpanded] = useState(false)
  const [hotChoicesLoading, setHotChoicesLoading] = useState(false)
  const [selectedBookOpeningPresetId, setSelectedBookOpeningPresetId] = useState('')
  const [customOpeningText, setCustomOpeningText] = useState('')
  const [contextAnalysisOpen, setContextAnalysisOpen] = useState(false)
  const [tokenUsageOpen, setTokenUsageOpen] = useState(false)
  const [contextAnalysisLoading, setContextAnalysisLoading] = useState(false)
  const [contextAnalysisError, setContextAnalysisError] = useState<string | null>(null)
  const [contextAnalysis, setContextAnalysis] = useState<ContextAnalysis | null>(null)
  const [activeSubAgentSessionKey, setActiveSubAgentSessionKey] = useState('')
  const hotChoicesAbortRef = useRef<AbortController | null>(null)
  const currentCompactionMessageIdRef = useRef<string | null>(null)
  const compactionIdCounterRef = useRef(0)
  const liveStageKeyRef = useRef(stageKey)
  const previousSnapshotKeyRef = useRef(snapshotKey)
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
      .filter((msg) => msg.role === 'assistant')
      .map((msg) => msg.content || '')
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
    if (!input.startsWith('/')) return []
    const query = input.slice(1).toLowerCase()
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
  }, [input, skillCommands, t])

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

  useEffect(() => {
    if (!showSkillCommands || filteredSkillCommands.length === 0) return
    skillCommandRefs.current[activeSkillCommandIndex]?.scrollIntoView({
      block: 'nearest',
    })
  }, [activeSkillCommandIndex, filteredSkillCommands.length, showSkillCommands])

  const historyMessages = useMemo<ChatMessage[]>(() => {
    const turns = snapshot?.turns || []
    const rewindIndex = rewindTurnId ? turns.findIndex((turn) => turn.id === rewindTurnId) : -1
    const pathTurns = rewindIndex >= 0 ? turns.slice(0, rewindIndex) : turns
    return pathTurns.flatMap((turn) => {
      const messages: ChatMessage[] = [
        {
          id: `${turn.id}-user`,
          turn_id: turn.id,
          role: 'user',
          content: turn.user,
        },
      ]
      const displayEvents = turn.display_events || []
      const hasDisplayTimelineThinking = displayEvents.some((event) => event.role === 'thinking')
      if (!hasDisplayTimelineThinking && turn.thinking?.trim()) {
        messages.push({
          id: `${turn.id}-thinking`,
          role: 'thinking',
          content: turn.thinking,
          streaming: false,
        })
      }
      for (const [index, event] of displayEvents.entries()) {
        if (event.role === 'thinking') {
          messages.push({
            id: event.id || `${turn.id}-thinking-${index}`,
            role: 'thinking',
            content: event.content || '',
            streaming: false,
            created_at: event.created_at,
            run_id: event.run_id,
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
          messages.push({
            id: event.id || `${turn.id}-tool-${index}`,
            role: 'tool_call',
            content: event.content || event.name || 'unknown_tool',
            name: event.name || event.content,
            args: event.args || '',
            status: event.status || 'success',
            result: event.result || '',
            streaming: false,
            created_at: event.created_at,
            run_id: event.run_id,
            agent_name: event.agent_name,
            root_agent_name: event.root_agent_name,
            run_path: event.run_path,
            subagent: event.subagent,
            subagent_session_id: event.subagent_session_id,
            subagent_type: event.subagent_type,
          })
          continue
        }
        if (event.role === 'assistant') {
          messages.push({
            id: event.id || `${turn.id}-subagent-${index}`,
            role: 'assistant',
            content: event.content || '',
            streaming: false,
            created_at: event.created_at,
            run_id: event.run_id,
            agent_name: event.agent_name,
            root_agent_name: event.root_agent_name,
            run_path: event.run_path,
            subagent: event.subagent,
            subagent_session_id: event.subagent_session_id,
            subagent_type: event.subagent_type,
          })
        }
      }
      messages.push({
        id: `${turn.id}-assistant`,
        turn_id: turn.id,
        role: 'assistant',
        content: turn.narrative,
        turn_versions: turn.versions,
        turn_version_index: turn.version_idx,
      })
      return messages
    })
  }, [rewindTurnId, snapshot?.turns])

  const displayLiveMessages = hasPersistedLiveTurn ? [] : liveMessages.filter((message) => message.role !== 'token_usage')
  const messages = useMemo(() => [...historyMessages, ...displayLiveMessages], [displayLiveMessages, historyMessages])
  const openSubAgentSession = useCallback((message: ChatMessage) => {
    const key = subAgentSessionKey(message)
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
  const title = pickSceneTitle(snapshot, branchId, t)
  const hotChoices = useMemo(
    () =>
      generatedHotChoices
        .map((choice) => choice.trim())
        .filter(Boolean)
        .slice(0, 10),
    [generatedHotChoices],
  )
  const canUseHotChoices = !streaming && !editingTurn && stagePreferences.hotChoicesEnabled && Boolean(storyId)
  const showHotChoices = canUseHotChoices && hotChoicesExpanded
  const messageListBottomPadding = inputFloatHeight > 0 ? inputFloatHeight + 20 : undefined
  const availableBookOpeningPresets = useMemo(() => bookOpeningPresets.filter((preset) => preset.content.trim()), [bookOpeningPresets])
  const selectedBookOpeningPreset = availableBookOpeningPresets.find((preset) => preset.id === selectedBookOpeningPresetId) || availableBookOpeningPresets[0] || null
  const turnsById = useMemo(() => {
    const result = new Map<string, { user: string }>()
    for (const turn of snapshot?.turns || []) {
      result.set(turn.id, { user: turn.user })
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
  }, [editingTurn, hotChoices.length, hotChoicesLoading, input, showHotChoices, syncInputFloatHeight])

  useEffect(() => {
    const element = inputFloatRef.current
    if (!element || typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(syncInputFloatHeight)
    observer.observe(element)
    return () => observer.disconnect()
  }, [syncInputFloatHeight])

  const requestHotChoices = useCallback(
    (append = false) => {
      if (!stagePreferences.hotChoicesEnabled || streaming || editingTurn || !storyId || hotChoicesLoading) return
      const abortController = new AbortController()
      hotChoicesAbortRef.current?.abort()
      hotChoicesAbortRef.current = abortController
      setHotChoicesLoading(true)
      generateInteractiveHotChoices(storyId, {
        branch: branchId || snapshot?.branch_id,
        exclude_choices: append ? hotChoices : [],
        signal: abortController.signal,
      })
        .then((result) => {
          if (abortController.signal.aborted) return
          const nextChoices = result.enabled ? result.choices || [] : []
          setGeneratedHotChoices((current) => (append ? mergeHotChoices(current, nextChoices) : nextChoices))
        })
        .catch((error) => {
          if (!isAbortError(error)) {
            console.warn('[interactive-stage] 生成快捷选择失败', error)
          }
          if (!abortController.signal.aborted && !append) setGeneratedHotChoices([])
        })
        .finally(() => {
          if (!abortController.signal.aborted) setHotChoicesLoading(false)
        })
    },
    [branchId, editingTurn, hotChoices, hotChoicesLoading, snapshot?.branch_id, stagePreferences.hotChoicesEnabled, storyId, streaming],
  )

  const toggleHotChoices = () => {
    if (!canUseHotChoices) return
    const nextExpanded = !hotChoicesExpanded
    setHotChoicesExpanded(nextExpanded)
    if (nextExpanded && hotChoices.length === 0 && !hotChoicesLoading) {
      requestHotChoices(false)
    }
  }

  useEffect(() => {
    hotChoicesAbortRef.current?.abort()
    setGeneratedHotChoices([])
    setHotChoicesExpanded(false)
    setHotChoicesLoading(false)
  }, [snapshotKey])

  useEffect(() => {
    if (!stagePreferences.hotChoicesEnabled) {
      hotChoicesAbortRef.current?.abort()
      setGeneratedHotChoices([])
      setHotChoicesExpanded(false)
      setHotChoicesLoading(false)
    }
  }, [stagePreferences.hotChoicesEnabled])

  useEffect(() => {
    setSelectedBookOpeningPresetId((current) => {
      if (current && availableBookOpeningPresets.some((preset) => preset.id === current)) return current
      return availableBookOpeningPresets[0]?.id || ''
    })
  }, [availableBookOpeningPresets])

  const send = async (override?: { message?: string; rewindTurnId?: string }) => {
    const sourceMessage = override?.message ?? input
    const message = sourceMessage.trim()
    if (!message || !storyId || streaming) return
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
    setActiveSkillCommandIndex(0)
    setStageActivityContent(t('storyStage.activity.connecting'))
    setStageLiveMessages([{ role: 'user', content: message }])
    currentCompactionMessageIdRef.current = null
    updateStageRun({ rewindTurnId: nextRewindTurnId || undefined })
    liveStageKeyRef.current = stageKey
    setStageStreaming(true)
    const abortController = new AbortController()
    stageAbortControllers.set(stageKey, abortController)
    const narrativeFilter = createInteractiveNarrativeFilter()
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
            const visible = narrativeFilter.push(data.content || '')
            if (visible) {
              collapseNonNarrativeMessages()
              appendAssistantMessage(visible)
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
          case 'tool_call': {
            const data = JSON.parse(value.data)
            appendToolCallMessage(data)
            setStageActivityContent(
              t('storyStage.activity.processingTool', {
                name: data.name || t('storyStage.activity.toolCall'),
              }),
            )
            break
          }
          case 'tool_args_delta': {
            const data = JSON.parse(value.data)
            appendToolArgsDelta(data)
            break
          }
          case 'tool_result': {
            const data = JSON.parse(value.data)
            updateToolCallMessage(data.id, data.name, 'success', data.content || '')
            setStageActivityContent('')
            break
          }
          case 'context_compaction': {
            const data = JSON.parse(value.data)
            appendContextCompactionMessage(data)
            setStageActivityContent('')
            if (data.status === 'completed' || data.status === 'failed') {
              currentCompactionMessageIdRef.current = null
            }
            break
          }
          case 'token_usage': {
            const data = JSON.parse(value.data)
            setStageLiveMessages((prev) => upsertTokenUsageMessage(prev, buildTokenUsageMessage(data)))
            break
          }
          case 'error': {
            const data = JSON.parse(value.data)
            setStageActivityContent('')
            setStageLiveMessages((prev) => [
              ...prev,
              {
                role: 'error',
                content: data.message || data.error || t('storyStage.activity.unknownError'),
              },
            ])
            break
          }
          case 'done': {
            const visible = narrativeFilter.flush()
            collapseNonNarrativeMessages()
            if (visible) appendAssistantMessage(visible)
            finishLiveMessages()
            setStageActivityContent(t('storyStage.activity.done'))
            break
          }
          case 'aborted': {
            const visible = narrativeFilter.flush()
            collapseNonNarrativeMessages()
            if (visible) appendAssistantMessage(visible)
            finishLiveMessages()
            setStageActivityContent(t('storyStage.activity.aborted'))
            break
          }
        }
      }
      await onDone()
    } catch (error) {
      if (!isAbortError(error)) {
        setStageActivityContent('')
        setStageLiveMessages((prev) => [
          ...prev,
          {
            role: 'error',
            content: error instanceof Error ? error.message : t('storyStage.activity.runFailed'),
          },
        ])
      }
    } finally {
      setStageStreaming(false)
      stageAbortControllers.delete(stageKey)
      currentCompactionMessageIdRef.current = null
      setStageActivityContent('')
    }
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
    stageAbortControllers.get(stageKey)?.abort()
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
    if (!message.turn_id || streaming) return
    const source = turnsById.get(message.turn_id)?.user || message.content || ''
    void send({ message: source, rewindTurnId: message.turn_id })
  }

  const startOpening = (mode: 'ai' | 'book_preset' | 'custom') => {
    if (!storyId || streaming) return
    if (mode === 'book_preset' && !selectedBookOpeningPreset?.content.trim()) return
    const customText = truncateStoryOpeningText(customOpeningText)
    if (mode === 'custom' && !customText) return
    if (mode === 'book_preset') {
      void send({
        message: buildOpeningPrompt(
          story,
          t,
          {
            mode: 'preset',
            preset_id: selectedBookOpeningPreset.id,
            preset_text: selectedBookOpeningPreset.content,
          },
          'book_preset',
        ),
      })
      return
    }
    if (mode === 'custom') {
      void send({ message: buildOpeningPrompt(story, t, { mode: 'custom', custom_text: customText }) })
      setCustomOpeningText('')
      return
    }
    void send({ message: buildOpeningPrompt(story, t, { mode: 'ai' }) })
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

  const cancelEditing = () => {
    setEditingTurn(null)
    setInput('')
    setStyleSceneQuery(null)
    setShowSkillCommands(false)
    setActiveSkillCommandIndex(0)
  }

  const handleInputChange = (event: ChangeEvent<HTMLTextAreaElement>) => {
    const nextValue = event.target.value
    setInput(nextValue)
    setShowSkillCommands(nextValue.startsWith('/'))
    setActiveSkillCommandIndex(0)
    const styleMatch = nextValue.match(/(?:^|\s)#([^\s#]*)$/)
    setStyleSceneQuery(styleMatch ? styleMatch[1] : null)
  }

  const selectSkillCommand = (name: string) => {
    setInput(`/${name} `)
    setShowSkillCommands(false)
    setActiveSkillCommandIndex(0)
    inputRef.current?.focus()
  }

  const selectStyleScene = (scene: string) => {
    setInput((current) =>
      current.replace(/(?:^|\s)#([^\s#]*)$/, (match) => {
        const prefix = match.startsWith(' ') ? ' ' : ''
        return `${prefix}#${scene} `
      }),
    )
    setStyleScenes((current) => Array.from(new Set([...current, scene])))
    setStyleSceneQuery(null)
    inputRef.current?.focus()
  }

  const removeStyleScene = (scene: string) => {
    setStyleScenes((current) => current.filter((item) => item !== scene))
  }

  const stageControls = (
    <>
      <StoryPicker stories={stories} currentStoryId={storyId} tellers={tellers} onSelect={onStorySelect} onCreate={onStoryCreate} onDelete={onStoryDelete} />
      <TellerPicker story={story} tellers={tellers} onChange={onTellerChange} />
      <ReplyTargetCharsControl story={story} onChange={onReplyTargetCharsChange} />
      {onToggleSceneMemory && (
        <Button type="button" variant="outline" size="sm" className={`h-7 gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-[11px] hover:bg-[var(--nova-hover)] ${sceneMemoryVisible ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'}`} onClick={onToggleSceneMemory} aria-label={sceneMemoryVisible ? t('storyStage.hideSceneMemory') : t('storyStage.showSceneMemory')} title={sceneMemoryVisible ? t('storyStage.hideSceneMemory') : t('storyStage.showSceneMemory')}>
          <PanelRight className="h-3.5 w-3.5" />
          {t('storyStage.sceneMemory')}
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
          <div className="nova-story-stage-header nova-topbar flex min-h-14 flex-wrap items-center justify-between gap-3 border-b px-4 py-2">
            <div className="nova-story-stage-title min-w-0 flex-1">
              <div className="text-[10px] font-medium leading-4 text-[var(--nova-text-faint)]">{t('storyStage.branchLabel', { branch: branchId || 'main' })}</div>
              <div className="truncate text-xs font-semibold leading-5 text-[var(--nova-text)]">{title}</div>
            </div>
            <div className="nova-story-stage-controls flex min-w-0 flex-wrap items-center justify-end gap-2">
              {stageControls}
            </div>
          </div>
        )}

        <div className="flex min-h-0 flex-1 overflow-hidden bg-[var(--nova-surface-2)]">
          <section className="relative flex min-h-0 min-w-0 flex-1 flex-col bg-[var(--nova-surface-2)]">
            {snapshotLoading && messages.length === 0 && !streaming ? (
              <div className="m-5 flex min-h-0 flex-1 items-center justify-center rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-6 text-center text-sm text-[var(--nova-text-faint)] shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]">
                <div className="flex max-w-md flex-col items-center gap-3">
                  <RefreshCw className="h-4 w-4 animate-spin text-[var(--nova-text-muted)]" />
                  <div className="text-xs leading-5 text-[var(--nova-text-faint)]">{t('common.loading')}</div>
                </div>
              </div>
            ) : messages.length === 0 && !streaming ? (
              <div className="m-5 flex min-h-0 flex-1 items-center justify-center rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-6 text-center text-sm text-[var(--nova-text-faint)] shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]">
                <div className="flex w-full max-w-xl flex-col items-center gap-3">
                  <Sparkles className="h-4 w-4 text-[var(--nova-text-muted)]" />
                  <div className="space-y-1">
                    <div className="text-sm font-medium text-[var(--nova-text)]">{t('storyStage.opening.emptyTitle')}</div>
                    <div className="text-xs leading-5 text-[var(--nova-text-faint)]">{t('storyStage.opening.emptyDescription')}</div>
                  </div>
                  {loreEmpty && onRequestLoreInit ? (
                    <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-center">
                      <div className="text-xs font-medium text-[var(--nova-text)]">{t('loreInit.interactiveTitle')}</div>
                      <div className="mt-1 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('loreInit.interactiveDescription')}</div>
                      <button type="button" className="nova-nav-item mt-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-1.5 text-xs text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" onClick={onRequestLoreInit}>
                        {t('loreInit.openAgent')}
                      </button>
                    </div>
                  ) : null}
                  <div className="grid w-full gap-2 sm:grid-cols-2">
                    <Button type="button" size="sm" className="gap-1.5" disabled={!storyId || streaming} onClick={() => startOpening('ai')}>
                      <Sparkles className="h-3.5 w-3.5" />
                      {t('storyStage.opening.startAI')}
                    </Button>
                    <div className="flex min-w-0 gap-2">
                      <Select value={selectedBookOpeningPreset?.id || ''} onValueChange={setSelectedBookOpeningPresetId} disabled={availableBookOpeningPresets.length === 0}>
                        <SelectTrigger size="sm" className="nova-field min-w-0 flex-1 px-3 py-0.5 text-xs focus:ring-0" aria-label={t('storyStage.opening.bookPresetSelect')}>
                          <SelectValue placeholder={t('storyStage.opening.bookPresetSelect')} />
                        </SelectTrigger>
                        <SelectContent className="nova-panel border text-[var(--nova-text)]">
                          {availableBookOpeningPresets.map((preset) => (
                            <SelectItem key={preset.id} value={preset.id}>
                              {preset.title || t('storyStage.opening.bookPresetUntitled')}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <Button type="button" variant="outline" size="sm" className="shrink-0 gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" disabled={!storyId || streaming || !selectedBookOpeningPreset} onClick={() => startOpening('book_preset')}>
                        <BookOpen className="h-3.5 w-3.5" />
                        {t('storyStage.opening.startBookPreset')}
                      </Button>
                    </div>
                  </div>
                  {availableBookOpeningPresets.length === 0 ? <div className="text-[11px] leading-4 text-[var(--nova-text-faint)]">{t('storyStage.opening.bookPresetMissing')}</div> : null}
                  <div className="w-full space-y-2">
                    <Textarea autoResize className="nova-field min-h-20 resize-none text-xs" placeholder={t('storyStage.opening.customPlaceholder')} value={customOpeningText} onChange={(event) => setCustomOpeningText(event.target.value)} />
                    <Button type="button" variant="outline" size="sm" className="gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" disabled={!storyId || streaming || !customOpeningText.trim()} onClick={() => startOpening('custom')}>
                      <Pencil className="h-3.5 w-3.5" />
                      {t('storyStage.opening.startCustom')}
                    </Button>
                  </div>
                </div>
              </div>
            ) : (
              <MessageList
                messages={messages}
                isStreaming={streaming}
                activityContent={activityContent}
                highlightDialogue
                collapseTraceBeforeAssistant
                scrollResetKey={scrollResetKey}
                bottomPaddingClassName="pb-36"
                bottomPaddingPx={messageListBottomPadding}
                messageStyle={stageTextStyle}
                onEditMessage={startEditingMessage}
                onRegenerateMessage={regenerateMessage}
                onSwitchMessageVersion={switchMessageVersion}
                onOpenSubAgentSession={openSubAgentSession}
                activeSubAgentSessionKey={activeSubAgentSessionKey}
              />
            )}
            {activeSubAgentSessionKey && (
              <div className="absolute inset-y-0 right-0 z-30 w-[min(420px,92vw)] border-l border-[var(--nova-border)] shadow-[var(--nova-shadow)]">
                <SubAgentSessionPanel
                  messages={messages}
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
      <div ref={inputFloatRef} className="nova-story-input-float pointer-events-none absolute inset-x-0 bottom-0 z-20 p-3">
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
          {showHotChoices ? (
            <div className="mb-2 overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
              <div className="flex min-h-8 items-center gap-1.5 px-2 py-1 text-[11px] text-[var(--nova-text-muted)]">
                <button type="button" className="nova-nav-item flex min-w-0 flex-1 items-center gap-1.5 rounded-[var(--nova-radius)] px-1.5 py-1 text-left hover:bg-[var(--nova-hover)]" onMouseDown={(event) => event.preventDefault()} onClick={() => setHotChoicesExpanded((value) => !value)} aria-expanded={hotChoicesExpanded}>
                  <Compass className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                  <span className="shrink-0 font-medium text-[var(--nova-text-muted)]">{t('storyStage.hotChoices.title')}</span>
                  <span className="min-w-0 flex-1 truncate text-[var(--nova-text-faint)]">
                    {hotChoicesLoading && hotChoices.length === 0
                      ? t('storyStage.hotChoices.generating')
                      : hotChoices.length > 0
                        ? t('storyStage.hotChoices.count', {
                            count: hotChoices.length,
                          })
                        : t('storyStage.hotChoices.emptyShort')}
                  </span>
                  {hotChoicesExpanded ? <ChevronUp className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" /> : <ChevronDown className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />}
                </button>
                {!hotChoicesLoading && (hotChoices.length === 0 || hotChoices.length < 10) ? (
                  <button type="button" className="nova-nav-item inline-flex h-7 shrink-0 items-center gap-1 rounded-[var(--nova-radius)] px-2 text-[11px] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:opacity-50" onMouseDown={(event) => event.preventDefault()} onClick={() => requestHotChoices(hotChoices.length > 0)}>
                    <RefreshCw className="h-3 w-3" />
                    {hotChoices.length > 0 ? t('storyStage.hotChoices.more') : t('storyStage.hotChoices.generate')}
                  </button>
                ) : null}
              </div>
              {hotChoicesExpanded ? (
                <div className="border-t border-[var(--nova-border)] px-2 py-2">
                  {hotChoicesLoading && hotChoices.length === 0 ? (
                    <div className="px-1 py-1 text-xs text-[var(--nova-text-faint)]">{t('storyStage.hotChoices.generatingLong')}</div>
                  ) : hotChoices.length === 0 ? (
                    <div className="px-1 py-1 text-xs text-[var(--nova-text-faint)]">{t('storyStage.hotChoices.emptyLong')}</div>
                  ) : (
                    <div className="flex max-h-48 flex-col gap-1.5 overflow-y-auto overscroll-contain pr-1">
                      {hotChoices.map((choice, index) => (
                        <button
                          key={`${index}-${choice}`}
                          type="button"
                          className="w-full max-w-full rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1 text-left text-xs leading-5 text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
                          onMouseDown={(event) => event.preventDefault()}
                          onClick={() => {
                            setInput(choice)
                            setShowSkillCommands(false)
                            setActiveSkillCommandIndex(0)
                            setHotChoicesExpanded(false)
                            window.requestAnimationFrame(() => {
                              inputRef.current?.focus()
                              inputRef.current?.setSelectionRange(choice.length, choice.length)
                            })
                          }}
                        >
                          {choice}
                        </button>
                      ))}
                    </div>
                  )}
                </div>
              ) : null}
            </div>
          ) : null}
          <div className="relative min-w-0">
            <ReferenceChips files={styleScenes} onRemove={removeStyleScene} prefix="#" tone="style" />
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
                      <CommandGroup heading={t('chat.commands.group')} className="[&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:pb-1 [&_[cmdk-group-heading]]:pt-1 [&_[cmdk-group-heading]]:text-[11px] [&_[cmdk-group-heading]]:text-[var(--nova-text-faint)]">
                        {filteredSkillCommands.map((skill, index) => {
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
                    </CommandList>
                  </Command>
                </PopoverContent>
              </Popover>
            <AgentComposerShell
              className="nova-story-stage-composer"
              input={
                <Textarea
                  ref={inputRef}
                  autoResize
                  multilineMode="sticky-until-empty"
                  className="nova-agent-composer-textarea min-h-[42px] resize-none border-0 bg-transparent px-1 py-[9px] text-sm leading-6 text-[var(--nova-text)] shadow-none placeholder:text-[var(--nova-text-faint)] focus-visible:border-transparent focus-visible:ring-0"
                  style={inputTextStyle}
                  value={input}
                  placeholder={!isMobile && skillCommands.length > 0 ? t('storyStage.inputPlaceholderWithSkills') : t('storyStage.inputPlaceholder')}
                  onChange={handleInputChange}
                  onKeyDown={(event) => {
                    const canPickSkill = showSkillCommands && filteredSkillCommands.length > 0
                    if (canPickSkill && (event.key === 'ArrowDown' || event.key === 'ArrowUp')) {
                      event.preventDefault()
                      setActiveSkillCommandIndex((current) => {
                        const direction = event.key === 'ArrowDown' ? 1 : -1
                        return (current + direction + filteredSkillCommands.length) % filteredSkillCommands.length
                      })
                      return
                    }
                    if (event.key === 'Escape') {
                      setStyleSceneQuery(null)
                      setShowSkillCommands(false)
                      setActiveSkillCommandIndex(0)
                      return
                    }
                    if (canPickSkill && event.key === 'Tab') {
                      event.preventDefault()
                      selectSkillCommand(filteredSkillCommands[activeSkillCommandIndex]?.name || filteredSkillCommands[0].name)
                      return
                    }
                    if (event.key === 'Enter' && !event.shiftKey) {
                      if (isComposingKeyboardEvent(event)) return
                      event.preventDefault()
                      if (canPickSkill) {
                        selectSkillCommand(filteredSkillCommands[activeSkillCommandIndex]?.name || filteredSkillCommands[0].name)
                        return
                      }
                      void send()
                    }
                  }}
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
                        disabled={streaming || (!storyId && tokenUsageMessages.length === 0 && !workspace)}
                        aria-label={t('chat.input.actions')}
                        title={t('chat.input.actions')}
                      >
                        <List className="h-3.5 w-3.5" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="start" side="top" className="w-80 border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2 text-[var(--nova-text)]">
                      <ModelProfileSwitcher agentKey="interactive_story" workspace={workspace} disabled={streaming} />
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
                        disabled={!storyId || streaming}
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
                  {stagePreferences.hotChoicesEnabled ? (
                    <Button type="button" variant="outline" className={`nova-agent-composer-pill h-8 shrink-0 rounded-[10px] border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 text-[11px] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] ${hotChoicesExpanded ? 'text-[var(--nova-text)]' : ''}`} disabled={!storyId || streaming || Boolean(editingTurn)} onMouseDown={(event) => event.preventDefault()} onClick={toggleHotChoices} aria-label={hotChoicesExpanded ? t('storyStage.hotChoices.collapse') : t('storyStage.hotChoices.get')} title={hotChoicesExpanded ? t('storyStage.hotChoices.collapse') : t('storyStage.hotChoices.get')}>
                      <Compass className={`h-3.5 w-3.5 ${hotChoicesLoading ? 'animate-pulse' : ''}`} />
                      {!isMobile ? t('storyStage.hotChoices.button') : null}
                    </Button>
                  ) : null}
                  {isMobile ? (
                    <Button type="button" variant="outline" className="nova-agent-composer-icon h-8 w-8 shrink-0 rounded-[10px] border-[var(--nova-border)] bg-[var(--nova-surface)] px-0 text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" onMouseDown={(event) => event.preventDefault()} onClick={openMobileNavigation} aria-label={t('workbench.mobile.navigationMenu')} title={t('workbench.mobile.navigationMenu')}>
                      <Plus className="h-3.5 w-3.5" />
                    </Button>
                  ) : null}
                </>
              }
              submitControl={
                <Button
                  className={`nova-agent-composer-submit h-9 w-9 shrink-0 rounded-[10px] px-0 text-[var(--nova-text)] shadow-[inset_0_1px_0_rgba(255,255,255,0.08)] ${streaming ? 'bg-[var(--nova-danger-bg)] hover:bg-[var(--nova-danger-bg)]' : 'bg-[var(--nova-active)] hover:bg-[var(--nova-hover)]'}`}
                  disabled={streaming ? false : !storyId || !input.trim()}
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
          <TokenUsageDialog open={tokenUsageOpen} messages={tokenUsageMessages} onOpenChange={setTokenUsageOpen} />
        </div>
      </div>
    </main>
  )

  function appendAssistantMessage(content: string, metadata: Partial<ChatMessage> = {}) {
    if (!content) return
    setStageLiveMessages((prev) => {
      const last = prev[prev.length - 1]
      if (last?.role === 'assistant' && last.streaming && sameLiveMessageSource(last, metadata)) {
        return [...prev.slice(0, -1), { ...last, content: `${last.content || ''}${content}` }]
      }
      return [...prev, { role: 'assistant', content, streaming: true, ...metadata }]
    })
  }

  function appendThinkingMessage(content: string, metadata: Partial<ChatMessage> = {}) {
    if (!content) return
    setStageLiveMessages((prev) => {
      const last = prev[prev.length - 1]
      if (last?.role === 'thinking' && sameLiveMessageSource(last, metadata)) {
        return [
          ...prev.slice(0, -1),
          {
            ...last,
            content: `${last.content || ''}${content}`,
            streaming: true,
          },
        ]
      }
      return [...prev, { role: 'thinking', content, streaming: true, ...metadata }]
    })
  }

  function appendToolCallMessage(payload: Record<string, unknown> & { id?: string; name?: string; args?: string }) {
    const id = payload.id || `tool-${Date.now()}-${Math.random().toString(16).slice(2)}`
    const name = payload.name || 'unknown_tool'
    const metadata = streamMetadataFromPayload(payload)
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

  function appendToolArgsDelta(payload: { id?: string; name?: string; args?: string; delta?: string }) {
    if (!payload.id && !payload.name) return
    setStageLiveMessages((prev) => {
      const targetIndex = findToolMessageIndex(prev, payload.id, payload.name)
      if (targetIndex < 0) return prev
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

  function updateToolCallMessage(id: string | undefined, name: string | undefined, status: 'success' | 'error', result = '') {
    setStageLiveMessages((prev) => {
      const targetIndex = findToolMessageIndex(prev, id, name)
      if (targetIndex < 0) return prev
      return prev.map((msg, index) => (
        index === targetIndex ? { ...msg, status, result, streaming: false } : msg
      ))
    })
  }

  function findToolMessageIndex(messages: ChatMessage[], id?: string, name?: string) {
    if (id) {
      for (let i = messages.length - 1; i >= 0; i--) {
        const message = messages[i]
        if (message.role === 'tool_call' && message.id === id) return i
      }
      return -1
    }
    if (name) {
      let match = -1
      for (let i = messages.length - 1; i >= 0; i--) {
        const message = messages[i]
        if (message.role === 'tool_call' && message.name === name) {
          if (match >= 0) return -1
          match = i
        }
      }
      return match
    }
    if (!id && !name) {
      for (let i = messages.length - 1; i >= 0; i--) {
        if (messages[i].role === 'tool_call') return i
      }
    }
    return -1
  }

  function appendContextCompactionMessage(data: Record<string, unknown>) {
    const compactionId = currentCompactionMessageIdRef.current || createContextCompactionMessageId(compactionIdCounterRef)
    currentCompactionMessageIdRef.current = compactionId
    setStageLiveMessages((prev) => upsertContextCompactionMessage(prev, buildContextCompactionMessage(data, compactionId)))
  }

  function collapseNonNarrativeMessages() {
    setStageLiveMessages((prev) =>
      prev.map((msg) =>
        msg.role === 'thinking' || msg.role === 'tool_call' || msg.role === 'context_compaction'
          ? {
              ...msg,
              streaming: false,
              status: msg.role === 'tool_call' || msg.role === 'context_compaction' ? (msg.status === 'running' ? 'success' : msg.status) : msg.status,
            }
          : msg,
      ),
    )
  }

  function finishLiveMessages() {
    setStageLiveMessages((prev) =>
      prev.map((msg) =>
        msg.role === 'assistant' || msg.role === 'thinking' || msg.role === 'tool_call' || msg.role === 'context_compaction'
          ? {
              ...msg,
              streaming: false,
              status: msg.role === 'tool_call' || msg.role === 'context_compaction' ? (msg.status === 'running' ? 'success' : msg.status) : msg.status,
            }
          : msg,
      ),
    )
  }
}

function streamMetadataFromPayload(payload: Record<string, unknown>): Partial<ChatMessage> {
  const runPath = Array.isArray(payload.run_path) ? payload.run_path.filter((item): item is string => typeof item === 'string') : undefined
  return {
    run_id: typeof payload.run_id === 'string' ? payload.run_id : undefined,
    agent_name: typeof payload.agent_name === 'string' ? payload.agent_name : undefined,
    root_agent_name: typeof payload.root_agent_name === 'string' ? payload.root_agent_name : undefined,
    run_path: runPath,
    subagent: readStreamBool(payload.subagent),
    subagent_session_id: typeof payload.subagent_session_id === 'string' ? payload.subagent_session_id : undefined,
    subagent_type: typeof payload.subagent_type === 'string' ? payload.subagent_type : undefined,
  }
}

function sameLiveMessageSource(message: ChatMessage, metadata: Partial<ChatMessage>) {
  if (Boolean(message.subagent) !== Boolean(metadata.subagent)) return false
  if (message.subagent || metadata.subagent) {
    return subAgentSessionKey(message) === subAgentSessionKey(metadata)
  }
  return true
}

function readStreamBool(value: unknown) {
  if (typeof value === 'boolean') return value
  if (typeof value === 'string') return value === 'true'
  return false
}

function ReplyTargetCharsControl({ story, onChange }: { story?: StorySummary; onChange?: (replyTargetChars: number) => void | Promise<void> }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState(String(normalizeReplyTargetChars(story?.reply_target_chars)))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const currentValue = normalizeReplyTargetChars(story?.reply_target_chars)

  useEffect(() => {
    if (!open) {
      setDraft(String(currentValue))
      setError('')
    }
  }, [currentValue, open])

  const save = async () => {
    const nextValue = Number(draft)
    if (!Number.isFinite(nextValue) || nextValue <= 0) {
      setError(t('storyStage.replyTarget.invalid'))
      return
    }
    setSaving(true)
    setError('')
    try {
      await onChange?.(Math.floor(nextValue))
      setOpen(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('storyStage.replyTarget.saveFailed'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" size="sm" disabled={!story || !onChange} className="h-7 gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-[11px] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" aria-label={t('storyStage.replyTarget.open')}>
          <Pencil className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
          {t('storyStage.replyTarget.compact', { count: currentValue })}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" sideOffset={6} className="nova-panel w-64 border border-[var(--nova-border)] p-3 text-[var(--nova-text)] shadow-[var(--nova-shadow)]">
        <div className="mb-2 text-xs font-medium">{t('storyStage.replyTarget.title')}</div>
        <Input
          className="nova-field text-xs"
          type="number"
          min={1}
          value={draft}
          onChange={(event) => {
            setDraft(event.target.value)
            setError('')
          }}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault()
              void save()
            }
          }}
        />
        {error && <div className="mt-2 text-[11px] leading-4 text-[var(--nova-danger)]">{error}</div>}
        <div className="mt-3 flex justify-end gap-2">
          <Button variant="ghost" size="xs" onClick={() => setOpen(false)}>
            {t('common.cancel')}
          </Button>
          <Button size="xs" disabled={saving} onClick={() => void save()}>
            {saving && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
            {t('common.save')}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}

function normalizeReplyTargetChars(value?: number) {
  return value && value > 0 ? value : DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS
}

function noop() {}

function useStagePreferences() {
  const [preferences, setPreferences] = useState({
    lineHeight: DEFAULT_STAGE_LINE_HEIGHT,
    hotChoicesEnabled: true,
  })

  const load = useCallback(async () => {
    try {
      const settings = await fetchSettings()
      const effective = settings.effective || {}
      setPreferences({
        lineHeight: clampNumber(effective.interactive_stage_line_height, 1.35, 2.4, DEFAULT_STAGE_LINE_HEIGHT),
        hotChoicesEnabled: effective.interactive_hot_choices_enabled !== false,
      })
    } catch (error) {
      console.warn('[interactive-stage] 加载故事舞台显示设置失败', error)
      setPreferences({
        lineHeight: DEFAULT_STAGE_LINE_HEIGHT,
        hotChoicesEnabled: true,
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

function mergeHotChoices(current: string[], next: string[]) {
  const merged: string[] = []
  const seen = new Set<string>()
  for (const choice of [...current, ...next]) {
    const normalized = choice.trim()
    if (!normalized || seen.has(normalized)) continue
    merged.push(normalized)
    seen.add(normalized)
    if (merged.length >= 10) break
  }
  return merged
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

function pickSceneTitle(snapshot: Snapshot | null, branchId: string, t: (key: string) => string) {
  const current = snapshot?.graph?.nodes?.find((node) => node.current && node.branch_id === (snapshot.branch_id || branchId)) || snapshot?.graph?.nodes?.find((node) => node.head && node.branch_id === (snapshot.branch_id || branchId))
  if (current?.title) return current.title
  return t('storyStage.primaryArea')
}
