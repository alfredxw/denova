import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, ChangeEvent } from 'react'
import { ChevronDown, ChevronUp, Command as CommandIcon, Compass, PanelRight, Pencil, RefreshCw, Send, Sparkles, Square, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Command, CommandEmpty, CommandGroup, CommandItem, CommandList } from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { FileReferencePicker } from '@/components/Chat/FileReferencePicker'
import { MessageList } from '@/components/Chat/MessageList'
import { ReferenceChips } from '@/components/Chat/ReferenceChips'
import type { ChatMessage } from '@/lib/api'
import { fetchSettings } from '@/features/settings/api'
import { fontStackFor } from '@/features/settings/font-options'
import { useSkillCommands } from '@/hooks/useSkillCommands'
import { abortInteractiveChat, generateInteractiveHotChoices, sendInteractiveMessage, switchInteractiveTurnVersion } from '../api'
import { createInteractiveNarrativeFilter } from '../stream-parser'
import { emptyStoryStageRun, useInteractiveStore } from '../stores/interactive-store'
import type { StoryStageRunState } from '../stores/interactive-store'
import type { Snapshot, StorySummary, Teller } from '../types'
import { StoryPicker } from './StoryPicker'
import { TellerPicker } from './TellerPicker'

interface StoryStageProps {
  workspace?: string
  styleSuggestions?: string[]
  stories?: StorySummary[]
  story?: StorySummary
  tellers?: Teller[]
  storyId: string
  branchId: string
  snapshot: Snapshot | null
  snapshotLoading?: boolean
  loreEmpty?: boolean
  sceneMemoryVisible?: boolean
  onStorySelect?: (storyId: string) => void
  onStoryCreate?: (input: { title: string; origin: string; story_teller_id: string; reply_target_chars: number }) => void
  onStoryDelete?: (storyId: string) => void
  onTellerChange?: (tellerId: string) => void
  onReplyTargetCharsChange?: (replyTargetChars: number) => void | Promise<void>
  onRequestLoreInit?: () => void
  onToggleSceneMemory?: () => void
  onDone: () => void | Promise<Snapshot | void>
}

const DEFAULT_READING_FONT_SIZE = 18
const DEFAULT_STAGE_LINE_HEIGHT = 1.78
const DEFAULT_READING_FONT = 'source-han-serif'
const EMPTY_STAGE_RUN = emptyStoryStageRun()
const stageAbortControllers = new Map<string, AbortController>()

export function StoryStage({ workspace, styleSuggestions = [], stories = [], story, tellers = [], storyId, branchId, snapshot, snapshotLoading = false, loreEmpty = false, sceneMemoryVisible = true, onStorySelect = noop, onStoryCreate = noop, onStoryDelete = noop, onTellerChange = noop, onReplyTargetCharsChange, onRequestLoreInit, onToggleSceneMemory, onDone }: StoryStageProps) {
  const { t } = useTranslation()
  const [input, setInput] = useState('')
  const [styleReferences, setStyleReferences] = useState<string[]>([])
  const [styleReferenceQuery, setStyleReferenceQuery] = useState<string | null>(null)
  const [showSkillCommands, setShowSkillCommands] = useState(false)
  const [activeSkillCommandIndex, setActiveSkillCommandIndex] = useState(0)
  const inputRef = useRef<HTMLTextAreaElement | null>(null)
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
  const hotChoicesAbortRef = useRef<AbortController | null>(null)
  const liveStageKeyRef = useRef(stageKey)
  const previousSnapshotKeyRef = useRef(snapshotKey)
  const stagePreferences = useStagePreferences()
  const stageTextStyle = useMemo<CSSProperties>(
    () => ({
      fontSize: `${stagePreferences.fontSize}px`,
      lineHeight: stagePreferences.lineHeight,
      fontFamily: stagePreferences.fontFamily,
    }),
    [stagePreferences.fontFamily, stagePreferences.fontSize, stagePreferences.lineHeight],
  )
  const inputTextStyle = useMemo<CSSProperties>(
    () => ({
      fontSize: `${Math.min(stagePreferences.fontSize, 16)}px`,
      lineHeight: 1.35,
      fontFamily: stagePreferences.fontFamily,
    }),
    [stagePreferences.fontFamily, stagePreferences.fontSize],
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
    return skillCommands.filter((skill) => skill.name.toLowerCase().startsWith(query))
  }, [input, skillCommands])

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
      if (turn.thinking?.trim()) {
        messages.push({
          id: `${turn.id}-thinking`,
          role: 'thinking',
          content: turn.thinking,
          streaming: false,
        })
      }
      for (const [index, event] of (turn.display_events || []).entries()) {
        if (event.role !== 'tool_call') continue
        messages.push({
          id: event.id || `${turn.id}-tool-${index}`,
          role: 'tool_call',
          content: event.content || event.name || 'unknown_tool',
          name: event.name || event.content,
          status: event.status || 'success',
          streaming: false,
          created_at: event.created_at,
        })
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

  const displayLiveMessages = hasPersistedLiveTurn ? [] : liveMessages
  const messages = useMemo(() => [...historyMessages, ...displayLiveMessages], [displayLiveMessages, historyMessages])
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
  const turnsById = useMemo(() => {
    const result = new Map<string, { user: string }>()
    for (const turn of snapshot?.turns || []) {
      result.set(turn.id, { user: turn.user })
    }
    return result
  }, [snapshot?.turns])

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

  const send = async (override?: { message?: string; rewindTurnId?: string }) => {
    const sourceMessage = override?.message ?? input
    const message = sourceMessage.trim()
    if (!message || !storyId || streaming) return
    const nextRewindTurnId = override?.rewindTurnId ?? editingTurn?.id
    const inlineStyleReferences = parseInlineStyleReferences(message)
    const mergedStyleReferences = Array.from(new Set([...styleReferences, ...inlineStyleReferences]))
    setInput('')
    setEditingTurn(null)
    setStyleReferences([])
    setStyleReferenceQuery(null)
    setShowSkillCommands(false)
    setActiveSkillCommandIndex(0)
    setStageActivityContent(t('storyStage.activity.connecting'))
    setStageLiveMessages([{ role: 'user', content: message }])
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
        style_references: mergedStyleReferences,
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
            appendThinkingMessage(data.content || '')
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
      setStageActivityContent('')
    }
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
    setStyleReferenceQuery(null)
    setShowSkillCommands(false)
    setActiveSkillCommandIndex(0)
  }

  const handleInputChange = (event: ChangeEvent<HTMLTextAreaElement>) => {
    const nextValue = event.target.value
    setInput(nextValue)
    setShowSkillCommands(nextValue.startsWith('/'))
    setActiveSkillCommandIndex(0)
    const styleMatch = nextValue.match(/(?:^|\s)#([^\s#]*)$/)
    setStyleReferenceQuery(styleMatch ? styleMatch[1] : null)
  }

  const selectSkillCommand = (name: string) => {
    setInput(`/${name} `)
    setShowSkillCommands(false)
    setActiveSkillCommandIndex(0)
    inputRef.current?.focus()
  }

  const selectStyleReference = (path: string) => {
    setInput((current) =>
      current.replace(/(?:^|\s)#([^\s#]*)$/, (match) => {
        const prefix = match.startsWith(' ') ? ' ' : ''
        return `${prefix}#${path} `
      }),
    )
    setStyleReferences((current) => Array.from(new Set([...current, path])))
    setStyleReferenceQuery(null)
    inputRef.current?.focus()
  }

  const removeStyleReference = (path: string) => {
    setStyleReferences((current) => current.filter((item) => item !== path))
  }

  return (
    <main className="relative flex h-full min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-[var(--nova-surface-2)]">
      <div data-testid="story-stage-card" className="flex min-h-0 flex-1 flex-col overflow-hidden bg-[var(--nova-surface-2)]">
        <div className="nova-topbar flex min-h-14 flex-wrap items-center justify-between gap-3 border-b px-4 py-2">
          <div className="min-w-0">
            <div className="text-[10px] font-medium leading-4 text-[var(--nova-text-faint)]">{t('storyStage.branchLabel', { branch: branchId || 'main' })}</div>
            <div className="truncate text-xs font-semibold leading-5 text-[var(--nova-text)]">{title}</div>
          </div>
          <div className="flex min-w-0 flex-wrap items-center justify-end gap-2">
            <StoryPicker stories={stories} currentStoryId={storyId} tellers={tellers} onSelect={onStorySelect} onCreate={onStoryCreate} onDelete={onStoryDelete} />
            <TellerPicker story={story} tellers={tellers} onChange={onTellerChange} />
            <ReplyTargetCharsControl story={story} onChange={onReplyTargetCharsChange} />
            {onToggleSceneMemory && (
              <Button type="button" variant="outline" size="sm" className={`h-7 gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-[11px] hover:bg-[var(--nova-hover)] ${sceneMemoryVisible ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'}`} onClick={onToggleSceneMemory} aria-label={sceneMemoryVisible ? t('storyStage.hideSceneMemory') : t('storyStage.showSceneMemory')} title={sceneMemoryVisible ? t('storyStage.hideSceneMemory') : t('storyStage.showSceneMemory')}>
                <PanelRight className="h-3.5 w-3.5" />
                {t('storyStage.sceneMemory')}
              </Button>
            )}
          </div>
        </div>

        <div className="flex min-h-0 flex-1 overflow-hidden bg-[var(--nova-surface-2)]">
          <section className="flex min-h-0 min-w-0 flex-1 flex-col bg-[var(--nova-surface-2)]">
            {snapshotLoading && messages.length === 0 && !streaming ? (
              <div className="m-5 flex min-h-0 flex-1 items-center justify-center rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-6 text-center text-sm text-[var(--nova-text-faint)] shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]">
                <div className="flex max-w-md flex-col items-center gap-3">
                  <RefreshCw className="h-4 w-4 animate-spin text-[var(--nova-text-muted)]" />
                  <div className="text-xs leading-5 text-[var(--nova-text-faint)]">{t('common.loading')}</div>
                </div>
              </div>
            ) : messages.length === 0 && !streaming ? (
              <div className="m-5 flex min-h-0 flex-1 items-center justify-center rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-6 text-center text-sm text-[var(--nova-text-faint)] shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]">
                {loreEmpty && onRequestLoreInit ? (
                  <div className="flex max-w-md flex-col items-center gap-3">
                    <Sparkles className="h-4 w-4 text-[var(--nova-text-muted)]" />
                    <div className="space-y-1">
                      <div className="text-xs text-[var(--nova-text-faint)]">{t('storyStage.empty')}</div>
                      <div className="text-sm font-medium text-[var(--nova-text)]">{t('loreInit.interactiveTitle')}</div>
                      <div className="text-xs leading-5 text-[var(--nova-text-faint)]">{t('loreInit.interactiveDescription')}</div>
                    </div>
                    <button type="button" className="nova-nav-item rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-1.5 text-xs text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" onClick={onRequestLoreInit}>
                      {t('loreInit.openAgent')}
                    </button>
                  </div>
                ) : (
                  t('storyStage.empty')
                )}
              </div>
            ) : (
              <MessageList messages={messages} isStreaming={streaming} activityContent={activityContent} highlightDialogue collapseTraceBeforeAssistant scrollResetKey={scrollResetKey} bottomPaddingClassName="pb-6" messageStyle={stageTextStyle} onEditMessage={startEditingMessage} onRegenerateMessage={regenerateMessage} onSwitchMessageVersion={switchMessageVersion} />
            )}
          </section>
        </div>
      </div>
      <div className="shrink-0 border-t border-[var(--nova-border)] bg-[var(--nova-surface)] p-3">
        <div className="mx-auto max-w-5xl">
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
                    <div className="flex max-h-24 flex-wrap gap-1.5 overflow-y-auto pr-1">
                      {hotChoices.map((choice, index) => (
                        <button
                          key={`${index}-${choice}`}
                          type="button"
                          className="max-w-full rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1 text-left text-xs leading-5 text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
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
          <div className="flex items-center gap-3">
            <div className="relative min-w-0 flex-1">
              <ReferenceChips files={styleReferences} onRemove={removeStyleReference} prefix="#" tone="style" />
              <FileReferencePicker open={styleReferenceQuery !== null && styleSuggestions.length > 0} query={styleReferenceQuery || ''} files={styleSuggestions} onSelect={selectStyleReference} trigger="#" placeholder={t('chat.styleReference.placeholder')} emptyText={t('chat.styleReference.empty')} heading={t('chat.styleReference.heading')} />
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
                          <div className="text-[11px] text-[var(--nova-text-faint)]">{t('chat.commands.skillsDescription')}</div>
                        </div>
                      </div>
                    </div>
                    <CommandList className="max-h-[312px] p-1.5">
                      <CommandEmpty className="py-5 text-center text-xs text-[var(--nova-text-faint)]">{t('chat.commands.empty')}</CommandEmpty>
                      <CommandGroup heading={t('chat.commands.skillsGroup')} className="[&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:pb-1 [&_[cmdk-group-heading]]:pt-1 [&_[cmdk-group-heading]]:text-[11px] [&_[cmdk-group-heading]]:text-[var(--nova-text-faint)]">
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
                                <Sparkles className="h-3.5 w-3.5" />
                              </span>
                              <span className="min-w-0 flex-1">
                                <span className="flex items-center gap-2">
                                  <span className="font-mono text-xs text-[var(--nova-text)]">/{skill.name}</span>
                                  <span className="truncate text-xs text-[var(--nova-text-muted)]">{skill.description || skill.name}</span>
                                </span>
                                <span className="mt-0.5 block text-[11px] text-[var(--nova-text-faint)]">{t('chat.command.skill.hint')}</span>
                              </span>
                            </CommandItem>
                          )
                        })}
                      </CommandGroup>
                    </CommandList>
                  </Command>
                </PopoverContent>
              </Popover>
              <Textarea
                ref={inputRef}
                autoResize
                className="nova-field min-h-11 flex-1 resize-none px-3 py-2 text-sm placeholder:text-[var(--nova-text-faint)] focus-visible:ring-1 focus-visible:ring-[var(--nova-border)]/35"
                style={inputTextStyle}
                value={input}
                placeholder={skillCommands.length > 0 ? t('storyStage.inputPlaceholderWithSkills') : t('storyStage.inputPlaceholder')}
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
                    setStyleReferenceQuery(null)
                    setShowSkillCommands(false)
                    setActiveSkillCommandIndex(0)
                    return
                  }
                  if (event.key === 'Enter' && !event.shiftKey) {
                    event.preventDefault()
                    if (canPickSkill) {
                      selectSkillCommand(filteredSkillCommands[activeSkillCommandIndex]?.name || filteredSkillCommands[0].name)
                      return
                    }
                    void send()
                  }
                }}
              />
            </div>
            {stagePreferences.hotChoicesEnabled ? (
              <Button type="button" variant="outline" className={`h-11 w-14 shrink-0 border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 text-xs text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] ${hotChoicesExpanded ? 'text-[var(--nova-text)]' : ''}`} disabled={!storyId || streaming || Boolean(editingTurn)} onMouseDown={(event) => event.preventDefault()} onClick={toggleHotChoices} aria-label={hotChoicesExpanded ? t('storyStage.hotChoices.collapse') : t('storyStage.hotChoices.get')} title={hotChoicesExpanded ? t('storyStage.hotChoices.collapse') : t('storyStage.hotChoices.get')}>
                <Compass className={`h-3.5 w-3.5 ${hotChoicesLoading ? 'animate-pulse' : ''}`} />
                {t('storyStage.hotChoices.button')}
              </Button>
            ) : null}
            <Button
              className={`h-11 w-20 border border-[var(--nova-border)] text-[var(--nova-text)] shadow-[inset_0_1px_0_rgba(255,255,255,0.08)] ${streaming ? 'bg-[var(--nova-danger-bg)] hover:bg-[var(--nova-danger-bg)]' : 'bg-[var(--nova-active)] hover:bg-[var(--nova-hover)]'}`}
              disabled={streaming ? false : !storyId || !input.trim()}
              onClick={() => {
                streaming ? stop() : void send()
              }}
              aria-label={streaming ? t('chat.input.stop') : editingTurn ? t('storyStage.sendRegenerate') : t('chat.input.send')}
            >
              {streaming ? <Square className="h-3.5 w-3.5 fill-current" /> : editingTurn ? <RefreshCw className="h-3.5 w-3.5" /> : <Send className="h-3.5 w-3.5" />}
              {streaming ? t('storyStage.stop') : editingTurn ? t('storyStage.regenerate') : t('chat.input.send')}
            </Button>
          </div>
        </div>
      </div>
    </main>
  )

  function appendAssistantMessage(content: string) {
    if (!content) return
    setStageLiveMessages((prev) => {
      const last = prev[prev.length - 1]
      if (last?.role === 'assistant' && last.streaming) {
        return [...prev.slice(0, -1), { ...last, content: `${last.content || ''}${content}` }]
      }
      return [...prev, { role: 'assistant', content, streaming: true }]
    })
  }

  function appendThinkingMessage(content: string) {
    if (!content) return
    setStageLiveMessages((prev) => {
      const last = prev[prev.length - 1]
      if (last?.role === 'thinking') {
        return [
          ...prev.slice(0, -1),
          {
            ...last,
            content: `${last.content || ''}${content}`,
            streaming: true,
          },
        ]
      }
      return [...prev, { role: 'thinking', content, streaming: true }]
    })
  }

  function appendToolCallMessage(payload: { id?: string; name?: string; args?: string }) {
    const id = payload.id || `tool-${Date.now()}-${Math.random().toString(16).slice(2)}`
    const name = payload.name || 'unknown_tool'
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
      },
    ])
  }

  function appendToolArgsDelta(payload: { id?: string; args?: string; delta?: string }) {
    if (!payload.id) return
    setStageLiveMessages((prev) =>
      prev.map((msg) =>
        msg.role === 'tool_call' && msg.id === payload.id
          ? {
              ...msg,
              args: payload.args !== undefined ? payload.args : `${msg.args || ''}${payload.delta || ''}`,
            }
          : msg,
      ),
    )
  }

  function updateToolCallMessage(id: string | undefined, name: string | undefined, status: 'success' | 'error', result = '') {
    setStageLiveMessages((prev) =>
      prev.map((msg) => {
        if (msg.role !== 'tool_call') return msg
        const matched = id ? msg.id === id : Boolean(name && msg.name === name)
        if (!matched) return msg
        return { ...msg, status, result, streaming: false }
      }),
    )
  }

  function collapseNonNarrativeMessages() {
    setStageLiveMessages((prev) =>
      prev.map((msg) =>
        msg.role === 'thinking' || msg.role === 'tool_call'
          ? {
              ...msg,
              streaming: false,
              status: msg.role === 'tool_call' ? (msg.status === 'running' ? 'success' : msg.status) : msg.status,
            }
          : msg,
      ),
    )
  }

  function finishLiveMessages() {
    setStageLiveMessages((prev) =>
      prev.map((msg) =>
        msg.role === 'assistant' || msg.role === 'thinking' || msg.role === 'tool_call'
          ? {
              ...msg,
              streaming: false,
              status: msg.role === 'tool_call' ? (msg.status === 'running' ? 'success' : msg.status) : msg.status,
            }
          : msg,
      ),
    )
  }
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
            {saving ? t('common.saving') : t('common.save')}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}

function normalizeReplyTargetChars(value?: number) {
  return value && value > 0 ? value : 1200
}

function noop() {}

function useStagePreferences() {
  const [preferences, setPreferences] = useState({
    fontSize: DEFAULT_READING_FONT_SIZE,
    lineHeight: DEFAULT_STAGE_LINE_HEIGHT,
    fontFamily: fontStackFor(DEFAULT_READING_FONT, DEFAULT_READING_FONT),
    hotChoicesEnabled: true,
  })

  const load = useCallback(async () => {
    try {
      const settings = await fetchSettings()
      const effective = settings.effective || {}
      setPreferences({
        fontSize: clampNumber(effective.reading_font_size, 14, 28, DEFAULT_READING_FONT_SIZE),
        lineHeight: clampNumber(effective.interactive_stage_line_height, 1.35, 2.4, DEFAULT_STAGE_LINE_HEIGHT),
        fontFamily: fontStackFor(effective.reading_font_family, DEFAULT_READING_FONT),
        hotChoicesEnabled: effective.interactive_hot_choices_enabled !== false,
      })
    } catch (error) {
      console.warn('[interactive-stage] 加载故事舞台显示设置失败', error)
      setPreferences({
        fontSize: DEFAULT_READING_FONT_SIZE,
        lineHeight: DEFAULT_STAGE_LINE_HEIGHT,
        fontFamily: fontStackFor(DEFAULT_READING_FONT, DEFAULT_READING_FONT),
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

function parseInlineStyleReferences(input: string): string[] {
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
