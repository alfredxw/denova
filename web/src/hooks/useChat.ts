import { useState, useCallback, useEffect } from 'react'
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
import type { ContextAnalysis, IDEContext, SessionSummary, TextSelection } from '@/lib/api'
import { isAbortError, isPlanProtocolToolName, normalizeRepeatedMessages, useAgentEventStream } from './useAgentEventStream'
import { fetchSettings } from '@/features/settings/api'
import { formatApprovedPlanExecutionMessage } from '@/lib/plan-mode'
import type { ChatMessage } from '@/lib/api'

interface ChatOptions {
  onAgentFileChange?: (path?: string) => void | Promise<void>
}

export interface ChatSendOptions {
  writingSkill?: string
  ideContext?: IDEContext
  imagePresetId?: string
  planMode?: boolean
  displayMessage?: string
  hideUserMessage?: boolean
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
  const [styleScenes, setStyleScenes] = useState<string[]>([])
  const [textSelections, setTextSelections] = useState<TextSelection[]>([])
  const [defaultPlanMode, setDefaultPlanMode] = useState(false)
  const [planModes, setPlanModes] = useState<Record<string, boolean>>(() => readChatPlanModes())
  const activePlanMode = planModeForSession(planModes, activeSessionId, defaultPlanMode)

  useEffect(() => {
    let cancelled = false
    fetchSettings()
      .then((data) => {
        if (!cancelled) setDefaultPlanMode(data.effective?.plan_mode_default === true)
      })
      .catch((e) => console.warn('加载 Plan Mode 默认配置失败', e))
    return () => { cancelled = true }
  }, [])

  const setSessionPlanMode = useCallback((sessionId: string, value: boolean) => {
    const id = sessionId || 'default'
    setPlanModes((current) => {
      const next = { ...current, [id]: value }
      writeChatPlanModes(next)
      return next
    })
  }, [])

  const setActivePlanMode = useCallback((value: boolean) => {
    setSessionPlanMode(activeSessionId || 'default', value)
  }, [activeSessionId, setSessionPlanMode])

  const togglePlanMode = useCallback(() => {
    setActivePlanMode(!activePlanMode)
  }, [activePlanMode, setActivePlanMode])

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
      setMessages(normalizeRepeatedMessages(filterInternalPlanMessages(msgs)))
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

  /** 添加场景风格选择 */
  const addStyleScene = useCallback((scene: string) => {
    setStyleScenes(prev => Array.from(new Set([...prev, scene])))
  }, [])

  /** 移除场景风格选择 */
  const removeStyleScene = useCallback((scene: string) => {
    setStyleScenes(prev => prev.filter(item => item !== scene))
  }, [])

  /** 清空文件引用 */
  const clearReferences = useCallback(() => {
    setReferences([])
  }, [])

  /** 清空资料库条目引用 */
  const clearLoreReferences = useCallback(() => {
    setLoreReferences([])
  }, [])

  /** 清空场景风格选择 */
  const clearStyleScenes = useCallback(() => {
    setStyleScenes([])
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
    clearStyleScenes()
    clearTextSelections()
  }, [clearLoreReferences, clearReferences, clearStyleScenes, clearTextSelections])

  const prepareAgentRequest = useCallback((input: string, forcedPlanMode?: boolean) => {
    if (input.startsWith('/')) {
      const cmd = input.slice(1).split(' ')[0]
      if (['clear', 'compact', 'status', 'help'].includes(cmd)) {
        throw new Error(t('chat.contextAnalysis.commandUnavailable'))
      }
    }

    let planMode = forcedPlanMode ?? activePlanMode
    let userMessage = input
    if (input.startsWith('/plan')) {
      planMode = true
      userMessage = input.replace(/^\/plan\s*/, '').trim()
      if (!userMessage) {
        throw new Error(t('chat.planUsage'))
      }
    }

    const inlineReferences = parseInlineReferences(userMessage)
    const mergedReferences = Array.from(new Set([...references, ...inlineReferences]))
    const mergedLoreReferences = Array.from(new Set(loreReferences))
    const inlineStyleScenes = parseInlineStyleScenes(userMessage)
    const mergedStyleScenes = Array.from(new Set([...styleScenes, ...inlineStyleScenes]))
    return {
      message: userMessage,
      references: mergedReferences,
      loreReferences: mergedLoreReferences,
      styleScenes: mergedStyleScenes,
      textSelections,
      planMode,
    }
  }, [activePlanMode, loreReferences, references, styleScenes, t, textSelections])

  /** 发送消息 */
  const send = useCallback(async (input: string, options: ChatSendOptions = {}) => {
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
      prepared = prepareAgentRequest(input, options.planMode)
    } catch (e) {
      setMessages(prev => [...prev, { role: 'system', content: (e as Error).message }])
      return
    }
    if (prepared.planMode !== activePlanMode || options.planMode !== undefined) {
      setActivePlanMode(prepared.planMode)
    }

    if (!options.hideUserMessage) {
      setMessages(prev => [...prev, { role: 'user', content: options.displayMessage || input }])
    }
    const abortController = new AbortController()
    setAbortController(abortController)

    try {
      const stream = await sendMessage(prepared.message, prepared.references, prepared.loreReferences, prepared.styleScenes, prepared.textSelections, abortController.signal, prepared.planMode, options.writingSkill, options.ideContext, options.imagePresetId)
      await consumeAgentStream(stream, { clearInputsOnFinish: clearInputState, showAbortMessage: true })
    } catch (e) {
      setMessages(prev => [...prev, { role: 'error', content: t('chat.activity.requestFailed', { error: String(e) }) }])
    }
  }, [activePlanMode, clearInputState, consumeAgentStream, isStreaming, loadHistory, loadSessions, prepareAgentRequest, setAbortController, setActivePlanMode, setMessages, t])

  const analyzeContext = useCallback(async (input: string, options: ChatSendOptions = {}): Promise<ContextAnalysis> => {
    if (isStreaming) throw new Error(t('chat.contextAnalysis.streamingUnavailable'))
    const prepared = prepareAgentRequest(input)
    return analyzeChatContext(prepared.message, prepared.references, prepared.loreReferences, prepared.styleScenes, prepared.textSelections, prepared.planMode, options.writingSkill, options.ideContext, options.imagePresetId)
  }, [isStreaming, prepareAgentRequest, t])

  const submitPlanQuestion = useCallback((message: ChatMessage, content: string, _preview: string) => {
    setMessages(prev => markPlanMessageAction(prev, message, 'answered'))
    void send(content, { planMode: true, hideUserMessage: true })
  }, [send, setMessages])

  const approveProposedPlan = useCallback((message: ChatMessage) => {
    const plan = message.content || ''
    if (!plan.trim()) return
    const planIndex = findMessageIndex(messages, message)
    const userContext = collectPlanUserContext(messages, planIndex)
    setMessages(prev => markPlanMessageAction(prev, message, 'approved'))
    void send(formatApprovedPlanExecutionMessage(plan, userContext), {
      planMode: false,
      hideUserMessage: true,
    })
  }, [messages, send, setMessages])

  const exitPlanMode = useCallback(() => {
    setActivePlanMode(false)
  }, [setActivePlanMode])

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
    styleScenes,
    textSelections,
    planMode: activePlanMode,
    setPlanMode: setActivePlanMode,
    togglePlanMode,
    send,
    analyzeContext,
    submitPlanQuestion,
    approveProposedPlan,
    exitPlanMode,
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
    addStyleScene,
    removeStyleScene,
    addTextSelection,
    removeTextSelection,
    clearReferences,
    clearStyleScenes,
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

function parseInlineStyleScenes(input: string): string[] {
  const result = new Set<string>()
  const regex = /(?:^|\s)#([^\s#]+)/g
  let match: RegExpExecArray | null
  while ((match = regex.exec(input)) !== null) {
    result.add(match[1])
  }
  return Array.from(result)
}

const CHAT_PLAN_MODES_STORAGE_KEY = 'nova.chat.plan_modes.v1'

function readChatPlanModes(): Record<string, boolean> {
  if (typeof window === 'undefined') return {}
  const raw = window.localStorage.getItem(CHAT_PLAN_MODES_STORAGE_KEY)
  if (!raw) return {}
  try {
    const parsed = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {}
    const result: Record<string, boolean> = {}
    for (const [key, value] of Object.entries(parsed)) {
      if (typeof key === 'string' && typeof value === 'boolean') result[key] = value
    }
    return result
  } catch {
    return {}
  }
}

function writeChatPlanModes(value: Record<string, boolean>) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(CHAT_PLAN_MODES_STORAGE_KEY, JSON.stringify(value))
}

function planModeForSession(planModes: Record<string, boolean>, sessionId: string, defaultValue: boolean) {
  const id = sessionId || 'default'
  return planModes[id] ?? defaultValue
}

function findMessageIndex(messages: ChatMessage[], target: ChatMessage) {
  if (target.id) {
    const byID = messages.findIndex((message) => message.id === target.id)
    if (byID >= 0) return byID
  }
  for (let i = messages.length - 1; i >= 0; i -= 1) {
    const message = messages[i]
    if (message.role === 'proposed_plan' && message.content === target.content) return i
  }
  return -1
}

function collectPlanUserContext(messages: ChatMessage[], planIndex: number) {
  const end = planIndex >= 0 ? planIndex : messages.length
  let start = 0
  for (let i = end - 1; i >= 0; i -= 1) {
    if (messages[i].role === 'proposed_plan') {
      start = i + 1
      break
    }
  }
  const userMessages = messages
    .slice(start, end)
    .filter((message) => message.role === 'user')
    .map((message) => (message.content || '').trim())
    .filter(Boolean)
  if (userMessages.length <= 1) return userMessages[0] || ''
  return [
    `原始请求：\n${userMessages[0]}`,
    `用户补充：\n${userMessages.slice(1).join('\n\n')}`,
  ].join('\n\n')
}

function filterInternalPlanMessages(messages: ChatMessage[]) {
  return messages.filter((message) => (
    !(message.role === 'user' && isPlanQuestionAnswerProtocol(message.content || '')) &&
    !isPlanProtocolToolMessage(message)
  ))
}

function isPlanQuestionAnswerProtocol(content: string) {
  return content.includes('<plan_question_answers>') || content.includes('</plan_question_answers>')
}

function isPlanProtocolToolMessage(message: ChatMessage) {
  if (message.role !== 'tool_call' && message.role !== 'tool_result') return false
  const name = message.name || (message.content || '').split(/\s+/)[0] || ''
  return isPlanProtocolToolName(name)
}

function markPlanMessageAction(
  messages: ChatMessage[],
  target: ChatMessage,
  action: NonNullable<ChatMessage['plan_action']>,
) {
  return messages.map((message) => {
    if (!isSamePlanMessage(message, target)) return message
    return { ...message, plan_action: action, streaming: false }
  })
}

function isSamePlanMessage(message: ChatMessage, target: ChatMessage) {
  if (target.id) return message.id === target.id
  return message.role === target.role && message.content === target.content
}
