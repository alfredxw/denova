import { useCallback, useEffect, useMemo, useRef } from 'react'
import { buildContextCompactionMessage, createContextCompactionMessageId, upsertContextCompactionMessage } from '@/components/Chat/context-compaction-message'
import type { AgentMessageMetadata, AgentUIMessage } from '@/lib/agent-ui'
import { createAgentDataMessage, createAgentTextMessage, createAgentToolMessage, parseAgentToolInput } from '@/lib/agent-ui-message'
import { createRafUpdateBatcher, STREAMING_RENDER_INTERVAL_MS } from '@/lib/streaming/raf-update-batcher'
import {
  appendBufferedLiveMessage,
  bindLiveToolEventKeys,
  completeToolMessage,
  findMappedLiveToolId,
  findToolMessageIndexForPayload,
  liveToolEventKeys,
  promoteMessageTarget,
  promoteMessageTargets,
  streamMetadataFromPayload,
  toolMessageInput,
  updateToolMessageInput,
  type BufferedLiveMessage,
} from './live-stream-messages'
import { publicRuleRollFromToolOutput } from './rule-roll'
import { createLiveTurnRenderKeys, type LiveTurnRenderKeys } from './utils'

type LiveMessageUpdater = (updater: AgentUIMessage[] | ((current: AgentUIMessage[]) => AgentUIMessage[])) => void

interface UseLiveMessageAccumulatorOptions {
  publicRuleRollVisible: boolean
  setMessages: LiveMessageUpdater
}

// Owns the animation-frame buffer and correlation state for one live story stream.
// Persisted history projection only receives stable render-key lookup methods, so
// display recovery state cannot leak into the model/session context boundary.
export function useLiveMessageAccumulator({ publicRuleRollVisible, setMessages }: UseLiveMessageAccumulatorOptions) {
  const toolKeyToMessageIdRef = useRef<Record<string, string>>({})
  const nonNarrativeStreamingRef = useRef(false)
  const stageKeyRef = useRef('')
  const currentTurnRenderKeysRef = useRef<LiveTurnRenderKeys | null>(null)
  const turnRenderKeysRef = useRef<Record<string, LiveTurnRenderKeys>>({})
  const currentCompactionMessageIdRef = useRef<string | null>(null)
  const compactionIdCounterRef = useRef(0)

  // Text, thinking and tool-argument deltas share one ordered commit lane. The
  // staged target is promoted once per batch so the presentation layer can
  // identify prose immediately without adding another render per provider delta.
  const updateBatcher = useMemo(
    () => createRafUpdateBatcher<AgentUIMessage[]>(
      (update) => setMessages((current) => promoteMessageTargets(update(current))),
      { minIntervalMs: STREAMING_RENDER_INTERVAL_MS },
    ),
    [setMessages],
  )

  const flushMessageBuffer = useCallback(() => {
    updateBatcher.flush()
  }, [updateBatcher])

  const flush = useCallback(() => {
    flushMessageBuffer()
  }, [flushMessageBuffer])

  const queue = useCallback((message: BufferedLiveMessage) => {
    updateBatcher.enqueue((current) => appendBufferedLiveMessage(current, message))
  }, [updateBatcher])

  const appendAssistant = useCallback((content: string, navigationAnchorId: string, metadata: AgentMessageMetadata = {}) => {
    if (!content) return
    const renderKey = metadata.subagent ? undefined : currentTurnRenderKeysRef.current?.assistant
    const navigationMetadata = metadata.subagent ? metadata : { ...metadata, navigation_turn_id: navigationAnchorId }
    queue({
      id: renderKey || metadata.display_segment_id,
      role: 'assistant',
      content,
      metadata: navigationMetadata,
    })
  }, [queue])

  const resetAssistant = useCallback(() => {
    flush()
    setMessages((current) => {
      const last = current[current.length - 1]
      return last?.parts.some((part) => part.type === 'text' && 'state' in part && part.state === 'streaming')
        ? current.slice(0, -1)
        : current
    })
  }, [flush, setMessages])

  const appendThinking = useCallback((content: string, metadata: AgentMessageMetadata = {}) => {
    if (!content) return
    nonNarrativeStreamingRef.current = true
    queue({ id: metadata.display_segment_id, role: 'reasoning', content, metadata })
  }, [queue])

  const appendToolCall = useCallback((payload: Record<string, unknown> & { id?: string; name?: string; args?: string }) => {
    flush()
    const toolKeys = liveToolEventKeys(payload)
    const mappedId = findMappedLiveToolId(toolKeys, toolKeyToMessageIdRef.current)
    const id = payload.id || mappedId || `tool-${Date.now()}-${Math.random().toString(16).slice(2)}`
    const name = payload.name || 'unknown_tool'
    const input = payload.args ?? ''
    if (toolKeys.length > 0) {
      toolKeyToMessageIdRef.current = bindLiveToolEventKeys(toolKeys, toolKeyToMessageIdRef.current, id)
    }
    nonNarrativeStreamingRef.current = true
    setMessages((current) => [
      ...current,
      createAgentToolMessage({
        id,
        name,
        state: 'input-streaming',
        input,
        metadata: streamMetadataFromPayload(payload),
      }),
    ])
  }, [flush, setMessages])

  const appendToolArgs = useCallback((payload: Record<string, unknown> & { id?: string; name?: string; args?: string; delta?: string }) => {
    if (!payload.id && !payload.name && liveToolEventKeys(payload).length === 0) return
    const inputDelta = payload.args !== undefined ? payload.args : (payload.delta || '')
    const replaceInput = payload.args !== undefined
    updateBatcher.enqueue((current) => {
      const targetIndex = findToolMessageIndexForPayload(current, payload, toolKeyToMessageIdRef.current)
      if (targetIndex < 0) return current
      const matchedId = current[targetIndex].id
      if (matchedId) {
        toolKeyToMessageIdRef.current = bindLiveToolEventKeys(liveToolEventKeys(payload), toolKeyToMessageIdRef.current, matchedId)
      }
      return current.map((message, index) => {
        if (index !== targetIndex) return message
        const input = replaceInput ? inputDelta : `${toolMessageInput(message)}${inputDelta}`
        return updateToolMessageInput(message, input)
      })
    })
  }, [updateBatcher])

  const completeToolCall = useCallback((payload: Record<string, unknown> & { id?: string; name?: string }, result = '') => {
    updateBatcher.flush()
    setMessages((current) => {
      const targetIndex = findToolMessageIndexForPayload(current, payload, toolKeyToMessageIdRef.current)
      if (targetIndex < 0) return current
      const matchedId = current[targetIndex].id
      if (matchedId) {
        toolKeyToMessageIdRef.current = bindLiveToolEventKeys(liveToolEventKeys(payload), toolKeyToMessageIdRef.current, matchedId)
      }
      return current.map((message, index) => index === targetIndex ? completeToolMessage(message, result) : message)
    })
  }, [setMessages, updateBatcher])

  const appendRuleRoll = useCallback((payload: Record<string, unknown> & { name?: string; content?: string }) => {
    if (!publicRuleRollVisible || payload.name !== 'prepare_interactive_turn') return
    flush()
    const ruleRoll = publicRuleRollFromToolOutput(payload.content || '')
    if (!ruleRoll) return
    setMessages((current) => {
      const id = ruleRoll.resolution_id ? `live-rule-roll-${ruleRoll.resolution_id}` : `live-rule-roll-${Date.now()}`
      if (current.some((message) => message.id === id)) return current
      return [...current, createAgentDataMessage({
        id,
        type: 'agent-rule-roll',
        data: { id, role: 'rule_roll', rule_roll: ruleRoll },
      })]
    })
  }, [flush, publicRuleRollVisible, setMessages])

  const appendContextCompaction = useCallback((data: Record<string, unknown>) => {
    flush()
    const id = currentCompactionMessageIdRef.current || createContextCompactionMessageId(compactionIdCounterRef)
    currentCompactionMessageIdRef.current = id
    nonNarrativeStreamingRef.current = true
    setMessages((current) => upsertContextCompactionMessage(current, buildContextCompactionMessage(data, id)))
  }, [flush, setMessages])

  const collapseNonNarrative = useCallback(() => {
    if (!nonNarrativeStreamingRef.current) return
    nonNarrativeStreamingRef.current = false
    updateBatcher.enqueue((current) => current.map((message) => settleAgentMessage(promoteMessageTarget(message), false)))
  }, [updateBatcher])

  const finishMessages = useCallback((toolStatus: 'success' | 'cancelled' = 'success') => {
    flush()
    nonNarrativeStreamingRef.current = false
    setMessages((current) => current.map((message) => settleAgentMessage(promoteMessageTarget(message), true, toolStatus)))
  }, [flush, setMessages])

  const resetForCheckpoint = useCallback(() => {
    updateBatcher.discard()
    toolKeyToMessageIdRef.current = {}
    nonNarrativeStreamingRef.current = false
    currentTurnRenderKeysRef.current = null
    currentCompactionMessageIdRef.current = null
    setMessages([])
  }, [setMessages, updateBatcher])

  const prepareTurn = useCallback((message: string, navigationAnchorId: string, mode: 'replace' | 'append', attachments: import('@/lib/chat-attachments').ChatAttachmentDescriptor[] = []) => {
    flush()
    toolKeyToMessageIdRef.current = {}
    nonNarrativeStreamingRef.current = false
    const renderKeys = createLiveTurnRenderKeys()
    currentTurnRenderKeysRef.current = renderKeys
    currentCompactionMessageIdRef.current = null
    const userMessage = createAgentTextMessage({
      id: renderKeys.user,
      role: 'user',
      text: message,
      metadata: { display_role: 'user', created_at: new Date().toISOString(), navigation_turn_id: navigationAnchorId, ...(attachments.length ? { attachments } : {}) },
    })
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
    updateBatcher.discard()
  }, [updateBatcher])

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
    prepareTurn,
    renderKeyFor,
    resetAssistant,
    resetForCheckpoint,
    resetCompaction,
  }
}

export type LiveMessageAccumulator = ReturnType<typeof useLiveMessageAccumulator>

function settleAgentMessage(message: AgentUIMessage, includeNarrative: boolean, toolStatus: 'success' | 'cancelled' = 'success'): AgentUIMessage {
  let changed = false
  const parts = message.parts.map((part) => {
    if ((part.type === 'text' && includeNarrative) || part.type === 'reasoning') {
      if (!('state' in part) || part.state !== 'streaming') return part
      changed = true
      return { ...part, state: 'done' as const }
    }
    if (part.type === 'dynamic-tool' && (part.state === 'input-streaming' || part.state === 'input-available')) {
      changed = true
      const currentToolMetadata = 'toolMetadata' in part && part.toolMetadata && typeof part.toolMetadata === 'object'
        ? part.toolMetadata
        : {}
      return {
        ...part,
        input: typeof part.input === 'string' ? parseAgentToolInput(part.input) : part.input,
        state: 'output-available' as const,
        ...(toolStatus === 'cancelled' ? { toolMetadata: { ...currentToolMetadata, status: 'cancelled' } } : {}),
      }
    }
    if (part.type === 'data-agent-context-compaction' && part.data.status === 'running') {
      changed = true
      return { ...part, data: { ...part.data, status: 'success' } }
    }
    return part
  })
  return changed ? { ...message, parts } as AgentUIMessage : message
}
