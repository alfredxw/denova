import { useCallback, useEffect, useMemo, useRef } from 'react'
import { buildContextCompactionMessage, createContextCompactionMessageId, upsertContextCompactionMessage } from '@/components/Chat/context-compaction-message'
import type { ChatMessage } from '@/lib/api'
import { createRafUpdateBatcher } from '@/lib/streaming/raf-update-batcher'
import {
  appendBufferedLiveMessage,
  bindLiveToolEventKeys,
  findMappedLiveToolId,
  findToolMessageIndexForPayload,
  liveToolEventKeys,
  promoteMessageTarget,
  promoteMessageTargets,
  streamMetadataFromPayload,
  type BufferedLiveMessage,
} from './live-stream-messages'
import { publicRuleRollFromToolOutput } from './rule-roll'
import { createLiveTurnRenderKeys, type LiveTurnRenderKeys } from './utils'

type LiveMessageUpdater = (updater: ChatMessage[] | ((current: ChatMessage[]) => ChatMessage[])) => void

interface UseLiveMessageAccumulatorOptions {
  publicRuleRollVisible: boolean
  setMessages: LiveMessageUpdater
}

// Owns the animation-frame buffer and correlation state for one live story stream.
// Persisted history projection only receives stable render-key lookup methods, so
// display recovery state cannot leak into the model/session context boundary.
export function useLiveMessageAccumulator({ publicRuleRollVisible, setMessages }: UseLiveMessageAccumulatorOptions) {
  const messageBufferRef = useRef<BufferedLiveMessage[]>([])
  const messageRafRef = useRef<number | null>(null)
  const promoteRafRef = useRef<number | null>(null)
  const toolKeyToMessageIdRef = useRef<Record<string, string>>({})
  const nonNarrativeStreamingRef = useRef(false)
  const stageKeyRef = useRef('')
  const currentTurnRenderKeysRef = useRef<LiveTurnRenderKeys | null>(null)
  const turnRenderKeysRef = useRef<Record<string, LiveTurnRenderKeys>>({})
  const currentCompactionMessageIdRef = useRef<string | null>(null)
  const compactionIdCounterRef = useRef(0)

  const toolArgsBatcher = useMemo(
    () => createRafUpdateBatcher<ChatMessage[]>(setMessages),
    [setMessages],
  )

  const promoteTargets = useCallback(() => {
    setMessages((current) => promoteMessageTargets(current))
  }, [setMessages])

  const schedulePromotion = useCallback(() => {
    if (promoteRafRef.current !== null) return
    promoteRafRef.current = window.requestAnimationFrame(() => {
      promoteRafRef.current = null
      promoteTargets()
    })
  }, [promoteTargets])

  const flushMessageBuffer = useCallback(() => {
    if (messageRafRef.current !== null) {
      window.cancelAnimationFrame(messageRafRef.current)
      messageRafRef.current = null
    }
    const buffered = messageBufferRef.current
    if (buffered.length === 0) return
    messageBufferRef.current = []
    setMessages((current) => buffered.reduce(appendBufferedLiveMessage, current))
    if (buffered.some((message) => message.role === 'assistant' || message.role === 'thinking')) {
      schedulePromotion()
    }
  }, [schedulePromotion, setMessages])

  const flush = useCallback(() => {
    flushMessageBuffer()
    toolArgsBatcher.flush()
  }, [flushMessageBuffer, toolArgsBatcher])

  const queue = useCallback((message: BufferedLiveMessage) => {
    messageBufferRef.current.push(message)
    if (messageRafRef.current !== null) return
    messageRafRef.current = window.requestAnimationFrame(() => {
      messageRafRef.current = null
      flushMessageBuffer()
    })
  }, [flushMessageBuffer])

  const appendAssistant = useCallback((content: string, navigationAnchorId: string, metadata: Partial<ChatMessage> = {}) => {
    if (!content) return
    const renderKey = metadata.render_key || (metadata.subagent ? undefined : currentTurnRenderKeysRef.current?.assistant)
    const navigationMetadata = metadata.subagent ? metadata : { ...metadata, navigation_turn_id: navigationAnchorId }
    queue({
      role: 'assistant',
      content,
      metadata: renderKey ? { ...navigationMetadata, render_key: renderKey } : navigationMetadata,
    })
  }, [queue])

  const resetAssistant = useCallback(() => {
    flush()
    setMessages((current) => {
      const last = current[current.length - 1]
      return last?.role === 'assistant' && last.streaming ? current.slice(0, -1) : current
    })
  }, [flush, setMessages])

  const appendThinking = useCallback((content: string, metadata: Partial<ChatMessage> = {}) => {
    if (!content) return
    nonNarrativeStreamingRef.current = true
    queue({ role: 'thinking', content, metadata })
  }, [queue])

  const appendToolCall = useCallback((payload: Record<string, unknown> & { id?: string; name?: string; args?: string }) => {
    const toolKeys = liveToolEventKeys(payload)
    const mappedId = findMappedLiveToolId(toolKeys, toolKeyToMessageIdRef.current)
    const id = payload.id || mappedId || `tool-${Date.now()}-${Math.random().toString(16).slice(2)}`
    const name = payload.name || 'unknown_tool'
    if (toolKeys.length > 0) {
      toolKeyToMessageIdRef.current = bindLiveToolEventKeys(toolKeys, toolKeyToMessageIdRef.current, id)
    }
    nonNarrativeStreamingRef.current = true
    setMessages((current) => [
      ...current,
      {
        id,
        role: 'tool_call',
        content: name,
        name,
        args: payload.args || '',
        status: 'running',
        streaming: true,
        ...streamMetadataFromPayload(payload),
      },
    ])
  }, [setMessages])

  const appendToolArgs = useCallback((payload: Record<string, unknown> & { id?: string; name?: string; args?: string; delta?: string }) => {
    if (!payload.id && !payload.name && liveToolEventKeys(payload).length === 0) return
    toolArgsBatcher.enqueue((current) => {
      const targetIndex = findToolMessageIndexForPayload(current, payload, toolKeyToMessageIdRef.current)
      if (targetIndex < 0) return current
      const matchedId = current[targetIndex].id
      if (matchedId) {
        toolKeyToMessageIdRef.current = bindLiveToolEventKeys(liveToolEventKeys(payload), toolKeyToMessageIdRef.current, matchedId)
      }
      return current.map((message, index) => index === targetIndex
        ? { ...message, args: payload.args !== undefined ? payload.args : `${message.args || ''}${payload.delta || ''}` }
        : message)
    })
  }, [toolArgsBatcher])

  const completeToolCall = useCallback((payload: Record<string, unknown> & { id?: string; name?: string }, result = '') => {
    setMessages((current) => {
      const targetIndex = findToolMessageIndexForPayload(current, payload, toolKeyToMessageIdRef.current)
      if (targetIndex < 0) return current
      const matchedId = current[targetIndex].id
      if (matchedId) {
        toolKeyToMessageIdRef.current = bindLiveToolEventKeys(liveToolEventKeys(payload), toolKeyToMessageIdRef.current, matchedId)
      }
      return current.map((message, index) => index === targetIndex
        ? { ...message, status: 'success' as const, result, streaming: false }
        : message)
    })
  }, [setMessages])

  const appendRuleRoll = useCallback((payload: Record<string, unknown> & { name?: string; content?: string }) => {
    if (!publicRuleRollVisible || payload.name !== 'prepare_interactive_turn') return
    const ruleRoll = publicRuleRollFromToolOutput(payload.content || '')
    if (!ruleRoll) return
    setMessages((current) => {
      const id = ruleRoll.resolution_id ? `live-rule-roll-${ruleRoll.resolution_id}` : `live-rule-roll-${Date.now()}`
      if (current.some((message) => message.role === 'rule_roll' && message.id === id)) return current
      return [...current, { id, role: 'rule_roll', rule_roll: ruleRoll, streaming: false }]
    })
  }, [publicRuleRollVisible, setMessages])

  const appendContextCompaction = useCallback((data: Record<string, unknown>) => {
    const id = currentCompactionMessageIdRef.current || createContextCompactionMessageId(compactionIdCounterRef)
    currentCompactionMessageIdRef.current = id
    nonNarrativeStreamingRef.current = true
    setMessages((current) => upsertContextCompactionMessage(current, buildContextCompactionMessage(data, id)))
  }, [setMessages])

  const collapseNonNarrative = useCallback(() => {
    if (!nonNarrativeStreamingRef.current) return
    flush()
    nonNarrativeStreamingRef.current = false
    setMessages((current) => current.map((message) => message.role === 'tool_call' || message.role === 'context_compaction'
      ? { ...message, status: message.status === 'running' ? 'success' : message.status }
      : message))
  }, [flush, setMessages])

  const finishMessages = useCallback(() => {
    flush()
    if (promoteRafRef.current !== null) {
      window.cancelAnimationFrame(promoteRafRef.current)
      promoteRafRef.current = null
    }
    nonNarrativeStreamingRef.current = false
    setMessages((current) => current.map((message) =>
      message.role === 'assistant' || message.role === 'thinking' || message.role === 'tool_call' || message.role === 'context_compaction'
        ? {
            ...promoteMessageTarget(message),
            streaming: false,
            status: message.role === 'tool_call' || message.role === 'context_compaction'
              ? (message.status === 'running' ? 'success' : message.status)
              : message.status,
          }
        : message))
  }, [flush, setMessages])

  const resetForCheckpoint = useCallback(() => {
    if (messageRafRef.current !== null) {
      window.cancelAnimationFrame(messageRafRef.current)
      messageRafRef.current = null
    }
    if (promoteRafRef.current !== null) {
      window.cancelAnimationFrame(promoteRafRef.current)
      promoteRafRef.current = null
    }
    messageBufferRef.current = []
    toolArgsBatcher.discard()
    toolKeyToMessageIdRef.current = {}
    nonNarrativeStreamingRef.current = false
    currentTurnRenderKeysRef.current = null
    currentCompactionMessageIdRef.current = null
    setMessages([])
  }, [setMessages, toolArgsBatcher])

  const prepareTurn = useCallback((message: string, navigationAnchorId: string, mode: 'replace' | 'append') => {
    flush()
    toolKeyToMessageIdRef.current = {}
    nonNarrativeStreamingRef.current = false
    const renderKeys = createLiveTurnRenderKeys()
    currentTurnRenderKeysRef.current = renderKeys
    currentCompactionMessageIdRef.current = null
    const userMessage: ChatMessage = {
      role: 'user',
      content: message,
      render_key: renderKeys.user,
      navigation_turn_id: navigationAnchorId,
    }
    setMessages((current) => mode === 'replace' ? [userMessage] : [...current, userMessage])
  }, [flush, setMessages])

  const bindPersistedTurn = useCallback((turnId: string) => {
    if (turnId && currentTurnRenderKeysRef.current) {
      turnRenderKeysRef.current[turnId] = currentTurnRenderKeysRef.current
    }
  }, [])

  const renderKeyFor = useCallback((turnId: string, role: 'user' | 'assistant') => {
    return turnRenderKeysRef.current[turnId]?.[role]
  }, [])

  const beginStage = useCallback((stageKey: string) => {
    stageKeyRef.current = stageKey
  }, [])
  const belongsToStage = useCallback((stageKey: string) => stageKeyRef.current === stageKey, [])
  const resetCompaction = useCallback(() => {
    currentCompactionMessageIdRef.current = null
  }, [])
  const endRun = useCallback(() => {
    toolKeyToMessageIdRef.current = {}
    currentCompactionMessageIdRef.current = null
    currentTurnRenderKeysRef.current = null
  }, [])

  useEffect(() => () => {
    if (messageRafRef.current !== null) window.cancelAnimationFrame(messageRafRef.current)
    if (promoteRafRef.current !== null) window.cancelAnimationFrame(promoteRafRef.current)
    messageBufferRef.current = []
    toolArgsBatcher.discard()
  }, [toolArgsBatcher])

  return {
    appendAssistant,
    appendContextCompaction,
    appendRuleRoll,
    appendThinking,
    appendToolArgs,
    appendToolCall,
    beginStage,
    bindPersistedTurn,
    belongsToStage,
    collapseNonNarrative,
    completeToolCall,
    endRun,
    finishMessages,
    flush,
    flushMessages: flushMessageBuffer,
    prepareTurn,
    renderKeyFor,
    resetAssistant,
    resetForCheckpoint,
    resetCompaction,
  }
}

export type LiveMessageAccumulator = ReturnType<typeof useLiveMessageAccumulator>
