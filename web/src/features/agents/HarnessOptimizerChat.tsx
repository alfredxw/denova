import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { RotateCcw, Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { UIMessageChunk } from 'ai'
import { Button } from '@/components/ui/button'
import { InputArea } from '@/components/Chat/InputArea'
import { MessageList } from '@/components/Chat/MessageList'
import { useAgentUIMessageStream } from '@/hooks/useAgentUIMessageStream'
import {
  answerHarnessOptimizerAsk,
  cancelHarnessOptimizerAsk,
  clearHarnessOptimizer,
  createAgentCommandID,
  getActiveHarnessOptimizer,
  getHarnessOptimizerMessages,
  reconnectHarnessOptimizer,
  runHarnessOptimizer,
} from '@/lib/api'
import type { ActiveChatTask, AgentAskAnswer } from '@/lib/api'
import { agentViewAskID, selectAgentTokenUsageRecords, type AgentMessageView } from '@/lib/agent-message-view'
import { createAgentDataMessage, createAgentTextMessage } from '@/lib/agent-ui-message'
import { agentCommandRetryKey, isKnownAgentCommandOutcome, rememberAgentCommandID } from '@/lib/agent-command'
import { normalizeAgentUIMessages } from '@/lib/agent-ui'
import { resolveAgentAskAndRefresh } from '@/lib/agent-ask'

const CHAT_KEY = 'harness-optimizer:user'

export function HarnessOptimizerChat({
  evidence,
  evidenceControl,
  onSettled,
}: {
  evidence?: string[]
  evidenceControl?: ReactNode
  onSettled: () => void
}) {
  const { t } = useTranslation()
  const mountedRef = useRef(false)
  const attachedTaskRef = useRef('')
  const startCommandIDsRef = useRef(new Map<string, string>())
  const [error, setError] = useState<string | null>(null)
  const [connecting, setConnecting] = useState(true)
  const [active, setActive] = useState<ActiveChatTask | null>(null)
  const [historyBefore, setHistoryBefore] = useState('0')
  const [hasEarlierMessages, setHasEarlierMessages] = useState(false)
  const [historyLoading, setHistoryLoading] = useState(false)
  const [inputAreaHeight, setInputAreaHeight] = useState(0)
  const {
    messages,
    setMessages,
    isStreaming,
    consumeAgentUIStream,
  } = useAgentUIMessageStream()
  const tokenUsageMessages = useMemo(() => selectAgentTokenUsageRecords(messages), [messages])

  const loadMessages = useCallback(async () => {
    const page = await getHarnessOptimizerMessages()
    if (!mountedRef.current) return
    setMessages(page.messages)
    setHistoryBefore(page.page.next_before)
    setHasEarlierMessages(page.page.has_more)
  }, [setMessages])

  const loadEarlierMessages = useCallback(async () => {
    if (!hasEarlierMessages || historyLoading) return
    setHistoryLoading(true)
    try {
      const page = await getHarnessOptimizerMessages(historyBefore)
      if (!mountedRef.current) return
      setMessages((current) => normalizeAgentUIMessages([...page.messages, ...current]))
      setHistoryBefore(page.page.next_before)
      setHasEarlierMessages(page.page.has_more)
    } catch (cause) {
      if (mountedRef.current) setError(`${t('continualLearning.chat.historyFailed')}: ${errorMessage(cause)}`)
    } finally {
      if (mountedRef.current) setHistoryLoading(false)
    }
  }, [hasEarlierMessages, historyBefore, historyLoading, setMessages, t])

  const applyProjection = useCallback((projection: ActiveChatTask) => {
    setActive(projection)
    if (projection.pending_ask) {
      setMessages((current) => normalizeAgentUIMessages([
        ...current,
        createAgentDataMessage({ type: 'agent-ask', data: { ...projection.pending_ask } }),
      ]))
    }
  }, [setMessages])

  const finishStream = useCallback(async (taskID: string) => {
    if (attachedTaskRef.current === taskID) attachedTaskRef.current = ''
    if (!mountedRef.current) return
    try {
      await loadMessages()
      const projection = await getActiveHarnessOptimizer()
      if (!mountedRef.current) return
      applyProjection(projection)
      if (!projection.active) onSettled()
    } catch (cause) {
      if (mountedRef.current) setError(`${t('continualLearning.chat.refreshFailed')}: ${errorMessage(cause)}`)
    } finally {
      if (mountedRef.current) setConnecting(false)
    }
  }, [applyProjection, loadMessages, onSettled, t])

  const consume = useCallback(async (stream: ReadableStream<UIMessageChunk>, taskID: string) => {
    try {
      await consumeAgentUIStream(stream, { shouldContinue: () => mountedRef.current })
    } catch (cause) {
      if (mountedRef.current && !isAbortLike(cause)) {
        setError(`${t('continualLearning.chat.streamFailed')}: ${errorMessage(cause)}`)
      }
    } finally {
      await finishStream(taskID)
    }
  }, [consumeAgentUIStream, finishStream, t])

  const attach = useCallback(async (projection: ActiveChatTask) => {
    const taskID = projection.task_id?.trim() || ''
    if (!projection.active || !taskID || attachedTaskRef.current === taskID) return
    attachedTaskRef.current = taskID
    setConnecting(true)
    try {
      const stream = await reconnectHarnessOptimizer(taskID)
      if (!mountedRef.current) {
        await stream.cancel().catch(() => {})
        return
      }
      setError(null)
      void consume(stream, taskID)
    } catch (cause) {
      if (attachedTaskRef.current === taskID) attachedTaskRef.current = ''
      if (mountedRef.current) {
        setConnecting(false)
        setError(`${t('continualLearning.chat.reconnectFailed')}: ${errorMessage(cause)}`)
      }
    }
  }, [consume, t])

  const inspect = useCallback(async () => {
    try {
      const projection = await getActiveHarnessOptimizer()
      if (!mountedRef.current) return
      applyProjection(projection)
      if (projection.active) await attach(projection)
      else setConnecting(false)
    } catch (cause) {
      if (mountedRef.current) {
        setConnecting(false)
        setError(`${t('continualLearning.chat.reconnectFailed')}: ${errorMessage(cause)}`)
      }
    }
  }, [applyProjection, attach, t])

  useEffect(() => {
    mountedRef.current = true
    setConnecting(true)
    void loadMessages().then(inspect).catch((cause) => {
      if (mountedRef.current) {
        setConnecting(false)
        setError(`${t('continualLearning.chat.historyFailed')}: ${errorMessage(cause)}`)
      }
    })
    const retry = () => {
      void inspect()
    }
    window.addEventListener('online', retry)
    window.addEventListener('focus', retry)
    return () => {
      mountedRef.current = false
      attachedTaskRef.current = ''
      window.removeEventListener('online', retry)
      window.removeEventListener('focus', retry)
    }
  }, [inspect, loadMessages, t])

  const appendError = useCallback((content: string) => {
    setMessages((current) => [...current, createAgentDataMessage({ type: 'agent-error', data: { content } })])
  }, [setMessages])

  const start = useCallback(async (rawInstruction: string, showUserMessage: boolean) => {
    const instruction = rawInstruction.trim()
    if (isStreaming || connecting || active?.active) return
    if (showUserMessage && instruction === '/clear') {
      try {
        await clearHarnessOptimizer()
        setMessages([createAgentDataMessage({ type: 'agent-clear', data: { created_at: new Date().toISOString() } })])
        setHistoryBefore('0')
        setHasEarlierMessages(false)
      } catch (cause) {
        appendError(`${t('continualLearning.chat.clearFailed')}: ${errorMessage(cause)}`)
      }
      return
    }
    if (showUserMessage && !instruction) return
    if (showUserMessage) {
      setMessages((current) => [...current, createAgentTextMessage({ role: 'user', text: instruction })])
    }
    setError(null)
    setConnecting(true)
    const retryKey = agentCommandRetryKey(CHAT_KEY, 'start', { instruction, evidence })
    const commandID = rememberAgentCommandID(startCommandIDsRef.current, retryKey, createAgentCommandID)
    try {
      const stream = await runHarnessOptimizer(commandID, instruction, evidence)
      startCommandIDsRef.current.delete(retryKey)
      if (!mountedRef.current) {
        await stream.cancel().catch(() => {})
        return
      }
      const projection = await getActiveHarnessOptimizer()
      if (!mountedRef.current) {
        await stream.cancel().catch(() => {})
        return
      }
      applyProjection(projection)
      const taskID = projection.task_id?.trim() || ''
      if (taskID) attachedTaskRef.current = taskID
      await consume(stream, taskID)
    } catch (cause) {
      if (isKnownAgentCommandOutcome(cause)) startCommandIDsRef.current.delete(retryKey)
      if (mountedRef.current) {
        setConnecting(false)
        appendError(`${t('continualLearning.chat.runFailed')}: ${errorMessage(cause)}`)
        void inspect()
      }
    }
  }, [active?.active, appendError, applyProjection, connecting, consume, evidence, inspect, isStreaming, setMessages, t])

  const resolveAsk = useCallback(async (view: AgentMessageView, action: { status: 'answered'; answers: AgentAskAnswer[] } | { status: 'cancelled' }) => {
    const askID = agentViewAskID(view)
    if (!askID) throw new Error('Cannot resolve an Ask without its interaction ID')
    return resolveAgentAskAndRefresh(action, {
      answer: (answers) => answerHarnessOptimizerAsk(askID, answers),
      cancel: () => cancelHarnessOptimizerAsk(askID),
    }, loadMessages)
  }, [loadMessages])

  const busy = isStreaming || connecting || active?.active === true
  return (
    <div className="relative flex h-full min-h-0 flex-col overflow-hidden">
      <div className="flex min-h-12 flex-wrap items-center justify-between gap-2 border-b border-[var(--nova-border)] px-3 py-2">
        <div className="min-w-0">
          <div className="truncate text-xs font-medium text-[var(--nova-text)]">{t('continualLearning.optimizer.title')}</div>
          <div className="truncate text-[10px] text-[var(--nova-text-faint)]">
            {evidence?.length
              ? t('continualLearning.optimizer.evidenceCount', { count: evidence.length })
              : t('continualLearning.optimizer.evidenceAuto')}
          </div>
        </div>
        <div className="ml-auto flex min-w-0 items-center gap-1.5">
          {evidenceControl}
          <Button type="button" size="xs" variant="outline" disabled={busy} onClick={() => void start('', false)}>
            {busy ? <RotateCcw className="animate-spin" /> : <Sparkles />}
            {t('continualLearning.optimizeNow')}
          </Button>
        </div>
      </div>
      {error && <div className="border-b border-[var(--nova-border)] px-3 py-2 text-xs text-red-400">{error}</div>}
      <MessageList
        projectId=""
        messages={messages}
        isStreaming={isStreaming}
        activityContent=""
        scrollResetKey={CHAT_KEY}
        bottomPaddingClassName="pb-36"
        bottomPaddingPx={inputAreaHeight > 0 ? inputAreaHeight + 20 : undefined}
        hasEarlierMessages={hasEarlierMessages}
        isLoadingEarlierMessages={historyLoading}
        onLoadEarlierMessages={loadEarlierMessages}
        collapseTraceGroups
        onResolveAsk={resolveAsk}
      />
      <InputArea
        onSend={(value) => start(value, true)}
        disabled={false}
        generationActive={busy}
        sendBlocked={busy}
        draftKey={CHAT_KEY}
        commandScope="all"
        builtinCommands={['/clear']}
        placeholder={busy ? t('continualLearning.optimizer.running') : t('continualLearning.optimizer.placeholder')}
        disabledPlaceholder={t('continualLearning.optimizer.running')}
        tokenUsageMessages={tokenUsageMessages}
        floating
        onHeightChange={setInputAreaHeight}
      />
    </div>
  )
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error || 'Unknown error')
}

function isAbortLike(error: unknown) {
  return error instanceof DOMException && error.name === 'AbortError'
}
