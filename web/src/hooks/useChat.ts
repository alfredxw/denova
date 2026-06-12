import { useState, useCallback } from 'react'
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
import type { SessionSummary, TextSelection } from '@/lib/api'
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
    setAbortController(abortController)

    try {
      const stream = await sendMessage(userMessage, mergedReferences, mergedLoreReferences, mergedStyleReferences, textSelections, abortController.signal, planMode)
      await consumeAgentStream(stream, { clearInputsOnFinish: clearInputState, showAbortMessage: true })
    } catch (e) {
      setMessages(prev => [...prev, { role: 'error', content: t('chat.activity.requestFailed', { error: String(e) }) }])
    }
  }, [clearInputState, consumeAgentStream, isStreaming, loadHistory, loadSessions, loreReferences, references, setAbortController, setMessages, styleReferences, t, textSelections])

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
