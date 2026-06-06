import { useRef, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import {
  abortChat,
  createSession,
  deleteSession,
  executeCommand,
  getActiveChatTask,
  getMessages,
  getSessions,
  renameSession,
  sendMessage,
  streamActiveChat,
  switchSession,
} from '@/lib/api'
import type { ChatMessage, SSEEvent, SessionSummary, TextSelection } from '@/lib/api'

interface ChatOptions {
  onAgentFileChange?: (path?: string) => void | Promise<void>
}

interface ToolCallInfo {
  id: string
  name: string
  args: string
}

type StreamSegmentRole = 'assistant' | 'thinking'

const STREAM_CHARS_PER_FRAME = 8

/** 聊天 hook，管理消息列表和流式响应 */
export function useChat(options: ChatOptions = {}) {
  const { t } = useTranslation()
  const { onAgentFileChange } = options
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [sessions, setSessions] = useState<SessionSummary[]>([])
  const [activeSessionId, setActiveSessionId] = useState('')
  const [isStreaming, setIsStreaming] = useState(false)
  const [activityContent, setActivityContent] = useState('')
  const [references, setReferences] = useState<string[]>([])
  const [loreReferences, setLoreReferences] = useState<string[]>([])
  const [styleReferences, setStyleReferences] = useState<string[]>([])
  const [textSelections, setTextSelections] = useState<TextSelection[]>([])
  const abortControllerRef = useRef<AbortController | null>(null)
  const currentSegmentIdRef = useRef<string | null>(null)
  const currentSegmentRoleRef = useRef<StreamSegmentRole | null>(null)
  const segmentIdCounterRef = useRef(0)
  const pendingToolCallsRef = useRef<Record<string, ToolCallInfo>>({})
  const toolCallQueueRef = useRef<string[]>([])
  const toolKeyToMessageIdRef = useRef<Record<string, string>>({})
  const toolIdCounterRef = useRef(0)
  // 文本流增量按帧合并，避免每个 token 都触发整条消息列表重渲染。
  const segmentBufferRef = useRef<Record<string, string>>({})
  const segmentRafRef = useRef<number | null>(null)
  // tool_args_delta 节流：累积增量，每帧批量刷新一次
  const deltaBufferRef = useRef<Record<string, string>>({})
  const deltaRafRef = useRef<number | null>(null)

  /** 重置本地流式状态，切换会话时避免旧会话残留在当前 UI。 */
  const resetStreamingState = useCallback(() => {
    abortControllerRef.current?.abort()
    abortControllerRef.current = null
    currentSegmentIdRef.current = null
    currentSegmentRoleRef.current = null
    pendingToolCallsRef.current = {}
    toolCallQueueRef.current = []
    toolKeyToMessageIdRef.current = {}
    segmentBufferRef.current = {}
    deltaBufferRef.current = {}
    if (segmentRafRef.current !== null) {
      cancelAnimationFrame(segmentRafRef.current)
      segmentRafRef.current = null
    }
    if (deltaRafRef.current !== null) {
      cancelAnimationFrame(deltaRafRef.current)
      deltaRafRef.current = null
    }
    setIsStreaming(false)
    setActivityContent('')
  }, [])

  /** 加载会话列表。 */
  const loadSessions = useCallback(async () => {
    try {
      const list = await getSessions()
      setSessions(list)
      setActiveSessionId(list.find(item => item.active)?.id || list[0]?.id || '')
      return list
    } catch (e) {
      console.error('加载会话列表失败', e)
      return []
    }
  }, [])

  /** 加载历史消息 */
  const loadHistory = useCallback(async (sessionId?: string) => {
    try {
      const msgs = await getMessages(sessionId)
      setMessages(normalizeRepeatedMessages(msgs))
    } catch (e) {
      console.error('加载历史失败', e)
    }
  }, [])

  /** 添加文件引用 */
  const addReference = useCallback((path: string) => {
    setReferences(prev => Array.from(new Set([...prev, path])))
  }, [])

  /** 添加资料库条目引用 */
  const addLoreReference = useCallback((id: string) => {
    setLoreReferences(prev => Array.from(new Set([...prev, id])))
  }, [])

  /** 移除文件引用 */
  const removeReference = useCallback((path: string) => {
    setReferences(prev => prev.filter(item => item !== path))
  }, [])

  /** 移除资料库条目引用 */
  const removeLoreReference = useCallback((id: string) => {
    setLoreReferences(prev => prev.filter(item => item !== id))
  }, [])

  /** 添加风格参考 */
  const addStyleReference = useCallback((path: string) => {
    setStyleReferences(prev => Array.from(new Set([...prev, path])))
  }, [])

  /** 移除风格参考 */
  const removeStyleReference = useCallback((path: string) => {
    setStyleReferences(prev => prev.filter(item => item !== path))
  }, [])

  /** 清空文件引用 */
  const clearReferences = useCallback(() => {
    setReferences([])
  }, [])

  /** 清空资料库条目引用 */
  const clearLoreReferences = useCallback(() => {
    setLoreReferences([])
  }, [])

  /** 清空风格参考 */
  const clearStyleReferences = useCallback(() => {
    setStyleReferences([])
  }, [])

  /** 添加文本片段引用 */
  const addTextSelection = useCallback((sel: TextSelection) => {
    setTextSelections(prev => [...prev, sel])
  }, [])

  /** 移除文本片段引用 */
  const removeTextSelection = useCallback((index: number) => {
    setTextSelections(prev => prev.filter((_, i) => i !== index))
  }, [])

  /** 清空文本片段引用 */
  const clearTextSelections = useCallback(() => {
    setTextSelections([])
  }, [])

  /** 将本轮仍未完成的工具标记为失败，避免 UI 长时间停在执行中。 */
  const markPendingToolsAsError = useCallback(() => {
    const pendingIds = new Set(Object.keys(pendingToolCallsRef.current))
    if (pendingIds.size === 0) return
    setMessages(prev => prev.map(message => (
      message.role === 'tool_call' && message.id && pendingIds.has(message.id)
        ? { ...message, status: 'error' }
        : message
    )))
  }, [])

  /** 将 tool_args_delta 的缓冲内容刷新到消息列表，避免恢复流结束时丢失最后一帧。 */
  const flushToolArgBuffer = useCallback(() => {
    const buffered = { ...deltaBufferRef.current }
    deltaBufferRef.current = {}
    if (deltaRafRef.current !== null) {
      cancelAnimationFrame(deltaRafRef.current)
      deltaRafRef.current = null
    }
    if (Object.keys(buffered).length === 0) return
    setMessages(prev => prev.map(message => {
      if (message.role === 'tool_call' && message.id && buffered[message.id]) {
        return { ...message, args: (message.args || '') + buffered[message.id] }
      }
      return message
    }))
  }, [])

  /** 将 assistant/thinking 流式文本按小片段刷入消息列表，模拟常规 LLM token 流。 */
  const flushStreamingSegmentBuffer = useCallback((flushAll = false) => {
    const buffered = { ...segmentBufferRef.current }
    if (segmentRafRef.current !== null) {
      cancelAnimationFrame(segmentRafRef.current)
      segmentRafRef.current = null
    }
    if (Object.keys(buffered).length === 0) return
    const visible: Record<string, string> = {}
    const remaining: Record<string, string> = {}
    for (const [id, text] of Object.entries(buffered)) {
      if (flushAll || text.length <= STREAM_CHARS_PER_FRAME) {
        visible[id] = text
        continue
      }
      visible[id] = text.slice(0, STREAM_CHARS_PER_FRAME)
      remaining[id] = text.slice(STREAM_CHARS_PER_FRAME)
    }
    segmentBufferRef.current = remaining
    setMessages(prev => updateStreamingSegments(prev, visible))
    if (!flushAll && Object.keys(remaining).length > 0) {
      segmentRafRef.current = requestAnimationFrame(() => flushStreamingSegmentBuffer(false))
    }
  }, [])

  /** 结束当前流式文本段，避免历史消息继续显示为更新中。 */
  const finishCurrentSegment = useCallback(() => {
    const segmentId = currentSegmentIdRef.current
    if (!segmentId) return
    flushStreamingSegmentBuffer(true)
    currentSegmentIdRef.current = null
    currentSegmentRoleRef.current = null
    setMessages(prev => finalizeStreamingSegment(prev, segmentId))
  }, [flushStreamingSegmentBuffer])

  /** 将连续的 assistant/thinking 增量写入同一条时间线消息。 */
  const appendStreamingSegment = useCallback((role: StreamSegmentRole, text: string) => {
    if (!text) return
    if (currentSegmentRoleRef.current !== role || !currentSegmentIdRef.current) {
      finishCurrentSegment()
      currentSegmentIdRef.current = createSegmentId(role, segmentIdCounterRef)
      currentSegmentRoleRef.current = role
      const segmentId = currentSegmentIdRef.current
      if (!segmentId) return
      setMessages(prev => appendStreamingSegmentMessage(prev, role, segmentId, text))
      return
    }
    const segmentId = currentSegmentIdRef.current
    if (!segmentId) return
    segmentBufferRef.current[segmentId] = (segmentBufferRef.current[segmentId] || '') + text
    if (segmentRafRef.current === null) {
      segmentRafRef.current = requestAnimationFrame(() => flushStreamingSegmentBuffer(false))
    }
  }, [finishCurrentSegment, flushStreamingSegmentBuffer])

  /** 消费聊天 SSE 流，发送与恢复订阅共用同一套状态更新逻辑。 */
  const consumeChatStream = useCallback(async (
    stream: ReadableStream<SSEEvent>,
    options: { clearInputsOnFinish?: boolean; showAbortMessage?: boolean } = {},
  ) => {
    pendingToolCallsRef.current = {}
    toolCallQueueRef.current = []
    toolKeyToMessageIdRef.current = {}
    currentSegmentIdRef.current = null
    currentSegmentRoleRef.current = null
      segmentBufferRef.current = {}
    setIsStreaming(true)
    setActivityContent(t('chat.activity.connecting'))

    try {
      const reader = stream.getReader()

      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        const event = value as SSEEvent
        switch (event.event) {
          case 'chunk': {
            const text = JSON.parse(event.data).content || ''
            appendStreamingSegment('assistant', text)
            setActivityContent('')
            break
          }
          case 'thinking': {
            const text = JSON.parse(event.data).content || ''
            appendStreamingSegment('thinking', text)
            setActivityContent(t('chat.activity.thinking'))
            break
          }
          case 'tool_call': {
            finishCurrentSegment()
            const data = JSON.parse(event.data)
            const toolName = data.name || 'unknown_tool'
            const args = data.args || ''
            const toolKey = getToolEventKey(data)
            const existingToolId = toolKey ? toolKeyToMessageIdRef.current[toolKey] : undefined
            const toolId = existingToolId || createToolMessageId(toolKey, toolIdCounterRef)
            if (toolKey) {
              toolKeyToMessageIdRef.current = { ...toolKeyToMessageIdRef.current, [toolKey]: toolId }
            }
            pendingToolCallsRef.current = {
              ...pendingToolCallsRef.current,
              [toolId]: { id: toolId, name: toolName, args },
            }
            if (!toolCallQueueRef.current.includes(toolId)) {
              toolCallQueueRef.current = [...toolCallQueueRef.current, toolId]
            }
            setActivityContent('')
            setMessages(prev => upsertToolCallMessage(prev, {
              role: 'tool_call',
              content: buildToolContent(toolName, args),
              id: toolId,
              name: toolName,
              args,
              status: 'running',
            }))
            break
          }
          case 'tool_result': {
            flushToolArgBuffer()
            const data = JSON.parse(event.data)
            const content = data.content || ''
            const toolId = findToolMessageId(data, toolKeyToMessageIdRef.current, toolCallQueueRef.current)
            const toolCall = toolId ? pendingToolCallsRef.current[toolId] : undefined
            const toolName = data.name || toolCall?.name || ''
            if (toolId) {
              const { [toolId]: _, ...restPending } = pendingToolCallsRef.current
              pendingToolCallsRef.current = restPending
              toolCallQueueRef.current = toolCallQueueRef.current.filter(id => id !== toolId)
            }
            setActivityContent('')
            if (toolId) {
              setMessages(prev => prev.map(message => (
                message.role === 'tool_call' && message.id === toolId
                  ? { ...message, status: 'success', result: content }
                  : message
              )))
            } else {
              setMessages(prev => [...prev, { role: 'tool_result', content }])
            }
            if (toolCall && isFileMutationTool(toolCall.name)) {
              void onAgentFileChange?.(extractToolPath(toolCall.args))
            }
            if (isLoreMutationTool(toolName)) {
              notifyLoreUpdated(readStringArray(data.item_ids))
            }
            break
          }
          case 'tool_args_delta': {
            const data = JSON.parse(event.data)
            const delta = data.delta || ''
            const toolId = findToolMessageId(data, toolKeyToMessageIdRef.current, toolCallQueueRef.current)
            if (toolId) {
              // 同步更新 pendingToolCallsRef 的 args，供 tool_result 时提取路径
              const pending = pendingToolCallsRef.current[toolId]
              if (pending) {
                pending.args = (pending.args || '') + delta
              }
              // 累积到 buffer，通过 rAF 节流批量刷新到 UI
              deltaBufferRef.current[toolId] = (deltaBufferRef.current[toolId] || '') + delta
              if (deltaRafRef.current === null) {
                deltaRafRef.current = requestAnimationFrame(flushToolArgBuffer)
              }
            }
            break
          }
          case 'done': {
            setActivityContent(t('chat.activity.done'))
            break
          }
          case 'aborted': {
            markPendingToolsAsError()
            setActivityContent(t('chat.activity.aborted'))
            break
          }
          case 'error': {
            const data = JSON.parse(event.data)
            markPendingToolsAsError()
            setActivityContent('')
            setMessages(prev => [...prev, { role: 'error', content: data.message || data.error || t('chat.activity.unknownError') }])
            break
          }
        }
      }

      // 流结束，将剩余缓冲的 thinking/content 刷入消息列表
      flushToolArgBuffer()
      flushStreamingSegmentBuffer(true)
      finishCurrentSegment()
      if (options.clearInputsOnFinish) {
        clearReferences()
        clearLoreReferences()
        clearStyleReferences()
        clearTextSelections()
      }
    } catch (e) {
      markPendingToolsAsError()
      if (isAbortError(e)) {
        setActivityContent(t('chat.activity.aborted'))
        flushToolArgBuffer()
        flushStreamingSegmentBuffer(true)
        finishCurrentSegment()
        if (options.showAbortMessage) {
          setMessages(prev => [...prev, { role: 'system', content: t('chat.activity.abortMessage') }])
        }
      } else {
        flushToolArgBuffer()
        flushStreamingSegmentBuffer(true)
        finishCurrentSegment()
        setMessages(prev => [...prev, { role: 'error', content: t('chat.activity.requestFailed', { error: String(e) }) }])
      }
      if (options.clearInputsOnFinish) {
      clearReferences()
      clearLoreReferences()
      clearStyleReferences()
        clearTextSelections()
      }
    } finally {
      abortControllerRef.current = null
      pendingToolCallsRef.current = {}
      toolCallQueueRef.current = []
      toolKeyToMessageIdRef.current = {}
      flushToolArgBuffer()
      flushStreamingSegmentBuffer(true)
      finishCurrentSegment()
      setIsStreaming(false)
      setActivityContent('')
    }
  }, [
    appendStreamingSegment,
    clearReferences,
    clearLoreReferences,
    clearStyleReferences,
    clearTextSelections,
    finishCurrentSegment,
    flushStreamingSegmentBuffer,
    flushToolArgBuffer,
    markPendingToolsAsError,
    onAgentFileChange,
    t,
  ])

  /** 发送消息 */
  const send = useCallback(async (input: string) => {
    if (isStreaming) return
    // 检查是否是命令
    if (input.startsWith('/')) {
      const cmd = input.slice(1).split(' ')[0]
      if (['clear', 'status', 'help'].includes(cmd)) {
        const result = await executeCommand(cmd)
        if (cmd === 'clear') {
          await loadHistory()
          await loadSessions()
          return
        }
        setMessages(prev => [...prev, { role: 'system', content: result }])
        return
      }
    }

    // 检测 /plan 前缀，进入规划模式
    let planMode = false
    let userMessage = input
    if (input.startsWith('/plan')) {
      planMode = true
      userMessage = input.replace(/^\/plan\s*/, '').trim()
      if (!userMessage) {
        setMessages(prev => [...prev, { role: 'system', content: t('chat.planUsage') }])
        return
      }
    }

    const inlineReferences = parseInlineReferences(input)
    const mergedReferences = Array.from(new Set([...references, ...inlineReferences]))
    const mergedLoreReferences = Array.from(new Set(loreReferences))
    const inlineStyleReferences = parseInlineStyleReferences(input)
    const mergedStyleReferences = Array.from(new Set([...styleReferences, ...inlineStyleReferences]))

    // 添加用户消息
    setMessages(prev => [...prev, { role: 'user', content: input }])
    const abortController = new AbortController()
    abortControllerRef.current = abortController

    try {
      const stream = await sendMessage(userMessage, mergedReferences, mergedLoreReferences, mergedStyleReferences, textSelections, abortController.signal, planMode)
      await consumeChatStream(stream, { clearInputsOnFinish: true, showAbortMessage: true })
    } catch (e) {
      setMessages(prev => [...prev, { role: 'error', content: t('chat.activity.requestFailed', { error: String(e) }) }])
      setIsStreaming(false)
    }
  }, [consumeChatStream, isStreaming, loadHistory, loadSessions, loreReferences, references, styleReferences, t, textSelections])

  /** 恢复订阅后台仍在运行的聊天任务。 */
  const resumeActiveChat = useCallback(async () => {
    if (isStreaming) return
    try {
      const activeTask = await getActiveChatTask()
      if (!activeTask.active) return

      const abortController = new AbortController()
      abortControllerRef.current = abortController
      const stream = await streamActiveChat(abortController.signal)
      await consumeChatStream(stream)
    } catch (e) {
      if (!isAbortError(e)) {
        console.error('恢复聊天流失败', e)
      }
    }
  }, [consumeChatStream, isStreaming])

  /** 中断当前 AI 执行 */
  const stop = useCallback(() => {
    void abortChat()
    abortControllerRef.current?.abort()
  }, [])

  /** 创建新会话，并刷新当前消息列表。 */
  const createChatSession = useCallback(async (title?: string) => {
    resetStreamingState()
    const session = await createSession(title)
    setActiveSessionId(session.id)
    await Promise.all([loadSessions(), loadHistory(session.id)])
    await resumeActiveChat()
  }, [loadHistory, loadSessions, resetStreamingState, resumeActiveChat])

  /** 切换会话并读取该会话历史。 */
  const switchChatSession = useCallback(async (id: string) => {
    if (!id || id === activeSessionId) return
    resetStreamingState()
    const session = await switchSession(id)
    setActiveSessionId(session.id)
    await Promise.all([loadSessions(), loadHistory(session.id)])
    await resumeActiveChat()
  }, [activeSessionId, loadHistory, loadSessions, resetStreamingState, resumeActiveChat])

  /** 重命名会话。 */
  const renameChatSession = useCallback(async (id: string, title: string) => {
    await renameSession(id, title)
    await loadSessions()
  }, [loadSessions])

  /** 删除会话并切换到后端返回的新激活会话。 */
  const deleteChatSession = useCallback(async (id: string) => {
    resetStreamingState()
    const session = await deleteSession(id)
    setActiveSessionId(session.id)
    await Promise.all([loadSessions(), loadHistory(session.id)])
    await resumeActiveChat()
  }, [loadHistory, loadSessions, resetStreamingState, resumeActiveChat])

  return {
    messages,
    sessions,
    activeSessionId,
    isStreaming,
    activityContent,
    references,
    loreReferences,
    styleReferences,
    textSelections,
    send,
    stop,
    loadSessions,
    loadHistory,
    resumeActiveChat,
    createChatSession,
    switchChatSession,
    renameChatSession,
    deleteChatSession,
    addReference,
    removeReference,
    addLoreReference,
    removeLoreReference,
    addStyleReference,
    removeStyleReference,
    addTextSelection,
    removeTextSelection,
    clearReferences,
    clearStyleReferences,
  }
}

function isAbortError(error: unknown) {
  return error instanceof DOMException && error.name === 'AbortError'
}

function parseInlineReferences(input: string): string[] {
  const result = new Set<string>()
  const regex = /(?:^|\s)@([^\s@]+)/g
  let match: RegExpExecArray | null
  while ((match = regex.exec(input)) !== null) {
    const value = match[1]
    if (value.startsWith('资料:')) continue
    result.add(value)
  }
  return Array.from(result)
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

function getToolEventKey(data: Record<string, unknown>): string | undefined {
  if (typeof data.id === 'string' && data.id) return `id:${data.id}`
  if (typeof data.index === 'number') return `index:${data.index}`
  if (typeof data.index === 'string' && data.index) return `index:${data.index}`
  return undefined
}

function createToolMessageId(toolKey: string | undefined, counterRef: { current: number }) {
  counterRef.current += 1
  const suffix = toolKey ? toolKey.replace(/[^a-zA-Z0-9:_-]/g, '_') : `local:${counterRef.current}`
  return `tool:${Date.now()}:${suffix}:${counterRef.current}`
}

function createSegmentId(role: StreamSegmentRole, counterRef: { current: number }) {
  counterRef.current += 1
  return `segment:${role}:${Date.now()}:${counterRef.current}`
}

function appendStreamingSegmentMessage(
  messages: ChatMessage[],
  role: StreamSegmentRole,
  id: string,
  text: string,
) {
  return [...messages, { role, id, content: text, streaming: true }]
}

function updateStreamingSegments(messages: ChatMessage[], buffered: Record<string, string>) {
  return messages.map(message => (
    message.id && buffered[message.id]
      ? { ...message, content: (message.content || '') + buffered[message.id], streaming: true }
      : message
  ))
}

function finalizeStreamingSegment(messages: ChatMessage[], id: string) {
  return messages.map(message => (
    message.id === id ? { ...message, streaming: false } : message
  ))
}

function findToolMessageId(
  data: Record<string, unknown>,
  keyToMessageId: Record<string, string>,
  fallbackQueue: string[],
) {
  const toolKey = getToolEventKey(data)
  if (toolKey && keyToMessageId[toolKey]) return keyToMessageId[toolKey]
  return fallbackQueue[0]
}

function buildToolContent(name: string, args: string) {
  return args ? `${name}\n${args}` : name
}

function upsertToolCallMessage(messages: ChatMessage[], next: ChatMessage) {
  if (!next.id) return [...messages, next]
  let found = false
  const updated = messages.map(message => {
    if (message.role !== 'tool_call' || message.id !== next.id) return message
    found = true
    const args = next.args || message.args || ''
    const name = next.name || message.name
    return {
      ...message,
      ...next,
      name,
      args,
      content: buildToolContent(name || 'unknown_tool', args),
      status: message.status === 'success' ? message.status : next.status,
      result: message.result,
    }
  })
  return found ? updated : [...messages, next]
}

function normalizeRepeatedMessages(messages: ChatMessage[]) {
  const normalized: ChatMessage[] = []
  for (const message of messages) {
    const prev = normalized[normalized.length - 1]
    if (
      prev &&
      prev.role === message.role &&
      normalizeMessageContent(prev.content || '') === normalizeMessageContent(message.content || '')
    ) {
      continue
    }
    normalized.push(message)
  }
  return normalized
}

function normalizeMessageContent(content: string) {
  return content.trim().replace(/\s+/g, ' ')
}

function isFileMutationTool(name: string) {
  return ['write_file', 'create_file', 'edit_file', 'replace_file', 'delete_file', 'rename_file'].includes(name)
}

function isLoreMutationTool(name: string) {
  return name === 'write_lore_items'
}

function notifyLoreUpdated(itemIds: string[] = []) {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent('nova:lore-updated', { detail: { item_ids: itemIds } }))
}

function readStringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string') : []
}

function extractToolPath(args: string): string | undefined {
  if (!args) return undefined
  try {
    const data = JSON.parse(args) as Record<string, unknown>
    const value = data.path || data.file_path || data.to || data.from
    return typeof value === 'string' ? value : undefined
  } catch {
    const match = args.match(/"(?:path|file_path)"\s*:\s*"([^"]+)"/)
    return match?.[1]
  }
}
