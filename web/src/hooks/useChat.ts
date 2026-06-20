import { useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import {
  abortChat,
  analyzeChatContext,
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
import type { ContextAnalysis, SessionSummary, TextSelection } from '@/lib/api'
import { isAbortError, normalizeRepeatedMessages, useAgentEventStream } from './useAgentEventStream'

interface ChatOptions {
  onAgentFileChange?: (path?: string) => void | Promise<void>
}

/** 聊天 hook，管理消息列表和流式响应 */
export function useChat(options: ChatOptions = {}) {
  const { t } = useTranslation()
  const { onAgentFileChange } = options
  const {
    messages,
    setMessages,
    isStreaming,
    activityContent,
    consumeAgentStream,
    resetStreamingState,
    setAbortController,
    abortLocalStream,
  } = useAgentEventStream({ onAgentFileChange })
  const [sessions, setSessions] = useState<SessionSummary[]>([])
  const [activeSessionId, setActiveSessionId] = useState('')
  const [references, setReferences] = useState<string[]>([])
  const [loreReferences, setLoreReferences] = useState<string[]>([])
  const [styleReferences, setStyleReferences] = useState<string[]>([])
  const [textSelections, setTextSelections] = useState<TextSelection[]>([])

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

  const clearInputState = useCallback(() => {
    clearReferences()
    clearLoreReferences()
    clearStyleReferences()
    clearTextSelections()
  }, [clearLoreReferences, clearReferences, clearStyleReferences, clearTextSelections])

  const prepareAgentRequest = useCallback((input: string) => {
    if (input.startsWith('/')) {
      const cmd = input.slice(1).split(' ')[0]
      if (['clear', 'compact', 'status', 'help'].includes(cmd)) {
        throw new Error(t('chat.contextAnalysis.commandUnavailable'))
      }
    }

    let planMode = false
    let userMessage = input
    if (input.startsWith('/plan')) {
      planMode = true
      userMessage = input.replace(/^\/plan\s*/, '').trim()
      if (!userMessage) {
        throw new Error(t('chat.planUsage'))
      }
    }

    const inlineReferences = parseInlineReferences(input)
    const mergedReferences = Array.from(new Set([...references, ...inlineReferences]))
    const mergedLoreReferences = Array.from(new Set(loreReferences))
    const inlineStyleReferences = parseInlineStyleReferences(input)
    const mergedStyleReferences = Array.from(new Set([...styleReferences, ...inlineStyleReferences]))
    return {
      message: userMessage,
      references: mergedReferences,
      loreReferences: mergedLoreReferences,
      styleReferences: mergedStyleReferences,
      textSelections,
      planMode,
    }
  }, [loreReferences, references, styleReferences, t, textSelections])

  /** 发送消息 */
  const send = useCallback(async (input: string) => {
    if (isStreaming) return
    const command = agentBypassCommand(input)
    if (command) {
      const result = await executeCommand(command)
      if (command === 'clear') {
        await loadHistory()
        await loadSessions()
        return
      }
      setMessages(prev => [...prev, { role: 'system', content: result }])
      return
    }

    let prepared: ReturnType<typeof prepareAgentRequest>
    try {
      prepared = prepareAgentRequest(input)
    } catch (e) {
      setMessages(prev => [...prev, { role: 'system', content: (e as Error).message }])
      return
    }

    // 添加用户消息
    setMessages(prev => [...prev, { role: 'user', content: input }])
    const abortController = new AbortController()
    setAbortController(abortController)

    try {
      const stream = await sendMessage(prepared.message, prepared.references, prepared.loreReferences, prepared.styleReferences, prepared.textSelections, abortController.signal, prepared.planMode)
      await consumeAgentStream(stream, { clearInputsOnFinish: clearInputState, showAbortMessage: true })
    } catch (e) {
      setMessages(prev => [...prev, { role: 'error', content: t('chat.activity.requestFailed', { error: String(e) }) }])
    }
  }, [clearInputState, consumeAgentStream, isStreaming, loadHistory, loadSessions, prepareAgentRequest, setAbortController, setMessages, t])

  const analyzeContext = useCallback(async (input: string): Promise<ContextAnalysis> => {
    if (isStreaming) throw new Error(t('chat.contextAnalysis.streamingUnavailable'))
    const prepared = prepareAgentRequest(input)
    return analyzeChatContext(prepared.message, prepared.references, prepared.loreReferences, prepared.styleReferences, prepared.textSelections, prepared.planMode)
  }, [isStreaming, prepareAgentRequest, t])

  /** 恢复订阅后台仍在运行的聊天任务。 */
  const resumeActiveChat = useCallback(async () => {
    if (isStreaming) return
    try {
      const activeTask = await getActiveChatTask()
      if (!activeTask.active) return

      const abortController = new AbortController()
      setAbortController(abortController)
      const stream = await streamActiveChat(abortController.signal)
      await consumeAgentStream(stream)
    } catch (e) {
      if (!isAbortError(e)) {
        console.error('恢复聊天流失败', e)
      }
    }
  }, [consumeAgentStream, isStreaming, setAbortController])

  /** 中断当前 AI 执行 */
  const stop = useCallback(() => {
    void abortChat()
    abortLocalStream()
  }, [abortLocalStream])

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
    analyzeContext,
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

function agentBypassCommand(input: string): string | null {
  if (!input.startsWith('/')) return null
  const cmd = input.slice(1).split(' ')[0]
  return ['clear', 'compact', 'status', 'help'].includes(cmd) ? cmd : null
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
