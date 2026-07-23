import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useChat as useAIChat } from '@ai-sdk/react'
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
  switchSession,
} from '@/lib/api'
import type { ContextAnalysis, IDEContext, SessionSummary, TextSelection } from '@/lib/api'
import type { UserMessageReference } from '@/lib/api-client/types'
import { fetchSettings } from '@/features/settings/api'
import { formatApprovedPlanExecutionMessage } from '@/lib/plan-mode'
import {
  AgentChatTransport,
  buildAgentChatRequestBody,
  normalizeAgentUIMessages,
  type AgentUIMessage,
} from '@/lib/agent-ui'
import { agentViewContent, buildAgentMessageViews, isPlanProtocolToolName, type AgentMessageView, type AgentPartRef } from '@/lib/agent-message-view'
import { isWorkspaceChangeForWorkspace, type WorkspaceChangeEvent } from '@/features/changes/types'

interface ChatOptions {
  workspace?: string
  onAgentFileChange?: (path?: string) => void | Promise<void>
  onWorkspaceChange?: (event: WorkspaceChangeEvent) => void | Promise<void>
}

export interface ChatSendOptions {
  writingSkill?: string
  ideContext?: IDEContext
  imagePresetId?: string
  tellerId?: string
  planMode?: boolean
  displayMessage?: string
  hideUserMessage?: boolean
  reviewFeedback?: Array<{
    source?: 'workspace_change' | 'document'
    reviewThreadId: string
    commentIds: string[]
  }>
  reviewFeedbackDisplay?: {
    comments: Array<{ id: string; body: string; path?: string; review_path?: string; review_line?: number }>
  }
  loreReferenceLabels?: Record<string, string>
  onSubmissionStart?: () => void
  onSubmissionError?: () => void
}

export function useAgentChat(options: ChatOptions = {}) {
  const { t } = useTranslation()
  const { workspace = '', onAgentFileChange, onWorkspaceChange } = options
  const transport = useMemo(() => new AgentChatTransport(), [])
  const {
    messages: uiMessages,
    setMessages: setUIMessages,
    sendMessage,
    resumeStream,
    stop: stopAIStream,
    status,
  } = useAIChat<AgentUIMessage>({
    transport,
    throttle: 60,
    onData: (part) => {
      if (part.type !== 'data-agent-workspace-change') return
      const event = part.data as WorkspaceChangeEvent
      if (!isWorkspaceChangeForWorkspace(event, workspace)) return
      window.dispatchEvent(new CustomEvent('nova:workspace-change', { detail: event }))
      void onWorkspaceChange?.(event)
    },
    onFinish: () => { void onAgentFileChange?.() },
  })
  const messages = useMemo(() => normalizeAgentUIMessages(uiMessages), [uiMessages])
  const isStreaming = status === 'submitted' || status === 'streaming'
  const activityContent = status === 'submitted' ? t('chat.activity.thinking') : ''
  const [sessions, setSessions] = useState<SessionSummary[]>([])
  const [activeSessionId, setActiveSessionId] = useState('')
  const [references, setReferences] = useState<string[]>([])
  const [loreReferences, setLoreReferences] = useState<string[]>([])
  const [styleScenes, setStyleScenes] = useState<string[]>([])
  const [textSelections, setTextSelections] = useState<TextSelection[]>([])
  const [defaultPlanMode, setDefaultPlanMode] = useState(false)
  const [planModes, setPlanModes] = useState<Record<string, boolean>>(() => readChatPlanModes())
  const activePlanMode = planModeForSession(planModes, activeSessionId, defaultPlanMode)
  const activePlanModeRef = useRef(activePlanMode)
  activePlanModeRef.current = activePlanMode

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
    activePlanModeRef.current = value
    setSessionPlanMode(activeSessionId || 'default', value)
  }, [activeSessionId, setSessionPlanMode])

  const togglePlanMode = useCallback(() => {
    setActivePlanMode(!activePlanModeRef.current)
  }, [setActivePlanMode])

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

  const loadHistory = useCallback(async (sessionId?: string) => {
    try {
      const nextMessages = await getMessages(sessionId)
      setUIMessages(filterInternalPlanUIMessages(nextMessages))
    } catch (e) {
      console.error('加载历史失败', e)
    }
  }, [setUIMessages])

  const addReference = useCallback((path: string) => {
    setReferences(prev => Array.from(new Set([...prev, path])))
  }, [])
  const addLoreReference = useCallback((id: string) => {
    setLoreReferences(prev => Array.from(new Set([...prev, id])))
  }, [])
  const removeReference = useCallback((path: string) => {
    setReferences(prev => prev.filter(item => item !== path))
  }, [])
  const removeLoreReference = useCallback((id: string) => {
    setLoreReferences(prev => prev.filter(item => item !== id))
  }, [])
  const addStyleScene = useCallback((scene: string) => {
    setStyleScenes(prev => Array.from(new Set([...prev, scene])))
  }, [])
  const removeStyleScene = useCallback((scene: string) => {
    setStyleScenes(prev => prev.filter(item => item !== scene))
  }, [])
  const clearReferences = useCallback(() => setReferences([]), [])
  const clearStyleScenes = useCallback(() => setStyleScenes([]), [])
  const addTextSelection = useCallback((sel: TextSelection) => {
    setTextSelections(prev => [...prev, sel])
  }, [])
  const removeTextSelection = useCallback((index: number) => {
    setTextSelections(prev => prev.filter((_, i) => i !== index))
  }, [])

  const prepareAgentRequest = useCallback((input: string, forcedPlanMode?: boolean) => {
    if (input.startsWith('/')) {
      const cmd = input.slice(1).split(' ')[0]
      if (['clear', 'compact', 'status', 'help'].includes(cmd)) {
        throw new Error(t('chat.contextAnalysis.commandUnavailable'))
      }
    }

    let planMode = forcedPlanMode ?? activePlanModeRef.current
    let userMessage = input
    if (input.startsWith('/plan')) {
      planMode = true
      userMessage = input.replace(/^\/plan\s*/, '').trim()
      if (!userMessage) throw new Error(t('chat.planUsage'))
    }

    const inlineReferences = parseInlineReferences(userMessage)
    const inlineStyleScenes = parseInlineStyleScenes(userMessage)
    return {
      message: userMessage,
      references: Array.from(new Set([...references, ...inlineReferences])),
      loreReferences: Array.from(new Set(loreReferences)),
      styleScenes: Array.from(new Set([...styleScenes, ...inlineStyleScenes])),
      textSelections,
      composerReferences: references,
      composerLoreReferences: loreReferences,
      composerStyleScenes: styleScenes,
      composerTextSelections: textSelections,
      planMode,
    }
  }, [loreReferences, references, styleScenes, t, textSelections])

  const send = useCallback(async (input: string, sendOptions: ChatSendOptions = {}) => {
    if (isStreaming) return false
    const command = agentBypassCommand(input)
    if (command) {
      const result = await executeCommand(command)
      if (command === 'clear') {
        await loadHistory()
        await loadSessions()
        return true
      }
      appendDataMessage(setUIMessages, 'data-agent-system', { content: result })
      return true
    }

    let prepared: ReturnType<typeof prepareAgentRequest>
    try {
      prepared = prepareAgentRequest(input, sendOptions.planMode)
    } catch (e) {
      appendDataMessage(setUIMessages, 'data-agent-system', { content: (e as Error).message })
      return false
    }
    if (prepared.planMode !== activePlanMode || sendOptions.planMode !== undefined) {
      setActivePlanMode(prepared.planMode)
    }

    const body = buildAgentChatRequestBody({
      message: prepared.message,
      references: prepared.references,
      lore_references: prepared.loreReferences,
      style_scenes: prepared.styleScenes,
      selections: prepared.textSelections.map(s => ({
        file_name: s.fileName,
        start_line: s.startLine,
        end_line: s.endLine,
        content: s.content,
      })),
      ide_context: normalizeIDEContext(sendOptions.ideContext),
      plan_mode: prepared.planMode,
      writing_skill: sendOptions.writingSkill,
      image_preset_id: sendOptions.imagePresetId,
      teller_id: sendOptions.tellerId,
      review_feedback: sendOptions.reviewFeedback?.map((feedback) => ({
        source: feedback.source,
        review_thread_id: feedback.reviewThreadId,
        comment_ids: feedback.commentIds,
      })),
    } as Parameters<typeof buildAgentChatRequestBody>[0] & { message: string }) as Record<string, unknown>
    body.message = prepared.message

    const userReferences = buildUserMessageReferences(prepared, sendOptions)
    let submissionStarted = false
    try {
      const pendingRequest = sendMessage({
        role: 'user',
        metadata: {
          ...(sendOptions.hideUserMessage ? { display_hidden: true } : {}),
          ...(userReferences.length ? { user_references: userReferences } : {}),
        },
        parts: [{ type: 'text', text: sendOptions.displayMessage || input }],
      }, { body })
      setReferences((current) => current.filter((item) => !prepared.composerReferences.includes(item)))
      setLoreReferences((current) => current.filter((item) => !prepared.composerLoreReferences.includes(item)))
      setStyleScenes((current) => current.filter((item) => !prepared.composerStyleScenes.includes(item)))
      setTextSelections((current) => current.filter((item) => !prepared.composerTextSelections.includes(item)))
      submissionStarted = true
      sendOptions.onSubmissionStart?.()
      await pendingRequest
      return true
    } catch (e) {
      if (submissionStarted) {
        setReferences((current) => Array.from(new Set([...prepared.composerReferences, ...current])))
        setLoreReferences((current) => Array.from(new Set([...prepared.composerLoreReferences, ...current])))
        setStyleScenes((current) => Array.from(new Set([...prepared.composerStyleScenes, ...current])))
        setTextSelections((current) => [...prepared.composerTextSelections.filter((item) => !current.includes(item)), ...current])
        sendOptions.onSubmissionError?.()
      }
      appendDataMessage(setUIMessages, 'data-agent-error', { content: t('chat.activity.requestFailed', { error: String(e) }) })
      return false
    }
  }, [activePlanMode, isStreaming, loadHistory, loadSessions, prepareAgentRequest, sendMessage, setActivePlanMode, setUIMessages, t])

  const analyzeContext = useCallback(async (input: string, sendOptions: ChatSendOptions = {}): Promise<ContextAnalysis> => {
    if (isStreaming) throw new Error(t('chat.contextAnalysis.streamingUnavailable'))
    const prepared = prepareAgentRequest(input)
    return analyzeChatContext(prepared.message, prepared.references, prepared.loreReferences, prepared.styleScenes, prepared.textSelections, prepared.planMode, sendOptions.writingSkill, sendOptions.ideContext, sendOptions.imagePresetId, sendOptions.tellerId)
  }, [isStreaming, prepareAgentRequest, t])

  const submitPlanQuestion = useCallback((ref: AgentPartRef, content: string, _preview: string) => {
    setUIMessages(prev => markPlanUIMessageAction(prev, ref, 'answered'))
    void send(content, { planMode: true, hideUserMessage: true })
  }, [send, setUIMessages])

  const approveProposedPlan = useCallback((ref: AgentPartRef) => {
    const planView = findAgentMessageView(messages, ref)
    const plan = planView ? agentViewContent(planView) : ''
    if (!plan.trim()) return
    const userContext = collectPlanUserContext(messages, ref)
    setUIMessages(prev => markPlanUIMessageAction(prev, ref, 'approved'))
    void send(formatApprovedPlanExecutionMessage(plan, userContext), {
      planMode: false,
      hideUserMessage: true,
    })
  }, [messages, send, setUIMessages])

  const exitPlanMode = useCallback(() => {
    setActivePlanMode(false)
  }, [setActivePlanMode])

  const resumeActiveChat = useCallback(async () => {
    if (isStreaming) return
    try {
      const activeTask = await getActiveChatTask()
      if (!activeTask.active) return
      await resumeStream()
    } catch (e) {
      if (!isAbortError(e)) console.error('恢复聊天流失败', e)
    }
  }, [isStreaming, resumeStream])

  const stop = useCallback(() => {
    void abortChat()
    stopAIStream()
  }, [stopAIStream])

  const createChatSession = useCallback(async (title?: string) => {
    const session = await createSession(title)
    setActiveSessionId(session.id)
    await Promise.all([loadSessions(), loadHistory(session.id)])
    await resumeActiveChat()
  }, [loadHistory, loadSessions, resumeActiveChat])

  const switchChatSession = useCallback(async (id: string) => {
    if (!id || id === activeSessionId) return
    const session = await switchSession(id)
    setActiveSessionId(session.id)
    await Promise.all([loadSessions(), loadHistory(session.id)])
    await resumeActiveChat()
  }, [activeSessionId, loadHistory, loadSessions, resumeActiveChat])

  const renameChatSession = useCallback(async (id: string, title: string) => {
    await renameSession(id, title)
    await loadSessions()
  }, [loadSessions])

  const deleteChatSession = useCallback(async (id: string) => {
    stopAIStream()
    const session = await deleteSession(id)
    setActiveSessionId(session.id)
    await Promise.all([loadSessions(), loadHistory(session.id)])
    await resumeActiveChat()
  }, [loadHistory, loadSessions, resumeActiveChat, stopAIStream])

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

function buildUserMessageReferences(
  prepared: {
    references: string[]
    loreReferences: string[]
    styleScenes: string[]
    textSelections: TextSelection[]
  },
  options: ChatSendOptions,
): UserMessageReference[] {
  const result: UserMessageReference[] = []
  for (const path of prepared.references) result.push({ kind: 'file', label: path })
  for (const id of prepared.loreReferences) result.push({ kind: 'lore', id, label: options.loreReferenceLabels?.[id] || id })
  for (const scene of prepared.styleScenes) result.push({ kind: 'style', label: scene })
  for (const selection of prepared.textSelections) {
    result.push({
      kind: 'selection',
      label: selection.fileName,
      start_line: selection.startLine,
      end_line: selection.endLine,
      detail: boundedReferenceDetail(selection.content),
    })
  }
  for (const comment of options.reviewFeedbackDisplay?.comments ?? []) {
    result.push({
      kind: 'review_comment',
      id: comment.id,
      label: comment.review_path || comment.path || comment.id,
      ...(comment.review_line !== undefined ? { start_line: comment.review_line, end_line: comment.review_line } : {}),
      detail: boundedReferenceDetail(comment.body),
    })
  }
  return result
}

function boundedReferenceDetail(value: string): string {
  const normalized = value.trim()
  return normalized.length > 512 ? `${normalized.slice(0, 512)}…` : normalized
}

function normalizeIDEContext(context?: IDEContext) {
  if (!context?.currentFile && !context?.openFiles?.length) return undefined
  return {
    current_file: context.currentFile || undefined,
    open_files: context.openFiles?.length ? context.openFiles : undefined,
  }
}

function appendDataMessage(setUIMessages: (updater: (messages: AgentUIMessage[]) => AgentUIMessage[]) => void, type: `data-agent-${string}`, data: Record<string, unknown>) {
  setUIMessages(messages => [
    ...messages,
    {
      id: `${type}-${Date.now()}-${messages.length}`,
      role: 'assistant',
      parts: [{ type, data, id: `${type}-${Date.now()}` } as AgentUIMessage['parts'][number]],
    } as AgentUIMessage,
  ])
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
  while ((match = regex.exec(input)) !== null) result.add(match[1])
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

function findAgentMessageView(messages: AgentUIMessage[], ref: AgentPartRef): AgentMessageView | undefined {
  return buildAgentMessageViews(messages).find((view) => sameAgentPartRef(view.ref, ref))
}

function collectPlanUserContext(messages: AgentUIMessage[], target: AgentPartRef) {
  const views = buildAgentMessageViews(messages)
  const planIndex = views.findIndex((view) => sameAgentPartRef(view.ref, target))
  const end = planIndex >= 0 ? planIndex : views.length
  let start = 0
  for (let i = end - 1; i >= 0; i -= 1) {
    if (views[i].kind === 'proposed-plan') {
      start = i + 1
      break
    }
  }
  const userMessages = views
    .slice(start, end)
    .filter((view) => view.kind === 'user')
    .map((view) => agentViewContent(view).trim())
    .filter(Boolean)
  if (userMessages.length <= 1) return userMessages[0] || ''
  return [
    `原始请求：\n${userMessages[0]}`,
    `用户补充：\n${userMessages.slice(1).join('\n\n')}`,
  ].join('\n\n')
}

function filterInternalPlanUIMessages(messages: AgentUIMessage[]) {
  return messages.filter((message) => {
    const text = message.parts.map(part => part.type === 'text' ? part.text : '').join('')
    if (message.role === 'user' && isPlanQuestionAnswerProtocol(text)) return false
    return !message.parts.some(part => isPlanProtocolToolPart(part))
  })
}

function isPlanQuestionAnswerProtocol(content: string) {
  return content.includes('<plan_question_answers>') || content.includes('</plan_question_answers>')
}

function isPlanProtocolToolPart(part: AgentUIMessage['parts'][number]) {
  if (part.type === 'dynamic-tool') return isPlanProtocolToolName(part.toolName)
  if (part.type.startsWith('tool-')) return isPlanProtocolToolName(part.type.replace(/^tool-/, ''))
  return false
}

function markPlanUIMessageAction(
  messages: AgentUIMessage[],
  target: AgentPartRef,
  action: AgentPlanAction,
) {
  return messages.map(message => ({
    ...message,
    parts: message.parts.map((part, index) => {
      const raw = part as Record<string, unknown>
      const type = typeof raw.type === 'string' ? raw.type : ''
      if (!type.startsWith('data-agent-plan-')) return part
      const data = 'data' in part && part.data && typeof part.data === 'object' && !Array.isArray(part.data)
        ? part.data as Record<string, unknown>
        : {}
      const partID = 'id' in part && typeof part.id === 'string' ? part.id : `${message.id}:${index}`
      const candidate = { messageId: message.id, partId: partID, partIndex: index, type }
      if (!sameAgentPartRef(candidate, target)) return part
      return { ...part, data: { ...data, plan_action: action, status: 'success' } } as AgentUIMessage['parts'][number]
    }),
  }))
}

type AgentPlanAction = 'answered' | 'approved' | 'continue' | 'exited'

function sameAgentPartRef(left: AgentPartRef, right: AgentPartRef) {
  return left.messageId === right.messageId
    && left.partIndex === right.partIndex
    && left.partId === right.partId
    && left.type === right.type
}

function isAbortError(error: unknown) {
  return error instanceof DOMException && error.name === 'AbortError'
}
