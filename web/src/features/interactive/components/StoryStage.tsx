import { useEffect, useMemo, useRef, useState } from 'react'
import { MessageSquareText, PenLine, Route, Send, Square } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { MessageList } from '@/components/Chat/MessageList'
import type { ChatMessage } from '@/lib/api'
import { abortInteractiveChat, sendInteractiveMessage } from '../api'
import { createInteractiveNarrativeFilter } from '../stream-parser'
import type { Snapshot } from '../types'

interface StoryStageProps {
  storyId: string
  branchId: string
  snapshot: Snapshot | null
  onDone: () => void
}

export function StoryStage({ storyId, branchId, snapshot, onDone }: StoryStageProps) {
  const [input, setInput] = useState('')
  const [workspaceMode, setWorkspaceMode] = useState('dialogue')
  const [streaming, setStreaming] = useState(false)
  const [activityContent, setActivityContent] = useState('')
  const [liveMessages, setLiveMessages] = useState<ChatMessage[]>([])
  const abortControllerRef = useRef<AbortController | null>(null)
  const snapshotKey = `${storyId || 'none'}:${snapshot?.branch_id || branchId || 'main'}:${snapshot?.turns?.[snapshot.turns.length - 1]?.id || 'empty'}`
  const previousSnapshotKeyRef = useRef(snapshotKey)

  useEffect(() => {
    if (previousSnapshotKeyRef.current === snapshotKey) return
    if (streaming) return
    previousSnapshotKeyRef.current = snapshotKey
    setActivityContent('')
    setLiveMessages([])
  }, [snapshotKey, streaming])

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
    return normalizeMessageContent(lastTurn.user) === normalizeMessageContent(latestLiveTurn.user) &&
      normalizeMessageContent(lastTurn.narrative) === normalizeMessageContent(latestLiveTurn.narrative)
  }, [latestLiveTurn, snapshot?.turns])

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

  const send = async () => {
    const message = input.trim()
    if (!message || !storyId || streaming) return
    setInput('')
    setActivityContent('正在连接 AI Agent…')
    setLiveMessages([{ role: 'user', content: message }])
    setStreaming(true)
    const abortController = new AbortController()
    abortControllerRef.current = abortController
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
            setActivityContent('')
            break
          }
          case 'thinking': {
            const data = JSON.parse(value.data)
            appendThinkingMessage(data.content || '')
            setActivityContent('正在思考…')
            break
          }
          case 'tool_call': {
            const data = JSON.parse(value.data)
            setActivityContent(`正在处理 ${data.name || '工具调用'}…`)
            break
          }
          case 'tool_args_delta': {
            break
          }
          case 'tool_result': {
            setActivityContent('')
            break
          }
          case 'error': {
            const data = JSON.parse(value.data)
            setActivityContent('')
            setLiveMessages((prev) => [...prev, { role: 'error', content: data.message || data.error || '未知错误' }])
            break
          }
          case 'done': {
            const visible = narrativeFilter.flush()
            collapseNonNarrativeMessages()
            if (visible) appendAssistantMessage(visible)
            setActivityContent('完成')
            break
          }
          case 'aborted': {
            const visible = narrativeFilter.flush()
            collapseNonNarrativeMessages()
            if (visible) appendAssistantMessage(visible)
            setActivityContent('已中断')
            break
          }
        }
      }
      await onDone()
    } catch (error) {
      if (!isAbortError(error)) {
        setActivityContent('')
        setLiveMessages((prev) => [...prev, { role: 'error', content: error instanceof Error ? error.message : '互动 Agent 执行失败' }])
      }
    } finally {
      setStreaming(false)
      abortControllerRef.current = null
      setActivityContent('')
    }
  }

  const stop = () => {
    void abortInteractiveChat()
    abortControllerRef.current?.abort()
    setActivityContent('正在中断…')
  }

  return (
    <main className="relative flex h-full min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-[#17191d] p-4">
      <div data-testid="story-stage-card" className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border border-[#343b47] bg-[#101216] shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]">
        <div className="flex min-h-12 items-center justify-between gap-3 border-b border-[#262c35] px-5">
          <div className="min-w-0">
            <div className="text-[11px] font-medium text-[#8893a4]">故事舞台 · 当前分支 {branchId || 'main'}</div>
            <div className="truncate text-sm font-semibold text-[#e2e6ee]">主创作区</div>
          </div>
          <div className="flex items-center gap-2">
            <Tabs value={workspaceMode} onValueChange={setWorkspaceMode}>
              <TabsList className="h-8 bg-[#1f2430]">
                <TabsTrigger value="draft" className="gap-1.5 px-2.5 text-xs">
                  <PenLine className="h-3.5 w-3.5" />
                  正文
                </TabsTrigger>
                <TabsTrigger value="dialogue" className="gap-1.5 px-2.5 text-xs">
                  <MessageSquareText className="h-3.5 w-3.5" />
                  对话
                </TabsTrigger>
                <TabsTrigger value="branch" className="gap-1.5 px-2.5 text-xs">
                  <Route className="h-3.5 w-3.5" />
                  推演
                </TabsTrigger>
              </TabsList>
            </Tabs>
            <Badge variant="outline" className="border-[#384150] bg-[#1c222b] text-[#8f98a8]">{snapshot?.turns?.length || 0} 回合</Badge>
          </div>
        </div>
        {messages.length === 0 && !streaming ? (
          <div className="m-5 flex min-h-0 flex-1 items-center justify-center rounded-lg border border-dashed border-[#343b47] bg-[#171b21]/80 text-sm text-[#858b96]">
            输入第一句话，开始互动故事。
          </div>
        ) : (
          <MessageList
            messages={messages}
            isStreaming={streaming}
            activityContent={activityContent}
            highlightDialogue
            scrollResetKey={scrollResetKey}
            bottomPaddingClassName="pb-32"
          />
        )}
      </div>
      <div className="pointer-events-none absolute inset-x-4 bottom-4 z-20 bg-gradient-to-t from-[#17191d] via-[#17191d]/95 to-transparent pt-8">
        <div className="pointer-events-auto rounded-xl border border-[#343b47] bg-[#101216]/95 p-3 shadow-[0_18px_48px_rgba(0,0,0,0.38),inset_0_1px_0_rgba(255,255,255,0.03)] backdrop-blur">
        <div className="flex items-center gap-3">
          <Textarea
            className="h-16 min-h-16 flex-1 resize-none border-[#343b47] bg-[#1b2028] text-sm leading-6 text-[#d7dbe2] placeholder:text-[#778091] focus-visible:ring-1"
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
            className={`h-16 w-24 text-white ${streaming ? 'bg-[#c95050] hover:bg-[#e05d5d]' : 'bg-[#2d6fb8] hover:bg-[#347dca]'}`}
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
    setLiveMessages((prev) => {
      const last = prev[prev.length - 1]
      if (last?.role === 'assistant' && last.streaming) {
        return [...prev.slice(0, -1), { ...last, content: `${last.content || ''}${content}` }]
      }
      return [...prev, { role: 'assistant', content, streaming: true }]
    })
  }

  function appendThinkingMessage(content: string) {
    if (!content) return
    setLiveMessages((prev) => {
      const last = prev[prev.length - 1]
      if (last?.role === 'thinking') {
        return [...prev.slice(0, -1), { ...last, content: `${last.content || ''}${content}`, streaming: true }]
      }
      return [...prev, { role: 'thinking', content, streaming: true }]
    })
  }

  function collapseNonNarrativeMessages() {
    setLiveMessages((prev) => prev.map((msg) => (
      msg.role === 'thinking' || msg.role === 'tool_call'
        ? { ...msg, streaming: false, status: msg.role === 'tool_call' ? (msg.status === 'running' ? 'success' : msg.status) : msg.status }
        : msg
    )))
  }
}

function isAbortError(error: unknown) {
  return error instanceof DOMException && error.name === 'AbortError'
}

function normalizeMessageContent(value: string) {
  return value.replace(/\r\n/g, '\n').trim()
}
