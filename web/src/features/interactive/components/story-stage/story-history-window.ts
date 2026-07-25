import type { Snapshot, StoryHistoryPage, TurnEvent } from '../../types'

// The display cache may be generous because message DOM is virtualized, but it
// must still have a hard ceiling independent from canonical story history.
export const STORY_HISTORY_CACHE_MAX_TURNS = 10_000
export const STORY_HISTORY_CACHE_MAX_BYTES = 96 * 1024 * 1024

export interface StoryHistoryCacheLimits {
  maxTurns: number
  maxBytes: number
}

const DEFAULT_STORY_HISTORY_CACHE_LIMITS: StoryHistoryCacheLimits = {
  maxTurns: STORY_HISTORY_CACHE_MAX_TURNS,
  maxBytes: STORY_HISTORY_CACHE_MAX_BYTES,
}

// Snapshot turns are immutable read-model objects. Cache their estimates so a
// new tail page does not JSON-serialize the retained 10,000-turn window again.
const storyTurnByteEstimates = new WeakMap<TurnEvent, number>()

export interface StoryHistoryWindow {
  stageKey: string
  turns: TurnEvent[]
  beforeCursor: string
  hasMore: boolean
  expanded: boolean
  followLatest: boolean
  approximateBytes: number
}

export function createStoryHistoryWindow(stageKey: string, snapshot: Snapshot | null): StoryHistoryWindow {
  const bounded = boundStoryTurns(snapshot?.turns || [], 'latest')
  return {
    stageKey,
    turns: bounded.turns,
    beforeCursor: snapshot?.history_before_cursor || '',
    hasMore: snapshot?.has_earlier_turns === true || bounded.trimmed,
    expanded: false,
    followLatest: true,
    approximateBytes: bounded.approximateBytes,
  }
}

export function reconcileStoryHistoryWindow(
  current: StoryHistoryWindow,
  stageKey: string,
  snapshot: Snapshot | null,
): StoryHistoryWindow {
  if (!snapshot || current.stageKey !== stageKey) return createStoryHistoryWindow(stageKey, snapshot)
  if (!current.followLatest) return current

  const bounded = boundStoryTurns(mergeStoryHistoryTurns(current.turns, snapshot.turns || []), 'latest')
  return {
    ...current,
    turns: bounded.turns,
    beforeCursor: current.beforeCursor || snapshot.history_before_cursor || '',
    hasMore: current.hasMore || snapshot.has_earlier_turns === true || bounded.trimmed,
    approximateBytes: bounded.approximateBytes,
  }
}

export function prependStoryHistoryPage(
  current: StoryHistoryWindow,
  stageKey: string,
  page: StoryHistoryPage,
): StoryHistoryWindow {
  if (current.stageKey !== stageKey) return current
  const currentIDs = new Set(current.turns.map((turn) => turn.id))
  const merged = [...(page.turns || []).filter((turn) => !currentIDs.has(turn.id)), ...current.turns]
  const newestBound = boundStoryTurns(merged, 'latest')
  const followLatest = current.followLatest && !newestBound.trimmed
  const bounded = followLatest ? newestBound : boundStoryTurns(merged, 'earliest')
  return {
    stageKey,
    turns: bounded.turns,
    beforeCursor: page.before_cursor || '',
    hasMore: page.has_more,
    expanded: true,
    followLatest,
    approximateBytes: bounded.approximateBytes,
  }
}

export function projectStoryHistorySnapshot(
  snapshot: Snapshot | null,
  window: StoryHistoryWindow,
  stageKey: string,
): Snapshot | null {
  if (!snapshot || window.stageKey !== stageKey) return snapshot
  if (!window.followLatest) return { ...snapshot, turns: window.turns }
  const bounded = boundStoryTurns(mergeStoryHistoryTurns(window.turns, snapshot.turns || []), 'latest')
  return { ...snapshot, turns: bounded.turns }
}

export function mergeStoryHistoryTurns(current: TurnEvent[], latest: TurnEvent[]): TurnEvent[] {
  if (current.length === 0) return latest
  if (latest.length === 0) return current
  const currentIndexes = new Map(current.map((turn, index) => [turn.id, index]))
  const firstOverlap = latest.findIndex((turn) => currentIndexes.has(turn.id))
  if (firstOverlap < 0) return latest
  const currentOverlap = currentIndexes.get(latest[firstOverlap].id) || 0
  return [...current.slice(0, currentOverlap), ...latest.slice(firstOverlap)]
}

export function boundStoryTurns(
  turns: TurnEvent[],
  keep: 'earliest' | 'latest',
  limits: StoryHistoryCacheLimits = DEFAULT_STORY_HISTORY_CACHE_LIMITS,
) {
  if (turns.length === 0) return { turns: [] as TurnEvent[], approximateBytes: 0, trimmed: false }
  const selected: TurnEvent[] = []
  let approximateBytes = 0
  const append = (turn: TurnEvent) => {
    const turnBytes = approximateStoryTurnBytes(turn)
    if (selected.length >= limits.maxTurns) return false
    if (selected.length > 0 && approximateBytes + turnBytes > limits.maxBytes) return false
    selected.push(turn)
    approximateBytes += turnBytes
    return true
  }

  if (keep === 'earliest') {
    for (const turn of turns) {
      if (!append(turn)) break
    }
  } else {
    for (let index = turns.length - 1; index >= 0; index -= 1) {
      if (!append(turns[index])) break
    }
    selected.reverse()
  }
  return { turns: selected, approximateBytes, trimmed: selected.length < turns.length }
}

function approximateStoryTurnBytes(turn: TurnEvent) {
  const cached = storyTurnByteEstimates.get(turn)
  if (cached !== undefined) return cached
  let estimate: number
  try {
    // JavaScript strings are normally UTF-16. This deliberately overestimates
    // ASCII payloads so tool traces reach the byte ceiling before heap pressure
    // becomes surprising.
    estimate = Math.max(1, JSON.stringify(turn).length * 2)
  } catch {
    estimate = STORY_HISTORY_CACHE_MAX_BYTES
  }
  storyTurnByteEstimates.set(turn, estimate)
  return estimate
}
