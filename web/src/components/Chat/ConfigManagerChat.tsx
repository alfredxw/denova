import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { UIMessageChunk } from 'ai'
import { InputArea } from './InputArea'
import { MessageList } from './MessageList'
import {
  answerConfigManagerAsk,
  cancelConfigManagerAsk,
  clearConfigManagerSession,
  createAgentCommandID,
  getActiveConfigManagerTask,
  getConfigManagerMessagesPage,
  reconnectConfigManagerStream,
  recoverConfigManagerRuntime,
  runConfigManagerStream,
} from '@/lib/api'
import type { ActiveChatTask, AgentAskAnswer, AgentRuntimeRecoveryAction, ConfigManagerRunRequest } from '@/lib/api'
import { useSkillCommands } from '@/hooks/useSkillCommands'
import { selectAgentTokenUsageRecords, type AgentMessageView } from '@/lib/agent-message-view'
import { createAgentDataMessage, createAgentTextMessage, useAgentUIMessageStream } from '@/hooks/useAgentUIMessageStream'
import { agentCommandRetryKey, isKnownAgentCommandOutcome, rememberAgentCommandID } from '@/lib/agent-command'
import { normalizeAgentUIMessages } from '@/lib/agent-ui'

interface ConfigManagerChatProps {
  workspace?: string
  origin: string
  resourceId?: string
  storyId?: string
  branchId?: string
  context?: Record<string, string>
  onMutated?: () => void
  className?: string
}

export function ConfigManagerChat({ workspace = '', origin, resourceId, storyId, branchId, context, onMutated, className = '' }: ConfigManagerChatProps) {
  const { t } = useTranslation()
  const activeKeyRef = useRef('')
  const handledToolViewsRef = useRef(new Set<string>())
  const startCommandIDsRef = useRef(new Map<string, string>())
  const runtimeProjectionRef = useRef<ActiveChatTask | null>(null)
  const runtimeInspectionRef = useRef<{ key: string; promise: Promise<void> } | null>(null)
  const inspectRuntimeRef = useRef<(attachStream: boolean) => Promise<void>>(async () => {})
  const attachScopedTaskRef = useRef<(taskID: string, key: string) => Promise<void>>(async () => {})
  const attachedTaskIDRef = useRef('')
  const immediateReconnectRef = useRef('')
  const [error, setError] = useState<string | null>(null)
  const [runtimeProjection, setRuntimeProjectionState] = useState<ActiveChatTask | null>(null)
  const [recoveryPending, setRecoveryPending] = useState(false)
  const [abortPending, setAbortPending] = useState(false)
  const [historyBefore, setHistoryBefore] = useState('0')
  const [hasEarlierMessages, setHasEarlierMessages] = useState(false)
  const [historyLoading, setHistoryLoading] = useState(false)
  const [inputAreaHeight, setInputAreaHeight] = useState(0)
  const skills = useSkillCommands({ agentKey: 'config_manager', workspace })
  const scope = useMemo(() => ({
    origin,
    resource_id: resourceId,
    story_id: storyId,
    branch_id: branchId,
  }), [branchId, origin, resourceId, storyId])
  const chatKey = useMemo(() => [
    'config-manager',
    workspace,
    origin,
    resourceId || '',
    storyId || '',
    branchId || '',
  ].join(':'), [branchId, origin, resourceId, storyId, workspace])

  const handleStreamView = useCallback((view: AgentMessageView) => {
    if (view.kind === 'tool' && view.status === 'success') {
      const key = `${view.messageId}:${view.partId}:${view.status}`
      if (!handledToolViewsRef.current.has(key)) {
        handledToolViewsRef.current.add(key)
        onMutated?.()
      }
    }
    if (
      view.kind === 'activity'
      && (view.data?.event === 'runtime_recovery_required' || view.data?.code === 'agent_runtime.recovery_required')
    ) {
      void inspectRuntimeRef.current(false)
    }
  }, [onMutated])
  const {
    messages,
    setMessages,
    isStreaming: running,
    consumeAgentUIStream,
    abortLocalStream,
  } = useAgentUIMessageStream({ onView: handleStreamView })
  const tokenUsageMessages = useMemo(
    () => selectAgentTokenUsageRecords(messages),
    [messages],
  )
  const messageListBottomPadding = inputAreaHeight > 0 ? inputAreaHeight + 20 : undefined

  const loadMessages = useCallback(async () => {
    if (!workspace) {
      setMessages([])
      return
    }
    const key = chatKey
    try {
      const page = await getConfigManagerMessagesPage(scope)
      if (activeKeyRef.current === key) {
        setMessages(page.messages)
        setHistoryBefore(page.nextBefore)
        setHasEarlierMessages(page.hasMore)
      }
    } catch (err) {
      if (activeKeyRef.current === key) {
        setError(`${t('configManager.historyLoadFailed')}: ${errorMessage(err)}`)
      }
    }
  }, [chatKey, scope, setMessages, t, workspace])

  const loadEarlierMessages = useCallback(async () => {
    if (!hasEarlierMessages || historyLoading) return
    const key = chatKey
    const before = historyBefore
    setHistoryLoading(true)
    try {
      const page = await getConfigManagerMessagesPage(scope, { before })
      if (activeKeyRef.current !== key) return
      setMessages((current) => normalizeAgentUIMessages([...page.messages, ...current]))
      setHistoryBefore(page.nextBefore)
      setHasEarlierMessages(page.hasMore)
    } catch (err) {
      if (activeKeyRef.current === key) {
        setError(`${t('configManager.historyLoadFailed')}: ${errorMessage(err)}`)
      }
    } finally {
      if (activeKeyRef.current === key) setHistoryLoading(false)
    }
  }, [chatKey, hasEarlierMessages, historyBefore, historyLoading, scope, setMessages, t])

  const setRuntimeProjection = useCallback((projection: ActiveChatTask | null) => {
    runtimeProjectionRef.current = projection
    setRuntimeProjectionState(projection)
    if (projection?.pending_ask) {
      setMessages((current) => normalizeAgentUIMessages([
        ...current,
        createAgentDataMessage('agent-ask', { ...projection.pending_ask }),
      ]))
    }
    setRecoveryPending(Boolean(
      projection?.runtime_recoverable
      || (projection?.active && attachedTaskIDRef.current === ''),
    ))
  }, [setMessages])

  const finishScopedStream = useCallback(async (key: string, taskID: string) => {
    if (attachedTaskIDRef.current === taskID) attachedTaskIDRef.current = ''
    if (activeKeyRef.current !== key) return
    await loadMessages()
    try {
      const projection = await getActiveConfigManagerTask(scope)
      if (activeKeyRef.current !== key) return
      setRuntimeProjection(projection)
      const projectedTaskID = projection.task_id?.trim() || ''
      if (projection.active && projectedTaskID) {
        const reconnectFingerprint = `${projectedTaskID}:${projection.stream_cursor || 0}`
        if (immediateReconnectRef.current !== reconnectFingerprint) {
          immediateReconnectRef.current = reconnectFingerprint
          void attachScopedTaskRef.current(projectedTaskID, key)
        }
      } else {
        immediateReconnectRef.current = ''
      }
    } catch (err) {
      if (activeKeyRef.current === key) {
        setRecoveryPending(true)
        setError(`${t('configManager.reconnectFailed')}: ${errorMessage(err)}`)
      }
    }
  }, [loadMessages, scope, setRuntimeProjection, t])

  const consumeScopedStream = useCallback(async (
    stream: ReadableStream<UIMessageChunk>,
    key: string,
    taskID: string,
  ) => {
    try {
      await consumeAgentUIStream(stream, {
        shouldContinue: () => activeKeyRef.current === key,
      })
    } catch (err) {
      if (activeKeyRef.current === key && !isAbortLike(err)) {
        setRecoveryPending(true)
        setError(`${t('configManager.reconnectFailed')}: ${errorMessage(err)}`)
      }
    } finally {
      await finishScopedStream(key, taskID)
    }
  }, [consumeAgentUIStream, finishScopedStream, t])

  const attachScopedTask = useCallback(async (taskID: string, key: string) => {
    const exactTaskID = taskID.trim()
    if (!exactTaskID || activeKeyRef.current !== key || attachedTaskIDRef.current === exactTaskID) return
    attachedTaskIDRef.current = exactTaskID
    setRecoveryPending(true)
    try {
      const stream = await reconnectConfigManagerStream(scope, exactTaskID)
      if (activeKeyRef.current !== key) {
        if (attachedTaskIDRef.current === exactTaskID) attachedTaskIDRef.current = ''
        return
      }
      setError(null)
      void consumeScopedStream(stream, key, exactTaskID)
    } catch (err) {
      if (attachedTaskIDRef.current === exactTaskID) attachedTaskIDRef.current = ''
      if (activeKeyRef.current === key && !isAbortLike(err)) {
        setRecoveryPending(true)
        setError(`${t('configManager.reconnectFailed')}: ${errorMessage(err)}`)
      }
    }
  }, [consumeScopedStream, scope, t])
  attachScopedTaskRef.current = attachScopedTask

  const inspectRuntime = useCallback(async (attachStream: boolean) => {
    if (!workspace) return
    const key = chatKey
    if (activeKeyRef.current !== key) return
    if (runtimeInspectionRef.current?.key === key) {
      await runtimeInspectionRef.current.promise
      return
    }
    const inspection = (async () => {
      try {
        let projection = await getActiveConfigManagerTask(scope)
        if (activeKeyRef.current !== key) return
        let taskID = projection.task_id?.trim() || ''
        const actions = configManagerRecoveryActionsToSubmit(projection)
        for (const action of actions) {
          const receipt = await recoverConfigManagerRuntime(scope, action)
          if (!sameRecoveryAction(receipt.recovery_action, action)) {
            throw new Error('Config Manager recovery identity changed')
          }
          taskID = receipt.task_id?.trim() || taskID
        }
        projection = projectConfigManagerRecovery(projection, actions, taskID)
        setRuntimeProjection(projection)
        if (attachStream && taskID && (projection.active || projection.runtime_recoverable || actions.length > 0)) {
          await attachScopedTask(taskID, key)
        }
      } catch (err) {
        if (activeKeyRef.current === key && !isAbortLike(err)) {
          setRecoveryPending(true)
          setError(`${t('configManager.recoveryFailed')}: ${errorMessage(err)}`)
        }
      }
    })()
    const trackedInspection = { key, promise: inspection }
    runtimeInspectionRef.current = trackedInspection
    try {
      await inspection
    } finally {
      if (runtimeInspectionRef.current === trackedInspection) runtimeInspectionRef.current = null
    }
  }, [attachScopedTask, chatKey, scope, setRuntimeProjection, t, workspace])
  inspectRuntimeRef.current = inspectRuntime

  useEffect(() => {
    activeKeyRef.current = chatKey
    attachedTaskIDRef.current = ''
    immediateReconnectRef.current = ''
    runtimeProjectionRef.current = null
    setRuntimeProjectionState(null)
    setError(null)
    // Admission stays closed until the exact scope has been inspected. This
    // prevents an initial /active request from opening a duplicate reconnect
    // stream beside a newly accepted POST stream.
    setRecoveryPending(Boolean(workspace))
    setAbortPending(false)
    setHistoryBefore('0')
    setHasEarlierMessages(false)
    setHistoryLoading(false)
    handledToolViewsRef.current = new Set()
    void loadMessages().then(() => inspectRuntime(true))
    return () => {
      if (activeKeyRef.current === chatKey) activeKeyRef.current = ''
      attachedTaskIDRef.current = ''
    }
  }, [chatKey, inspectRuntime, loadMessages])

  useEffect(() => {
    if (!recoveryPending || running) return
    const retry = () => void inspectRuntime(true)
    const retryWhenVisible = () => {
      if (document.visibilityState === 'visible') retry()
    }
    window.addEventListener('online', retry)
    window.addEventListener('focus', retry)
    document.addEventListener('visibilitychange', retryWhenVisible)
    return () => {
      window.removeEventListener('online', retry)
      window.removeEventListener('focus', retry)
      document.removeEventListener('visibilitychange', retryWhenVisible)
    }
  }, [inspectRuntime, recoveryPending, running])

  const appendErrorMessage = (content: string) => {
    setMessages((current) => [...current, createAgentDataMessage('agent-error', { content })])
  }

  const send = async (message: string) => {
    const instruction = message.trim()
    if (!instruction || running || recoveryPending) return
    if (instruction === '/clear') {
      setError(null)
      try {
        await clearConfigManagerSession(scope)
        setRuntimeProjection(null)
        setHistoryBefore('0')
        setHasEarlierMessages(false)
        setMessages([createAgentDataMessage('agent-clear', { created_at: new Date().toISOString() })])
      } catch (err) {
        appendErrorMessage(`${t('configManager.clearFailed')}: ${errorMessage(err)}`)
      }
      return
    }
    setMessages((current) => [...current, createAgentTextMessage('user', instruction)])
    setError(null)
    setRecoveryPending(true)
    const activeChatKey = chatKey
    const retryKey = agentCommandRetryKey(activeChatKey, 'start', { instruction, ...scope, context })
    const commandID = rememberAgentCommandID(startCommandIDsRef.current, retryKey, createAgentCommandID)
    const req: ConfigManagerRunRequest = {
      command_id: commandID,
      instruction,
      ...scope,
      context,
    }
    let stream: ReadableStream<UIMessageChunk>
    try {
      stream = await runConfigManagerStream(req)
    } catch (err) {
      if (isKnownAgentCommandOutcome(err)) startCommandIDsRef.current.delete(retryKey)
      if (activeKeyRef.current === activeChatKey) {
        appendErrorMessage(`${t('configManager.runFailed')}: ${errorMessage(err)}`)
        void inspectRuntime(true)
      }
      return
    }
    // The backend has durably accepted the command. Scoped /active now owns
    // refresh and reconnect identity even if this component is remounted.
    startCommandIDsRef.current.delete(retryKey)
    try {
      const projection = await getActiveConfigManagerTask(scope)
      if (activeKeyRef.current !== activeChatKey) {
        await stream.cancel().catch(() => {})
        return
      }
      const taskID = projection.task_id?.trim() || ''
      if (taskID) attachedTaskIDRef.current = taskID
      setRuntimeProjection(projection)
      await consumeScopedStream(stream, activeChatKey, taskID)
    } catch (err) {
      if (activeKeyRef.current === activeChatKey) {
        // The accepted POST stream is already the exact display owner. Keep
        // consuming it when projection refresh fails instead of opening a
        // second subscription with component-local command state.
        setError(`${t('configManager.reconnectFailed')}: ${errorMessage(err)}`)
        await consumeScopedStream(stream, activeChatKey, '')
      } else {
        await stream.cancel().catch(() => {})
      }
    }
  }

  const abortRecovery = async () => {
    const action = runtimeProjectionRef.current?.recovery_actions?.find((candidate) => candidate.kind === 'abort')
    if (!action || abortPending) return
    setAbortPending(true)
    const activeChatKey = chatKey
    try {
      const receipt = await recoverConfigManagerRuntime(scope, action)
      if (!sameRecoveryAction(receipt.recovery_action, action)) throw new Error('Config Manager abort identity changed')
      const taskID = receipt.task_id?.trim() || ''
      setRuntimeProjection({
        ...(runtimeProjectionRef.current || { active: true }),
        active: true,
        status: 'running',
        task_id: taskID,
        recovery_paused: false,
        runtime_recoverable: false,
        stream_attached: true,
        recovery_actions: [],
      })
      if (taskID && attachedTaskIDRef.current !== taskID) await attachScopedTask(taskID, activeChatKey)
    } catch (err) {
      if (activeKeyRef.current === activeChatKey) {
        setError(`${t('configManager.abortFailed')}: ${errorMessage(err)}`)
      }
    } finally {
      if (activeKeyRef.current === activeChatKey) setAbortPending(false)
    }
  }

  const runtimeBusy = running || recoveryPending
  const canAbortRecovery = runtimeProjection?.recovery_actions?.some((action) => action.kind === 'abort') === true
  const resolveAsk = useCallback(async (view: AgentMessageView, action: { status: 'answered'; answers: AgentAskAnswer[] } | { status: 'cancelled' }) => {
    const askID = typeof view.data.id === 'string' ? view.data.id.trim() : ''
    if (!askID) throw new Error('Cannot resolve an Ask without its interaction ID')
    return action.status === 'answered'
      ? answerConfigManagerAsk(scope, askID, action.answers)
      : cancelConfigManagerAsk(scope, askID)
  }, [scope])

  return (
    <div className={`relative flex h-full min-h-0 flex-col overflow-hidden ${className}`}>
      {error && <div className="border-b border-[var(--nova-border)] px-3 py-2 text-xs text-red-400">{error}</div>}
      <MessageList
        messages={messages}
        isStreaming={running}
        activityContent=""
        scrollResetKey={chatKey}
        bottomPaddingClassName="pb-36"
        bottomPaddingPx={messageListBottomPadding}
        hasEarlierMessages={hasEarlierMessages}
        isLoadingEarlierMessages={historyLoading}
        onLoadEarlierMessages={loadEarlierMessages}
        collapseTraceGroups
        onResolveAsk={resolveAsk}
      />
      <InputArea
        onSend={(value) => void send(value)}
        onStop={canAbortRecovery ? () => void abortRecovery() : (running ? () => abortLocalStream() : undefined)}
        disabled={false}
        generationActive={runtimeBusy}
        abortPending={abortPending}
        sendBlocked={runtimeBusy}
        draftKey={chatKey}
        skills={skills}
        commandScope="all"
        builtinCommands={['/clear']}
        placeholder={runtimeBusy ? t('configManager.executing') : t('configManager.placeholder')}
        disabledPlaceholder={t('configManager.executing')}
        tokenUsageMessages={tokenUsageMessages}
        agentKey="config_manager"
        workspace={workspace}
        floating
        onHeightChange={setInputAreaHeight}
      />
    </div>
  )
}

function configManagerRecoveryActionsToSubmit(projection: ActiveChatTask): AgentRuntimeRecoveryAction[] {
  if (!projection.runtime_recoverable) return []
  if (projection.stream_attached && !projection.recovery_paused) return []
  const actions = projection.recovery_actions || []
  const attach = projection.stream_attached
    ? undefined
    : actions.find((action) => action.kind === 'start_turn')
  const stateChange = actions.find((action) => action.kind !== 'start_turn' && action.kind !== 'abort')
  return [attach, stateChange].filter((action): action is AgentRuntimeRecoveryAction => Boolean(action))
}

function projectConfigManagerRecovery(
  projection: ActiveChatTask,
  actions: AgentRuntimeRecoveryAction[],
  taskID: string,
): ActiveChatTask {
  if (!actions.length) return projection
  const executionResumed = actions.some((action) => action.kind !== 'start_turn')
  const abortActions = (projection.recovery_actions || []).filter((action) => action.kind === 'abort')
  return {
    ...projection,
    active: true,
    status: 'running',
    task_id: taskID,
    recovery_paused: !executionResumed,
    runtime_recoverable: !executionResumed,
    stream_attached: true,
    recovery_actions: executionResumed ? [] : abortActions,
  }
}

function sameRecoveryAction(left: AgentRuntimeRecoveryAction, right: AgentRuntimeRecoveryAction) {
  return left.kind === right.kind && left.command_id === right.command_id && left.operation_id === right.operation_id
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error || 'Unknown error')
}

function isAbortLike(error: unknown) {
  return error instanceof DOMException && error.name === 'AbortError'
}
