import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import { Compass, GitBranch, MessageSquareText, Send, Square } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { MessageList } from '@/components/Chat/MessageList'
import type { ChatMessage } from '@/lib/api'
import { fetchSettings } from '@/features/settings/api'
import { fontStackFor } from '@/features/settings/font-options'
import { abortInteractiveChat, sendInteractiveMessage } from '../api'
import { createInteractiveNarrativeFilter } from '../stream-parser'
import { emptyStoryStageRun, useInteractiveStore } from '../stores/interactive-store'
import type { StoryStageRunState } from '../stores/interactive-store'
import type { Snapshot } from '../types'

interface StoryStageProps {
  workspace?: string
  storyId: string
  branchId: string
  snapshot: Snapshot | null
  onDone: () => void
}

const DEFAULT_STAGE_FONT_SIZE = 16
const DEFAULT_STAGE_LINE_HEIGHT = 1.78
const DEFAULT_READING_FONT = 'source-han-serif'
const EMPTY_STAGE_RUN = emptyStoryStageRun()
const stageAbortControllers = new Map<string, AbortController>()

export function StoryStage({ workspace, storyId, branchId, snapshot, onDone }: StoryStageProps) {
  const [input, setInput] = useState('')
  const inputRef = useRef<HTMLTextAreaElement | null>(null)
  const snapshotKey = `${storyId || 'none'}:${snapshot?.branch_id || branchId || 'main'}:${snapshot?.turns?.[snapshot.turns.length - 1]?.id || 'empty'}`
  const stageKey = `${workspace || 'current'}:${storyId || 'none'}:${branchId || snapshot?.branch_id || 'main'}`
  const { storyStageRuns, setStoryStageRun, clearStoryStageRun } = useInteractiveStore()
  const stageRun = storyStageRuns[stageKey] || EMPTY_STAGE_RUN
  const streaming = stageRun.streaming
  const activityContent = stageRun.activityContent
  const liveMessages = stageRun.liveMessages
  const liveStageKeyRef = useRef(stageKey)
  const previousSnapshotKeyRef = useRef(snapshotKey)
  const stageTypography = useStageTypography()
  const stageTextStyle = useMemo<CSSProperties>(() => ({
    fontSize: `${stageTypography.fontSize}px`,
    lineHeight: stageTypography.lineHeight,
    fontFamily: stageTypography.fontFamily,
  }), [stageTypography.fontFamily, stageTypography.fontSize, stageTypography.lineHeight])

  const updateStageRun = useCallback((updater: Partial<StoryStageRunState> | ((current: StoryStageRunState) => StoryStageRunState)) => {
    setStoryStageRun(stageKey, updater)
  }, [setStoryStageRun, stageKey])

  const setStageStreaming = useCallback((value: boolean) => {
    updateStageRun({ streaming: value })
  }, [updateStageRun])

  const setStageActivityContent = useCallback((value: string) => {
    updateStageRun({ activityContent: value })
  }, [updateStageRun])

  const setStageLiveMessages = useCallback((updater: ChatMessage[] | ((current: ChatMessage[]) => ChatMessage[])) => {
    updateStageRun((current) => ({
      ...current,
      liveMessages: typeof updater === 'function' ? updater(current.liveMessages) : updater,
    }))
  }, [updateStageRun])

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

  const duplicatePersistedLiveTurn = useMemo(() => {
    const lastTurn = snapshot?.turns?.[snapshot.turns.length - 1]
    if (!lastTurn || !latestLiveTurn) return false
    if (liveStageKeyRef.current !== stageKey) return false
    return normalizeMessageContent(lastTurn.user) === normalizeMessageContent(latestLiveTurn.user) &&
      normalizeMessageContent(lastTurn.narrative) === normalizeMessageContent(latestLiveTurn.narrative)
  }, [latestLiveTurn, snapshot?.turns, stageKey])

  useEffect(() => {
    if (previousSnapshotKeyRef.current === snapshotKey) return
    if (streaming) return
    previousSnapshotKeyRef.current = snapshotKey
    setStageActivityContent('')
    if (!duplicatePersistedLiveTurn) {
      clearStoryStageRun(stageKey)
    }
  }, [clearStoryStageRun, duplicatePersistedLiveTurn, setStageActivityContent, snapshotKey, stageKey, streaming])

  const historyMessages = useMemo<ChatMessage[]>(() => {
    const turns = snapshot?.turns || []
    const visibleTurns = duplicatePersistedLiveTurn ? turns.slice(0, -1) : turns
    return visibleTurns.flatMap((turn) => {
      const messages: ChatMessage[] = [
        { id: `${turn.id}-user`, role: 'user', content: turn.user },
      ]
      if (turn.thinking?.trim()) {
        messages.push({ id: `${turn.id}-thinking`, role: 'thinking', content: turn.thinking, streaming: false })
      }
      messages.push({ id: `${turn.id}-assistant`, role: 'assistant', content: turn.narrative })
      return messages
    })
  }, [duplicatePersistedLiveTurn, snapshot?.turns])

  const messages = useMemo(() => [...historyMessages, ...liveMessages], [historyMessages, liveMessages])
  const scrollResetKey = `${storyId || 'none'}:${branchId || snapshot?.branch_id || 'main'}`
  const title = pickSceneTitle(snapshot, branchId)
  const hotChoices = useMemo(() => {
    const choices = snapshot?.current_turn?.hot_state?.choices || []
    return choices.map((choice) => choice.trim()).filter(Boolean).slice(0, 5)
  }, [snapshot?.current_turn?.hot_state?.choices])

  const send = async () => {
    const message = input.trim()
    if (!message || !storyId || streaming) return
    setInput('')
    setStageActivityContent('正在连接 AI Agent…')
    setStageLiveMessages([{ role: 'user', content: message }])
    liveStageKeyRef.current = stageKey
    setStageStreaming(true)
    const abortController = new AbortController()
    stageAbortControllers.set(stageKey, abortController)
    const narrativeFilter = createInteractiveNarrativeFilter()
    try {
      const stream = await sendInteractiveMessage({ mode: 'story', story_id: storyId, branch: branchId, message, signal: abortController.signal })
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
            setStageActivityContent('正在思考…')
            break
          }
          case 'tool_call': {
            const data = JSON.parse(value.data)
            setStageActivityContent(`正在处理 ${data.name || '工具调用'}…`)
            break
          }
          case 'tool_args_delta': {
            break
          }
          case 'tool_result': {
            setStageActivityContent('')
            break
          }
          case 'error': {
            const data = JSON.parse(value.data)
            setStageActivityContent('')
            setStageLiveMessages((prev) => [...prev, { role: 'error', content: data.message || data.error || '未知错误' }])
            break
          }
          case 'done': {
            const visible = narrativeFilter.flush()
            collapseNonNarrativeMessages()
            if (visible) appendAssistantMessage(visible)
            setStageActivityContent('完成')
            break
          }
          case 'aborted': {
            const visible = narrativeFilter.flush()
            collapseNonNarrativeMessages()
            if (visible) appendAssistantMessage(visible)
            setStageActivityContent('已中断')
            break
          }
        }
      }
      await onDone()
    } catch (error) {
      if (!isAbortError(error)) {
        setStageActivityContent('')
        setStageLiveMessages((prev) => [...prev, { role: 'error', content: error instanceof Error ? error.message : '互动 Agent 执行失败' }])
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
    setStageActivityContent('正在中断…')
  }

  return (
    <main className="relative flex h-full min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-[var(--nova-surface-2)]">
      <div data-testid="story-stage-card" className="flex min-h-0 flex-1 flex-col overflow-hidden bg-[var(--nova-surface-2)]">
        <div className="nova-topbar flex min-h-10 items-center justify-between gap-3 border-b px-4">
          <div className="min-w-0">
            <div className="text-[10px] font-medium leading-4 text-[var(--nova-text-faint)]">故事舞台 · 当前分支 {branchId || 'main'}</div>
            <div className="truncate text-xs font-semibold leading-5 text-[var(--nova-text)]">{title}</div>
          </div>
          <div className="flex items-center gap-2">
            <div className="flex h-7 items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-[11px] text-[var(--nova-text-muted)] shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]">
              <MessageSquareText className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
              互动创作
            </div>
            <Badge variant="outline" className="h-7 gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-[11px] text-[var(--nova-text-muted)]">
              <GitBranch className="h-3 w-3" />
              {snapshot?.turns?.length || 0} 回合
            </Badge>
          </div>
        </div>

        <div className="flex min-h-0 flex-1 overflow-hidden bg-[var(--nova-surface-2)]">
          <section className="flex min-h-0 min-w-0 flex-1 flex-col bg-[var(--nova-surface-2)]">
            <div className="flex h-8 shrink-0 items-center justify-between border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 text-[11px] text-[var(--nova-text-faint)]">
              <span className="flex items-center gap-1.5">
                <MessageSquareText className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
                指令流
              </span>
              <span>{messages.length} 条记录</span>
            </div>
            {messages.length === 0 && !streaming ? (
              <div className="m-5 flex min-h-0 flex-1 items-center justify-center rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] text-sm text-[var(--nova-text-faint)] shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]">
                输入第一句话，开始互动故事。
              </div>
            ) : (
              <MessageList
                messages={messages}
                isStreaming={streaming}
                activityContent={activityContent}
                highlightDialogue
                scrollResetKey={scrollResetKey}
                bottomPaddingClassName="pb-6"
                messageStyle={stageTextStyle}
              />
            )}
          </section>
        </div>
      </div>
      <div className="shrink-0 border-t border-[var(--nova-border)] bg-[var(--nova-surface)] p-3">
        <div className="mx-auto max-w-5xl">
          {!streaming && hotChoices.length > 0 ? (
            <div className="mb-3 border-b border-[var(--nova-border)] pb-3">
              <div className="mb-2 flex items-center gap-1.5 text-[11px] font-medium text-[var(--nova-text-faint)]">
                <Compass className="h-3.5 w-3.5" />
                可选择
              </div>
              <div className="flex flex-wrap gap-2">
                {hotChoices.map((choice, index) => (
                  <button
                    key={`${index}-${choice}`}
                    type="button"
                    className="max-w-full rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-1.5 text-left text-xs leading-5 text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
                    onClick={() => {
                      setInput(choice)
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
            </div>
          ) : null}
          <div className="flex items-center gap-3">
            <Textarea
              ref={inputRef}
              className="nova-field h-14 min-h-14 flex-1 resize-none text-sm leading-6 placeholder:text-[var(--nova-text-faint)] focus-visible:ring-1 focus-visible:ring-[var(--nova-border)]/35"
              style={stageTextStyle}
              value={input}
              placeholder="你要做什么？"
              onChange={(event) => setInput(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' && !event.shiftKey) {
                  event.preventDefault()
                  void send()
                }
              }}
            />
            <Button
              className={`h-14 w-24 border border-[var(--nova-border)] text-[var(--nova-text)] shadow-[inset_0_1px_0_rgba(255,255,255,0.08)] ${streaming ? 'bg-red-500/45 hover:bg-red-500/55' : 'bg-[var(--nova-active)] hover:bg-[var(--nova-hover)]'}`}
              disabled={streaming ? false : (!storyId || !input.trim())}
              onClick={() => { streaming ? stop() : void send() }}
              aria-label={streaming ? '中断 AI 执行' : '发送'}
            >
              {streaming ? <Square className="h-4 w-4 fill-current" /> : <Send className="h-4 w-4" />}
              {streaming ? '中断' : '发送'}
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
        return [...prev.slice(0, -1), { ...last, content: `${last.content || ''}${content}`, streaming: true }]
      }
      return [...prev, { role: 'thinking', content, streaming: true }]
    })
  }

  function collapseNonNarrativeMessages() {
    setStageLiveMessages((prev) => prev.map((msg) => (
      msg.role === 'thinking' || msg.role === 'tool_call'
        ? { ...msg, streaming: false, status: msg.role === 'tool_call' ? (msg.status === 'running' ? 'success' : msg.status) : msg.status }
        : msg
    )))
  }
}

function useStageTypography() {
  const [typography, setTypography] = useState({
    fontSize: DEFAULT_STAGE_FONT_SIZE,
    lineHeight: DEFAULT_STAGE_LINE_HEIGHT,
    fontFamily: fontStackFor(DEFAULT_READING_FONT, DEFAULT_READING_FONT),
  })

  const load = useCallback(async () => {
    try {
      const settings = await fetchSettings()
      const effective = settings.effective || {}
      setTypography({
        fontSize: clampNumber(effective.interactive_stage_font_size, 13, 24, DEFAULT_STAGE_FONT_SIZE),
        lineHeight: clampNumber(effective.interactive_stage_line_height, 1.35, 2.4, DEFAULT_STAGE_LINE_HEIGHT),
        fontFamily: fontStackFor(effective.reading_font_family, DEFAULT_READING_FONT),
      })
    } catch (error) {
      console.warn('[interactive-stage] 加载故事舞台显示设置失败', error)
      setTypography({
        fontSize: DEFAULT_STAGE_FONT_SIZE,
        lineHeight: DEFAULT_STAGE_LINE_HEIGHT,
        fontFamily: fontStackFor(DEFAULT_READING_FONT, DEFAULT_READING_FONT),
      })
    }
  }, [])

  useEffect(() => {
    void load()
    window.addEventListener('nova:settings-updated', load)
    return () => window.removeEventListener('nova:settings-updated', load)
  }, [load])

  return typography
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

function pickSceneTitle(snapshot: Snapshot | null, branchId: string) {
  const current = snapshot?.graph?.nodes?.find((node) => node.current && node.branch_id === (snapshot.branch_id || branchId)) ||
    snapshot?.graph?.nodes?.find((node) => node.head && node.branch_id === (snapshot.branch_id || branchId))
  if (current?.title) return current.title
  return '主创作区'
}
