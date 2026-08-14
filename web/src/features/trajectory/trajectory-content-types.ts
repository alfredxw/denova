import type { AgentRunTraceRecord } from '@/lib/api'
import type { TrajectorySpan } from './trajectory-analysis'

export type TrajectoryContentKind = 'system' | 'user' | 'context' | 'assistant' | 'tool'
export type TrajectoryDirection = 'input' | 'output'

export interface TrajectoryToolDefinition {
  name: string
  description: string
  parameters: unknown
  parametersError: string
  extra: Record<string, unknown>
}

export interface TrajectoryToolCall {
  id: string
  name: string
  arguments: string
  raw: Record<string, unknown>
}

export interface TrajectoryToolOutput {
  callID: string
  executionID: string
  name: string
  status: string
  content: string
  error: string
  truncated: boolean
  originalBytes: number
  returnedBytes: number
  span: TrajectorySpan | null
  raw: AgentRunTraceRecord
}

export interface TrajectoryContentEntry {
  id: string
  kind: TrajectoryContentKind
  direction: TrajectoryDirection
  requestIndex: number
  requestID: string
  messageIndex: number
  label: string
  content: string
  reasoning: string
  status: string
  toolName: string
  toolCallID: string
  toolCall: TrajectoryToolCall | null
  toolCalls: TrajectoryToolCall[]
  tools: TrajectoryToolDefinition[]
  raw: unknown
  source: Record<string, unknown>
  span: TrajectorySpan | null
  inputRecord: AgentRunTraceRecord
  outputRecord: AgentRunTraceRecord | null
  previousContent: string
  previousTools: TrajectoryToolDefinition[]
}

export interface TrajectoryToolExchange {
  id: string
  call: TrajectoryToolCall
  caller: TrajectoryContentEntry | null
  result: TrajectoryContentEntry | null
  output: TrajectoryToolOutput | null
  definition: TrajectoryToolDefinition | null
  span: TrajectorySpan | null
}

export type TrajectoryConversationNode =
  | { type: 'message'; id: string; entry: TrajectoryContentEntry }
  | { type: 'tool-group'; id: string; calls: TrajectoryToolExchange[] }

export interface TrajectoryRequest {
  id: string
  index: number
  span: TrajectorySpan | null
  inputRecord: AgentRunTraceRecord
  outputRecord: AgentRunTraceRecord | null
  tools: TrajectoryToolDefinition[]
  entries: TrajectoryContentEntry[]
  inputNodes: TrajectoryConversationNode[]
  outputNodes: TrajectoryConversationNode[]
  debugInputEntries: TrajectoryContentEntry[]
  debugOutputEntries: TrajectoryContentEntry[]
}

export interface TrajectoryContentAnalysis {
  available: boolean
  requests: TrajectoryRequest[]
  entries: TrajectoryContentEntry[]
  toolCalls: TrajectoryToolExchange[]
}
