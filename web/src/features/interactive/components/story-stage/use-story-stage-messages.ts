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
  parseAgentToolInput,
} from '@/lib/agent-ui-message'
import type { Snapshot, TurnDisplayEvent, TurnEvent } from '../../types'
import { isDirectorDisplayEvent } from '../director-console/utils'
import type { TurnNavigationItem } from '../TurnNavigator'
import { sanitizeStoredNarrative } from '../../stream-parser'
import {
  latestMergedInteractiveImage,
  mergeInteractiveImages,
  readInteractiveImage,
  readInteractiveImageError,
} from './interactive-images'
import { publicRuleRollFromResolution } from './rule-roll'
import { buildTokenUsageMessage, mergeTokenUsageMessages } from './token-usage'
import { normalizeMessageContent } from './utils'

interface UseStoryStageMessagesOptions {
  snapshot: Snapshot | null
  rewindTurnId?: string
  liveMessages: AgentUIMessage[]
  streaming: boolean
  stageKey: string
  liveTurnNavigationAnchorId: string
  publicRuleRollVisible: boolean
  optimisticInteractiveImages: Record<string, import('@/lib/api').InteractiveImage[]>
  belongsToStage: (stageKey: string) => boolean
  renderKeyFor: (turnId: string, role: 'user' | 'assistant') => string | undefined
}

// Projects persisted domain turns and ephemeral display events directly into
// the shared UI message protocol. This remains a UI-only read model and never
// participates in model-context assembly.
export function useStoryStageMessages({
  snapshot,
  rewindTurnId,
  liveMessages,
  streaming,
  stageKey,
  liveTurnNavigationAnchorId,
  publicRuleRollVisible,
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
      publicRuleRollVisible,
      renderKeyFor,
    })),
    [optimisticInteractiveImages, publicRuleRollVisible, renderKeyFor, storyPathTurns],
  )

  const agentMessages = useMemo(
    () => normalizeAgentUIMessages([
      ...historyMessages,
      ...(hasPersistedLiveTurn ? [] : liveMessages.filter((message) => !agentMessageHasDataPart(message, 'agent-token-usage'))),
    ]),
    [hasPersistedLiveTurn, historyMessages, liveMessages],
  )
  const turnNavigationItems = useMemo<TurnNavigationItem[]>(() => {
    const items: TurnNavigationItem[] = storyPathTurns.map((turn) => ({
      anchorId: turn.id,
      user: turn.user,
      narrative: sanitizeStoredNarrative(turn.narrative),
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
  publicRuleRollVisible: boolean
  renderKeyFor: (turnId: string, role: 'user' | 'assistant') => string | undefined
}) {
  const messages: AgentUIMessage[] = [createAgentTextMessage({
    id: options.renderKeyFor(turn.id, 'user') || `${turn.id}-user`,
    role: 'user',
    text: turn.user,
    metadata: {
      display_role: 'user',
      turn_id: turn.id,
      navigation_turn_id: turn.id,
    },
  })]
  const displayEvents = (turn.display_events || []).filter((event) => !isDirectorDisplayEvent(event))
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
      case 'thinking':
        timeline.push(createAgentReasoningMessage({
          id: event.id || `${turn.id}-thinking-${index}`,
          text: event.content || '',
          metadata: { ...metadata, display_role: 'thinking' },
        }))
        break
      case 'tool_call': {
        if (event.tool_presentation?.result === 'interactive_media') {
          deferredImageEvents.push(event)
          break
        }
        const status = event.status || 'success'
        timeline.push(createAgentToolMessage({
          id: event.id || `${turn.id}-tool-${index}`,
          name: event.name || event.content || 'unknown_tool',
          state: status === 'error' ? 'output-error' : status === 'success' ? 'output-available' : 'input-available',
          input: parseAgentToolInput(event.args || ''),
          output: status === 'error' ? undefined : event.result || undefined,
          errorText: status === 'error' ? event.result || '' : undefined,
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
  const ruleRoll = options.publicRuleRollVisible ? publicRuleRollFromResolution(turn.rule_resolution) : null
  if (ruleRoll) {
    const id = `${turn.id}-rule-roll`
    messages.push(createAgentDataMessage({
      id,
      type: 'agent-rule-roll',
      metadata: { display_role: 'rule_roll', turn_id: turn.id, navigation_turn_id: turn.id },
      data: { id, role: 'rule_roll', rule_roll: ruleRoll },
    }))
  }
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
    turn_id: turn.id,
    navigation_turn_id: turn.id,
    run_id: turn.run_id,
    agent_kind: turn.agent_kind,
    turn_versions: turn.versions,
    turn_version_index: turn.version_idx,
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
    display_segment_id: event.id,
    agent_kind: event.agent_kind,
    agent_name: event.agent_name,
    root_agent_name: event.root_agent_name,
    run_path: event.run_path,
    subagent: event.subagent,
    subagent_session_id: event.subagent_session_id,
    subagent_type: event.subagent_type,
		parent_call_id: event.parent_call_id,
    sse_hidden_fields: event.sse_hidden_fields,
    sse_hidden_reason: event.sse_hidden_reason,
    sse_display_notice: event.sse_display_notice,
    sse_generated_chars: event.sse_generated_chars,
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
