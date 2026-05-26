import type { SSEEvent } from '@/lib/api'

export type InteractiveSubmode = 'story' | 'setting'

export interface StorySummary {
  id: string
  title: string
  origin: string
  story_teller_id: string
  created_at: string
  updated_at: string
  branches: number
  events: number
}

export interface StoryIndex {
  current_story_id: string
  stories: StorySummary[]
}

export interface Teller {
  id: string
  name: string
  description: string
  random_event_rate: number
  tags: string[]
  prompt?: string
  custom: boolean
  invalid?: boolean
  error?: string
}

export interface TurnEvent {
  id: string
  parent_id: string | null
  branch_id: string
  ts: string
  user: string
  narrative: string
  thinking?: string
  state_delta?: StateDelta
}

export interface StateDelta {
  ops: StateOp[]
}

export interface StateOp {
  op: string
  path: string
  value?: unknown
}

export interface Snapshot {
  story_id: string
  branch_id: string
  turns: TurnEvent[]
  current_turn?: TurnEvent
  state: Record<string, unknown>
  graph?: StoryGraph
}

export interface BranchSummary {
  id: string
  head: string
  from?: string
  from_event?: string
  title?: string
  created_at: string
  current: boolean
}

export interface PlotNode {
  id: string
  parent_id?: string
  branch_id: string
  title: string
  summary: string
  ts: string
  current: boolean
  head: boolean
}

export interface StoryGraph {
  nodes: PlotNode[]
  branches: BranchSummary[]
}

export type InteractiveSSEEvent = SSEEvent
