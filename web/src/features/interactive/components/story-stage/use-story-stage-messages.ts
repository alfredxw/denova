import { useMemo } from 'react'
import { selectAgentTokenUsageRecords } from '@/lib/agent-message-view'
import { normalizeAgentUIMessages, type AgentMessageMetadata, type AgentUIMessage } from '@/lib/agent-ui'
import {
  agentMessageDisplayText,
  agentMessageHasDataPart,
  createAgentDataMessage,
  createAgentReasoningMessage,
  createAgentTextMessage,
  createAgentToolMessage,
} from '@/lib/agent-ui-message'
import type { Snapshot, TurnDisplayEvent, TurnEvent } from '../../types'
import type { TurnNavigationItem } from '../TurnNavigator'
import { sanitizeStoredNarrative } from '../../stream-parser'
import {
  latestMergedInteractiveImage,
  mergeInteractiveImages,
  readInteractiveImage,
  readInteractiveImageError,
} from './interactive-images'
import { buildTokenUsageMessage, mergeTokenUsageMessages } from './token-usage'
import { normalizeMessageContent } from './utils'

interface UseStoryStageMessagesOptions {
  pendingAsk?: import('@/lib/api').AgentAskInteraction
  snapshot: Snapshot | null
  rewindTurnId?: string
  liveMessages: AgentUIMessage[]
  streaming: boolean
  stageKey: string
  liveTurnNavigationAnchorId: string
  optimisticInteractiveImages: Record<string, import('@/lib/api').InteractiveImage[]>
  belongsToStage: (stageKey: string) => boolean
  renderKeyFor: (turnId: string, role: 'user' | 'assistant') => string | undefined
}

// Projects persisted domain turns and ephemeral display events directly into
// the shared UI message protocol. This remains a UI-only read model and never
// participates in model-context assembly.
export function useStoryStageMessages({
  pendingAsk,
  snapshot,
  rewindTurnId,
  liveMessages,
  streaming,
  stageKey,
  liveTurnNavigationAnchorId,
  optimisticInteractiveImages,
  belongsToStage,
  renderKeyFor,
}: UseStoryStageMessagesOptions) {
  const storyPathTurns = useMemo(() => {
    const turns = snapshot?.turns || []
    const rewindIndex = rewindTurnId ? turns.findIndex((turn) => turn.id === rewindTurnId) : -1
    return rewindIndex >= 0 ? turns.slice(0, rewindIndex) : turns
  }, [rewindTurnId, snapshot?.turns])

  const latestLiveTurn = useMemo(() => {
    if (liveMessages.length === 0) return null
    const user = liveMessages.find((message) => message.role === 'user')
    const narrative = liveMessages
      .filter((message) => message.role === 'assistant' && !message.metadata?.subagent && message.parts.some((part) => part.type === 'text'))
      .map(agentMessageDisplayText)
      .join('')
    const userText = user ? agentMessageDisplayText(user) : ''
    return userText || narrative ? { user: userText, narrative } : null
  }, [liveMessages])

  const hasPersistedLiveTurn = useMemo(() => {
    const lastTurn = snapshot?.turns?.[snapshot.turns.length - 1]
    if (!lastTurn || !latestLiveTurn || !belongsToStage(stageKey)) return false
    return normalizeMessageContent(lastTurn.user) === normalizeMessageContent(latestLiveTurn.user)
      && normalizeMessageContent(lastTurn.narrative) === normalizeMessageContent(latestLiveTurn.narrative)
  }, [belongsToStage, latestLiveTurn, snapshot?.turns, stageKey])

  const historyMessages = useMemo(
    () => storyPathTurns.flatMap((turn) => projectPersistedTurn(turn, {
      optimisticImages: optimisticInteractiveImages[turn.id],
      renderKeyFor,
    })),
    [optimisticInteractiveImages, renderKeyFor, storyPathTurns],
  )

  const agentMessages = useMemo(
    () => normalizeAgentUIMessages([
      ...historyMessages,
      ...(snapshot?.pending_display_events || []).filter(event => !liveMessages.some(message => message.id === event.id)).map((event, index) => createAgentDataMessage({
        id: event.id || `pending-${event.role}-${index}`,
        type: event.role === 'todo_updated' ? 'agent-todo' : 'agent-context-compaction',
        data: event.role === 'todo_updated' ? JSON.parse(event.content || '{}') : { ...event, status: event.status || 'success' },
        metadata: displayEventMetadata(event),
      })),
      ...(hasPersistedLiveTurn ? [] : liveMessages.filter((message) => !agentMessageHasDataPart(message, 'agent-token-usage'))),
      ...(pendingAsk ? [createAgentDataMessage({ id: `ask-${pendingAsk.id}`, type: 'agent-ask', data: { ...pendingAsk } })] : []),
    ]),
    [hasPersistedLiveTurn, historyMessages, liveMessages, pendingAsk, snapshot?.pending_display_events],
  )
  const turnNavigationItems = useMemo<TurnNavigationItem[]>(() => {
    const items: TurnNavigationItem[] = storyPathTurns.map((turn) => ({
      anchorId: turn.id,
      user: turn.user,
      narrative: sanitizeStoredNarrative(turn.narrative),
      contextOnly: turn.user_context_only,
    }))
    if (!hasPersistedLiveTurn && latestLiveTurn) {
      items.push({
        anchorId: liveTurnNavigationAnchorId,
        user: latestLiveTurn.user,
        narrative: latestLiveTurn.narrative,
        pending: streaming || !latestLiveTurn.narrative.trim(),
      })
    }
    return items
  }, [hasPersistedLiveTurn, latestLiveTurn, liveTurnNavigationAnchorId, storyPathTurns, streaming])
  const persistedTokenUsage = useMemo(
    () => (snapshot?.token_usage_events || []).map((event, index) => buildTokenUsageMessage(event, event.id || `token-usage-${index + 1}`)),
    [snapshot?.token_usage_events],
  )
  const liveTokenUsage = useMemo(
    () => liveMessages.filter((message) => agentMessageHasDataPart(message, 'agent-token-usage')),
    [liveMessages],
  )
  const tokenUsageMessages = useMemo(
    () => selectAgentTokenUsageRecords(mergeTokenUsageMessages(persistedTokenUsage, liveTokenUsage)),
    [liveTokenUsage, persistedTokenUsage],
  )
  const turnsById = useMemo(() => new Map<string, TurnEvent>((snapshot?.turns || []).map((turn) => [turn.id, turn])), [snapshot?.turns])

  return { agentMessages, tokenUsageMessages, turnNavigationItems, turnsById }
}

function projectPersistedTurn(turn: TurnEvent, options: {
  optimisticImages?: import('@/lib/api').InteractiveImage[]
  renderKeyFor: (turnId: string, role: 'user' | 'assistant') => string | undefined
}) {
  const messages: AgentUIMessage[] = turn.user_context_only ? [] : [createAgentTextMessage({
    id: options.renderKeyFor(turn.id, 'user') || `${turn.id}-user`,
    role: 'user',
    text: turn.user,
    metadata: {
      display_role: 'user',
      created_at: turn.ts,
      turn_id: turn.id,
      navigation_turn_id: turn.id,
      ...(turn.attachments?.length ? { attachments: turn.attachments } : {}),
    },
  })]
  const displayEvents = (turn.display_events || []).filter(event => event.status !== 'discarded')
  // A confirmed result may be a separate journal event. Fold it into its call
  // without rewriting history or leaving a completed tool permanently running.
  const toolResults = new Map(displayEvents.filter(event => event.role === 'tool_result' && event.id).map(event => [event.id, event]))
  if (!displayEvents.some((event) => event.role === 'thinking') && turn.thinking?.trim()) {
    messages.push(createAgentReasoningMessage({
      id: `${turn.id}-thinking`,
      text: turn.thinking,
      metadata: { display_role: 'thinking' },
    }))
  }
  const deferredImageEvents: TurnDisplayEvent[] = []
  const beforeNarrative: AgentUIMessage[] = []
  const afterNarrative: AgentUIMessage[] = []
  let narrativeAnchored = false
  for (const [index, event] of displayEvents.entries()) {
    if (event.role === 'narrative') {
      narrativeAnchored = true
      continue
    }
    const timeline = narrativeAnchored ? afterNarrative : beforeNarrative
    const metadata = displayEventMetadata(event)
    switch (event.role) {
      case 'todo_updated':
        timeline.push(createAgentDataMessage({
          id: event.id || `${turn.id}-todo-${index}`,
          type: 'agent-todo',
          data: JSON.parse(event.content || '{}'),
          metadata,
        }))
        break
      case 'context_compaction':
        timeline.push(createAgentDataMessage({
          id: event.id || `${turn.id}-compaction-${index}`,
          type: 'agent-context-compaction',
          data: { ...event, status: event.status || 'success' },
          metadata,
        }))
        break
      case 'thinking':
        timeline.push(createAgentReasoningMessage({
          id: event.id || `${turn.id}-thinking-${index}`,
          text: event.content || '',
          metadata: { ...metadata, display_role: 'thinking' },
        }))
        break
      case 'tool_call': {
        const result = toolResults.get(event.id)
        const status = result?.status || event.status || 'success'
        const output = result?.result ?? result?.content ?? event.result
        if (event.tool_presentation?.result === 'interactive_media') {
          deferredImageEvents.push({ ...event, status, result: output })
          break
        }
        timeline.push(createAgentToolMessage({
          id: event.id || `${turn.id}-tool-${index}`,
          name: event.name || event.content || 'unknown_tool',
          state: status === 'error' ? 'output-error' : status === 'success' ? 'output-available' : 'input-available',
          input: event.args || '',
          output: status === 'error' ? undefined : output || undefined,
          errorText: status === 'error' ? output || '' : undefined,
          metadata: { ...metadata, display_role: 'tool_call' },
        }))
        break
      }
      case 'assistant':
        timeline.push(createAgentTextMessage({
          id: event.id || `${turn.id}-subagent-${index}`,
          role: 'assistant',
          text: event.content || '',
          metadata: { ...metadata, display_role: 'assistant' },
        }))
        break
      default:
        break
    }
  }
  messages.push(...beforeNarrative)
  messages.push(projectNarrativeMessage(turn, deferredImageEvents, options))
  messages.push(...afterNarrative)
  return messages
}

function projectNarrativeMessage(
  turn: TurnEvent,
  deferredImageEvents: TurnDisplayEvent[],
  options: {
    optimisticImages?: import('@/lib/api').InteractiveImage[]
    renderKeyFor: (turnId: string, role: 'user' | 'assistant') => string | undefined
  },
) {
  const id = options.renderKeyFor(turn.id, 'assistant') || `${turn.id}-assistant`
  const images = mergeInteractiveImages(
    deferredImageEvents.map((event) => readInteractiveImage(event.result)).filter((image): image is NonNullable<typeof image> => Boolean(image)),
    options.optimisticImages,
  )
  const imageError = latestImageError(deferredImageEvents)
  const imageStatus = images?.length ? 'success' : latestImageStatus(deferredImageEvents)
  const metadata: AgentMessageMetadata = {
    display_role: 'assistant',
    created_at: turn.ts,
    turn_id: turn.id,
    navigation_turn_id: turn.id,
    run_id: turn.run_id,
    agent_kind: turn.agent_kind,
    turn_versions: turn.versions,
    turn_version_index: turn.version_idx,
    display_phase: 'final',
  }
  const message = createAgentTextMessage({
    id,
    role: 'assistant',
    text: sanitizeStoredNarrative(turn.narrative),
    metadata,
  })
  if (!images?.length && !imageError && !imageStatus) return message
  return {
    ...message,
    parts: [
      ...message.parts,
      {
        type: 'data-agent-interactive-image',
        id: `${id}:interactive-image`,
        providerMetadata: {
          agent: { tool_presentation: { call: 'interactive_media', result: 'interactive_media' } },
        },
        data: {
          id,
          role: 'assistant',
          interactive_image: latestMergedInteractiveImage(images),
          interactive_images: images,
          interactive_image_error: imageError,
          interactive_image_status: imageStatus,
        },
      },
    ],
  } as AgentUIMessage
}

function displayEventMetadata(event: TurnDisplayEvent): AgentMessageMetadata {
  return {
    created_at: event.created_at,
    run_id: event.run_id,
    agent_cycle: event.agent_cycle,
    display_segment_id: event.id,
    agent_kind: event.agent_kind,
    agent_name: event.agent_name,
    root_agent_name: event.root_agent_name,
    run_path: event.run_path,
    subagent: event.subagent,
    subagent_session_id: event.subagent_session_id,
    subagent_type: event.subagent_type,
		parent_call_id: event.parent_call_id,
    tool_presentation: event.tool_presentation,
  }
}

function latestImageError(events: TurnDisplayEvent[]) {
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const error = readInteractiveImageError(events[index].result)
    if (error) return error
  }
  return undefined
}

function latestImageStatus(events: TurnDisplayEvent[]): 'running' | 'success' | 'error' | undefined {
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const status = events[index].status
    if (status === 'running' || status === 'success' || status === 'error') return status
  }
  return undefined
}
