import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useChat as useAIChat } from '@ai-sdk/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  analyzeChatContext,
  createAgentCommandID,
  createSession,
  deleteSession,
  executeCommand,
  renameSession,
  submitChatCommand,
  submitQueuedChatCommand,
  switchSession,
} from '@/lib/api'
import type { AgentQueuedCommandAction, AgentRuntimeQueuedCommand, ContextAnalysis, IDEContext, SessionSummary, TextSelection } from '@/lib/api'
import { fetchSettings } from '@/features/settings/api'
import { formatApprovedPlanExecutionMessage } from '@/lib/plan-mode'
import { agentCommandErrorMessage, agentCommandRetryKey, isKnownAgentCommandOutcome, rememberAgentCommandID } from '@/lib/agent-command'
import { AgentChatTransport, buildAgentChatRequestBody, normalizeAgentUIMessages, type AgentUIMessage } from '@/lib/agent-ui'
import { agentViewContent, type AgentPartRef } from '@/lib/agent-message-view'
import { isWorkspaceChangeForWorkspace, type WorkspaceChangeEvent } from '@/features/changes/types'
import {
  agentBypassCommand,
  appendDataMessage,
  buildUserMessageReferences,
  collectPlanUserContext,
  findAgentMessageView,
  markPlanUIMessageAction,
  mergeProjectedQueue,
  normalizeIDEContext,
  parseInlineReferences,
  parseInlineStyleScenes,
  planModeForSession,
  readChatPlanModes,
  writeChatPlanModes,
} from './agent-chat-state'
import { useWritingAgentHistory } from './use-agent-history'
import {
  useWritingAgentRuntimeRecovery,
  type WritingDisplayRehydrateRequest,
  type WritingTaskStatus,
} from './use-agent-runtime-recovery'

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
    comments: Array<{
      id: string
      body: string
      path?: string
      review_path?: string
      review_line?: number
    }>
  }
  loreReferenceLabels?: Record<string, string>
  onSubmissionStart?: () => void
  onSubmissionError?: () => void
}

interface QueuedComposerDraft {
  message: string
  composerReferences: string[]
  composerLoreReferences: string[]
  composerStyleScenes: string[]
  composerTextSelections: TextSelection[]
  restoreSubmission?: () => void
}

export function useAgentChat(options: ChatOptions = {}) {
  const { t } = useTranslation()
  const { workspace = '', onAgentFileChange, onWorkspaceChange } = options
  const transport = useMemo(() => new AgentChatTransport(), [])
  const [runtimeRecoverySignal, setRuntimeRecoverySignal] = useState(0)
  const [displayRehydrateRequest, setDisplayRehydrateRequest] = useState<WritingDisplayRehydrateRequest | null>(null)
  const {
    messages: uiMessages,
    setMessages: setUIMessages,
    sendMessage,
    resumeStream,
    stop: stopAIStream,
    status,
    error,
  } = useAIChat<AgentUIMessage>({
    transport,
    throttle: 60,
    onData: (part) => {
      if (part.type === 'data-agent-error') {
        const data = part.data as Record<string, unknown>
        const content = [data.content, data.message, data.error]
          .find((value): value is string => typeof value === 'string' && Boolean(value.trim()))
        toast.error(content?.trim() || t('chat.activity.unknownError'))
        return
      }
      if (part.type === 'data-agent-activity') {
        const data = part.data as Record<string, unknown>
        if (data.event === 'task_rehydrate_required' || data.code === 'agent_stream.rehydrate_required') {
          const taskID = typeof data.task_id === 'string' ? data.task_id.trim() : ''
          const cursor = typeof data.cursor === 'number' ? data.cursor : Number(data.cursor)
          if (taskID && Number.isSafeInteger(cursor) && cursor >= 0) {
            setDisplayRehydrateRequest((current) => ({
              signal: (current?.signal || 0) + 1,
              taskID,
              cursor,
              settled: data.settled === true,
              status: writingTaskStatus(data.status),
              terminalReason: typeof data.terminal_reason === 'string' ? data.terminal_reason.trim() : undefined,
              terminalReasonTruncated: data.terminal_reason_truncated === true,
            }))
          }
          return
        }
        if (data.event === 'runtime_recovery_required' || data.code === 'agent_runtime.recovery_required') {
          setRuntimeRecoverySignal((current) => current + 1)
          return
        }
      }
      if (part.type !== 'data-agent-workspace-change') return
      const event = part.data as WorkspaceChangeEvent
      if (!isWorkspaceChangeForWorkspace(event, workspace)) return
      window.dispatchEvent(new CustomEvent('nova:workspace-change', { detail: event }))
      void onWorkspaceChange?.(event)
    },
    onFinish: () => {
      void onAgentFileChange?.()
    },
  })
  const messages = useMemo(() => (
    normalizeAgentUIMessages(uiMessages).flatMap<AgentUIMessage>((message) => {
      const visibleParts = message.parts.filter((part) => part.type !== 'data-agent-error')
      if (visibleParts.length === message.parts.length) return [message]
      if (visibleParts.length === 0) return []
      return [{ ...message, parts: visibleParts }]
    })
  ), [uiMessages])
  const transportStreaming = status === 'submitted' || status === 'streaming'
  const {
    activeSessionId,
    hasEarlierMessages,
    isLoadingEarlierHistory,
    loadEarlierHistory,
    loadHistory,
    loadHistoryAuthoritative,
    loadSessions,
    sessions,
    setActiveSessionId,
  } = useWritingAgentHistory({ setMessages: setUIMessages })
  const [references, setReferences] = useState<string[]>([])
  const [loreReferences, setLoreReferences] = useState<string[]>([])
  const [styleScenes, setStyleScenes] = useState<string[]>([])
  const [textSelections, setTextSelections] = useState<TextSelection[]>([])
  const [defaultPlanMode, setDefaultPlanMode] = useState(false)
  const [planModes, setPlanModes] = useState<Record<string, boolean>>(() => readChatPlanModes())
  const [abortPending, setAbortPending] = useState(false)
  const [commandSubmitting, setCommandSubmitting] = useState(false)
  const [queueActionPendingCommandID, setQueueActionPendingCommandID] = useState('')
  const commandSubmittingRef = useRef(false)
  const retryCommandIDsRef = useRef(new Map<string, string>())
  const initialStartCommandIDsRef = useRef(new Map<string, string>())
  const queuedComposerDraftsRef = useRef(new Map<string, QueuedComposerDraft>())
  const projectedOperationIDRef = useRef('')
  const activePlanMode = planModeForSession(planModes, activeSessionId, defaultPlanMode)

  useEffect(() => {
    let cancelled = false
    fetchSettings()
      .then((data) => {
        if (!cancelled) setDefaultPlanMode(data.effective?.plan_mode_default === true)
      })
      .catch((e) => console.warn('加载 Plan Mode 默认配置失败', e))
    return () => {
      cancelled = true
    }
  }, [])

  const setSessionPlanMode = useCallback((sessionId: string, value: boolean) => {
    const id = sessionId || 'default'
    setPlanModes((current) => {
      const next = { ...current, [id]: value }
      writeChatPlanModes(next)
      return next
    })
  }, [])

  const setActivePlanMode = useCallback(
    (value: boolean) => {
      setSessionPlanMode(activeSessionId || 'default', value)
    },
    [activeSessionId, setSessionPlanMode],
  )

  const togglePlanMode = useCallback(() => {
    setActivePlanMode(!activePlanMode)
  }, [activePlanMode, setActivePlanMode])

  const notifyDisplayRehydrated = useCallback(() => {
    appendDataMessage(setUIMessages, 'data-agent-system', {
      content: t('chat.activity.displayRehydrated'),
    })
  }, [setUIMessages, t])

  const restoreDisplayTerminal = useCallback(
    (request: WritingDisplayRehydrateRequest) => {
      switch (request.status) {
        case 'error':
          toast.error(request.terminalReason || t('chat.activity.unknownError'))
          return
        case 'aborted':
          toast.info(t('chat.activity.abortMessage'))
          return
        case 'running':
        case 'done':
        case undefined:
          return
      }
    },
    [t],
  )

  const { abortRecovery, recoveryPending, resumeActiveChat, runtimeProjection, setRuntimeProjection } = useWritingAgentRuntimeRecovery({
    activeSessionId,
    displayRehydrateRequest,
    loadHistoryAuthoritative,
    onDisplayRehydrated: notifyDisplayRehydrated,
    onDisplayTerminalRestored: restoreDisplayTerminal,
    onSettled: () => setAbortPending(false),
    runtimeRecoverySignal,
    resumeStream,
    transport,
    transportError: error,
    transportStatus: status,
    transportStreaming,
  })
  const isStreaming = transportStreaming || recoveryPending
  const activityContent = recoveryPending ? t('chat.activity.recovering') : status === 'submitted' ? t('chat.activity.thinking') : ''

  useEffect(() => {
    const operationID = runtimeProjection?.active_operation_id?.trim() || ''
    if (projectedOperationIDRef.current && projectedOperationIDRef.current !== operationID) {
      setAbortPending(false)
    }
    projectedOperationIDRef.current = operationID
  }, [runtimeProjection?.active_operation_id])

  useEffect(() => {
    const queuedIDs = new Set((runtimeProjection?.queue || []).map((item) => item.command_id))
    for (const commandID of queuedComposerDraftsRef.current.keys()) {
      if (!queuedIDs.has(commandID)) queuedComposerDraftsRef.current.delete(commandID)
    }
  }, [runtimeProjection?.queue])

  const addReference = useCallback((path: string) => {
    setReferences((prev) => Array.from(new Set([...prev, path])))
  }, [])
  const addLoreReference = useCallback((id: string) => {
    setLoreReferences((prev) => Array.from(new Set([...prev, id])))
  }, [])
  const removeReference = useCallback((path: string) => {
    setReferences((prev) => prev.filter((item) => item !== path))
  }, [])
  const removeLoreReference = useCallback((id: string) => {
    setLoreReferences((prev) => prev.filter((item) => item !== id))
  }, [])
  const addStyleScene = useCallback((scene: string) => {
    setStyleScenes((prev) => Array.from(new Set([...prev, scene])))
  }, [])
  const removeStyleScene = useCallback((scene: string) => {
    setStyleScenes((prev) => prev.filter((item) => item !== scene))
  }, [])
  const clearReferences = useCallback(() => setReferences([]), [])
  const clearStyleScenes = useCallback(() => setStyleScenes([]), [])
  const addTextSelection = useCallback((sel: TextSelection) => {
    setTextSelections((prev) => [...prev, sel])
  }, [])
  const removeTextSelection = useCallback((index: number) => {
    setTextSelections((prev) => prev.filter((_, i) => i !== index))
  }, [])

  const prepareAgentRequest = useCallback(
    (input: string, forcedPlanMode?: boolean) => {
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
    },
    [activePlanMode, loreReferences, references, styleScenes, t, textSelections],
  )

  const send = useCallback(
    async (input: string, sendOptions: ChatSendOptions = {}) => {
      const command = isStreaming ? '' : agentBypassCommand(input)
      if (command) {
        const result = await executeCommand(command)
        if (command === 'clear') {
          await loadHistory()
          await loadSessions()
          return true
        }
        appendDataMessage(setUIMessages, 'data-agent-system', {
          content: result,
        })
        return true
      }

      let prepared: ReturnType<typeof prepareAgentRequest>
      try {
        prepared = prepareAgentRequest(input, sendOptions.planMode)
      } catch (e) {
        toast.error((e as Error).message)
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
        selections: prepared.textSelections.map((s) => ({
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
      } as Parameters<typeof buildAgentChatRequestBody>[0] & {
        message: string
      }) as Record<string, unknown>
      body.message = prepared.message

      const userReferences = buildUserMessageReferences(prepared, sendOptions)
      if (isStreaming) {
        if (abortPending || commandSubmittingRef.current) return false
        const operationID = runtimeProjection?.active_operation_id?.trim()
        if (!runtimeProjection?.active || !operationID) {
          toast.error(t('chat.runtime.operationUnavailable'))
          return false
        }
        const delivery = 'follow_up' as const
        const retryKey = agentCommandRetryKey(operationID, delivery, body)
        const commandID = rememberAgentCommandID(retryCommandIDsRef.current, retryKey, createAgentCommandID)
        commandSubmittingRef.current = true
        setCommandSubmitting(true)
        try {
          const receipt = await submitChatCommand(delivery, commandID, operationID, body)
          retryCommandIDsRef.current.delete(retryKey)
          queuedComposerDraftsRef.current.set(commandID, {
            message: prepared.message,
            composerReferences: prepared.composerReferences,
            composerLoreReferences: prepared.composerLoreReferences,
            composerStyleScenes: prepared.composerStyleScenes,
            composerTextSelections: prepared.composerTextSelections,
            restoreSubmission: sendOptions.onSubmissionError,
          })
          setRuntimeProjection((current) => {
            if (!current || current.active_operation_id !== operationID) return current
            return {
              ...current,
              cursor: receipt.cursor,
              active_operation_id: receipt.operation_id,
              recovery_paused: false,
              runtime_recoverable: false,
              recovery_actions: [],
              queue: mergeProjectedQueue(current.queue, {
                command_id: commandID,
                operation_id: receipt.operation_id,
                delivery,
                message: prepared.message,
              }),
            }
          })
          setReferences((current) => current.filter((item) => !prepared.composerReferences.includes(item)))
          setLoreReferences((current) => current.filter((item) => !prepared.composerLoreReferences.includes(item)))
          setStyleScenes((current) => current.filter((item) => !prepared.composerStyleScenes.includes(item)))
          setTextSelections((current) => current.filter((item) => !prepared.composerTextSelections.includes(item)))
          sendOptions.onSubmissionStart?.()
          return true
        } catch (error) {
          if (isKnownAgentCommandOutcome(error)) retryCommandIDsRef.current.delete(retryKey)
          sendOptions.onSubmissionError?.()
          toast.error(agentCommandErrorMessage(error, t))
          return false
        } finally {
          commandSubmittingRef.current = false
          setCommandSubmitting(false)
        }
      }
      const initialRetryKey = agentCommandRetryKey('', 'start_turn', body)
      const initialCommandID = rememberAgentCommandID(initialStartCommandIDsRef.current, initialRetryKey, createAgentCommandID)
      body.command_id = initialCommandID
      let submissionStarted = false
      try {
        const pendingRequest = sendMessage(
          {
            role: 'user',
            metadata: {
              ...(sendOptions.hideUserMessage ? { display_hidden: true } : {}),
              ...(userReferences.length ? { user_references: userReferences } : {}),
            },
            parts: [{ type: 'text', text: sendOptions.displayMessage || input }],
          },
          { body },
        )
        setReferences((current) => current.filter((item) => !prepared.composerReferences.includes(item)))
        setLoreReferences((current) => current.filter((item) => !prepared.composerLoreReferences.includes(item)))
        setStyleScenes((current) => current.filter((item) => !prepared.composerStyleScenes.includes(item)))
        setTextSelections((current) => current.filter((item) => !prepared.composerTextSelections.includes(item)))
        submissionStarted = true
        sendOptions.onSubmissionStart?.()
        await pendingRequest
        transport.takeInitialSubmissionOutcome(initialCommandID)
        initialStartCommandIDsRef.current.delete(initialRetryKey)
        return true
      } catch (e) {
        const acceptance = transport.takeInitialSubmissionOutcome(initialCommandID)
        if (acceptance !== 'uncertain') initialStartCommandIDsRef.current.delete(initialRetryKey)
        if (submissionStarted) {
          setReferences((current) => Array.from(new Set([...prepared.composerReferences, ...current])))
          setLoreReferences((current) => Array.from(new Set([...prepared.composerLoreReferences, ...current])))
          setStyleScenes((current) => Array.from(new Set([...prepared.composerStyleScenes, ...current])))
          setTextSelections((current) => [...prepared.composerTextSelections.filter((item) => !current.includes(item)), ...current])
          sendOptions.onSubmissionError?.()
        }
        toast.error(t('chat.activity.requestFailed', { error: String(e) }))
        return false
      }
    },
    [
      abortPending,
      activePlanMode,
      isStreaming,
      loadHistory,
      loadSessions,
      prepareAgentRequest,
      runtimeProjection,
      sendMessage,
      setActivePlanMode,
      setUIMessages,
      t,
      transport,
    ],
  )

  const submitQueuedControl = useCallback(
    async (item: AgentRuntimeQueuedCommand, action: AgentQueuedCommandAction, reason?: string) => {
      if (abortPending || commandSubmittingRef.current) return false
      const operationID = runtimeProjection?.active_operation_id?.trim()
      if (!runtimeProjection?.active || !operationID || item.operation_id !== operationID) {
        toast.error(t('chat.runtime.operationUnavailable'))
        return false
      }
      if (!runtimeProjection.queue?.some((candidate) => candidate.command_id === item.command_id)) {
        toast.error(t('chat.runtime.invalidCommand'))
        return false
      }

      const retryKey = agentCommandRetryKey(operationID, action, {
        target_command_id: item.command_id,
        ...(reason ? { reason } : {}),
      })
      const commandID = rememberAgentCommandID(retryCommandIDsRef.current, retryKey, createAgentCommandID)
      commandSubmittingRef.current = true
      setCommandSubmitting(true)
      setQueueActionPendingCommandID(item.command_id)
      try {
        const receipt = await submitQueuedChatCommand(action, commandID, operationID, item.command_id, reason)
        retryCommandIDsRef.current.delete(retryKey)
        setRuntimeProjection((current) => {
          if (!current || current.active_operation_id !== operationID) return current
          const queue = action === 'cancel_queued'
            ? (current.queue || []).filter((candidate) => candidate.command_id !== item.command_id)
            : (current.queue || []).map((candidate) => candidate.command_id === item.command_id
              ? { ...candidate, steer_requested: true }
              : candidate)
          return {
            ...current,
            cursor: receipt.cursor,
            active_operation_id: receipt.operation_id,
            ...(action === 'steer_queued' ? {
              recovery_paused: false,
              runtime_recoverable: false,
              recovery_actions: [],
            } : {}),
            queue,
          }
        })
        return true
      } catch (error) {
        if (isKnownAgentCommandOutcome(error)) retryCommandIDsRef.current.delete(retryKey)
        toast.error(agentCommandErrorMessage(error, t))
        return false
      } finally {
        commandSubmittingRef.current = false
        setCommandSubmitting(false)
        setQueueActionPendingCommandID('')
      }
    },
    [abortPending, runtimeProjection, setRuntimeProjection, t],
  )

  const steerQueuedCommand = useCallback(
    (item: AgentRuntimeQueuedCommand) => submitQueuedControl(item, 'steer_queued'),
    [submitQueuedControl],
  )

  const deleteQueuedCommand = useCallback(async (item: AgentRuntimeQueuedCommand) => {
    const accepted = await submitQueuedControl(item, 'cancel_queued', 'user_deleted')
    if (accepted) queuedComposerDraftsRef.current.delete(item.command_id)
    return accepted
  }, [submitQueuedControl])

  const editQueuedCommand = useCallback(async (item: AgentRuntimeQueuedCommand) => {
    const draft = queuedComposerDraftsRef.current.get(item.command_id)
    const accepted = await submitQueuedControl(item, 'cancel_queued', 'returned_to_editor')
    if (!accepted) return null
    queuedComposerDraftsRef.current.delete(item.command_id)
    if (draft) {
      setReferences((current) => Array.from(new Set([...draft.composerReferences, ...current])))
      setLoreReferences((current) => Array.from(new Set([...draft.composerLoreReferences, ...current])))
      setStyleScenes((current) => Array.from(new Set([...draft.composerStyleScenes, ...current])))
      setTextSelections((current) => [...draft.composerTextSelections.filter((selection) => !current.includes(selection)), ...current])
      draft.restoreSubmission?.()
    }
    return draft?.message || item.message
  }, [submitQueuedControl])

  const analyzeContext = useCallback(
    async (input: string, sendOptions: ChatSendOptions = {}): Promise<ContextAnalysis> => {
      if (isStreaming) throw new Error(t('chat.contextAnalysis.streamingUnavailable'))
      const prepared = prepareAgentRequest(input)
      return analyzeChatContext(
        prepared.message,
        prepared.references,
        prepared.loreReferences,
        prepared.styleScenes,
        prepared.textSelections,
        prepared.planMode,
        sendOptions.writingSkill,
        sendOptions.ideContext,
        sendOptions.imagePresetId,
        sendOptions.tellerId,
      )
    },
    [isStreaming, prepareAgentRequest, t],
  )

  const submitPlanQuestion = useCallback(
    (ref: AgentPartRef, content: string, _preview: string) => {
      setUIMessages((prev) => markPlanUIMessageAction(prev, ref, 'answered'))
      void send(content, { planMode: true, hideUserMessage: true })
    },
    [send, setUIMessages],
  )

  const approveProposedPlan = useCallback(
    (ref: AgentPartRef) => {
      const planView = findAgentMessageView(messages, ref)
      const plan = planView ? agentViewContent(planView) : ''
      if (!plan.trim()) return
      const userContext = collectPlanUserContext(messages, ref)
      setUIMessages((prev) => markPlanUIMessageAction(prev, ref, 'approved'))
      void send(formatApprovedPlanExecutionMessage(plan, userContext), {
        planMode: false,
        hideUserMessage: true,
      })
    },
    [messages, send, setUIMessages],
  )

  const exitPlanMode = useCallback(() => {
    setActivePlanMode(false)
  }, [setActivePlanMode])

  const stop = useCallback(async () => {
    if (abortPending || commandSubmittingRef.current) return
    let retryKey = ''
    commandSubmittingRef.current = true
    setCommandSubmitting(true)
    try {
      if (await abortRecovery()) {
        setAbortPending(true)
        return
      }
      const operationID = runtimeProjection?.active_operation_id?.trim()
      if (!runtimeProjection?.active || !operationID) {
        toast.error(t('chat.runtime.operationUnavailable'))
        return
      }
      retryKey = agentCommandRetryKey(operationID, 'abort', {
        reason: 'user_requested',
      })
      const commandID = rememberAgentCommandID(retryCommandIDsRef.current, retryKey, createAgentCommandID)
      const receipt = await submitChatCommand('abort', commandID, operationID, undefined, 'user_requested')
      retryCommandIDsRef.current.delete(retryKey)
      setAbortPending(true)
      setRuntimeProjection((current) =>
        current && current.active_operation_id === operationID
          ? {
              ...current,
              cursor: receipt.cursor,
              active_operation_id: receipt.operation_id,
            }
          : current,
      )
    } catch (error) {
      if (retryKey && isKnownAgentCommandOutcome(error)) retryCommandIDsRef.current.delete(retryKey)
      toast.error(agentCommandErrorMessage(error, t))
    } finally {
      commandSubmittingRef.current = false
      setCommandSubmitting(false)
    }
  }, [abortPending, abortRecovery, runtimeProjection, t])

  const createChatSession = useCallback(
    async (title?: string) => {
      const session = await createSession(title)
      setActiveSessionId(session.id)
      await Promise.all([loadSessions(), loadHistory(session.id)])
      await resumeActiveChat()
    },
    [loadHistory, loadSessions, resumeActiveChat],
  )

  const switchChatSession = useCallback(
    async (id: string) => {
      if (!id || id === activeSessionId) return
      const previousSessionId = activeSessionId
      if (isStreaming) stopAIStream()
      setActiveSessionId(id)

      let session: SessionSummary
      try {
        session = await switchSession(id)
      } catch (error) {
        setActiveSessionId((current) => (current === id ? previousSessionId : current))
        throw error
      }

      setActiveSessionId(session.id)
      await Promise.all([loadSessions(), loadHistory(session.id)])
      await resumeActiveChat()
    },
    [activeSessionId, isStreaming, loadHistory, loadSessions, resumeActiveChat, stopAIStream],
  )

  const renameChatSession = useCallback(
    async (id: string, title: string) => {
      await renameSession(id, title)
      await loadSessions()
    },
    [loadSessions],
  )

  const deleteChatSession = useCallback(
    async (id: string) => {
      stopAIStream()
      const session = await deleteSession(id)
      setActiveSessionId(session.id)
      await Promise.all([loadSessions(), loadHistory(session.id)])
      await resumeActiveChat()
    },
    [loadHistory, loadSessions, resumeActiveChat, stopAIStream],
  )

  return {
    messages,
    sessions,
    activeSessionId,
    isStreaming,
    runtimeProjection,
    abortPending,
    commandSubmitting,
    queueActionPendingCommandID,
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
    steerQueuedCommand,
    deleteQueuedCommand,
    editQueuedCommand,
    loadSessions,
    loadHistory,
    loadEarlierHistory,
    hasEarlierMessages,
    isLoadingEarlierHistory,
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

function writingTaskStatus(value: unknown): WritingTaskStatus | undefined {
  switch (value) {
    case 'running':
    case 'done':
    case 'aborted':
    case 'error':
      return value
    default:
      return undefined
  }
}
