import { requestJSON } from './client'

// —— 类型:与 Go 侧 interactive 包结构一一对应 ——

export type MemoryKind =
  | 'knowledge'
  | 'reveal'
  | 'promise'
  | 'object_state'
  | 'relationship'
  | 'beat'

export interface NarrativeMemoryRecord {
  id: string
  kind: MemoryKind
  subject: string
  object?: string
  text: string
  evidence: string
  valid_from: string
  valid_to?: string
  status?: 'open' | 'paid'
  confidence?: number
}

export interface MemoryLibraryEntry extends NarrativeMemoryRecord {
  event_id: string
  epoch: number
  source_turn_id: string
  ts: string
}

export interface NarrativeMemoryTrace {
  model?: string
  duration_ms?: number
  prompt_tokens?: number
  completion_tokens?: number
  dropped_records?: { raw: string; reason: string }[]
  /** 确定性对齐层改写过的实体写法。 */
  aligned_entities?: { record_id: string; field: string; from: string; to: string }[]
  skipped_reason?: string
}

export interface MemoryLibraryStats {
  total_turns: number
  turns_with_memory: number
  coverage: number
  events: number
  records: number
  kind_counts: Record<string, number>
  open_promises: number
  paid_promises: number
  expired_records: number
  last_publish?: NarrativeMemoryTrace
}

export interface MemoryLibraryView {
  story_id: string
  branch_id: string
  kind?: string
  before_turn_id?: string
  entries: MemoryLibraryEntry[]
  stats: MemoryLibraryStats
}

export interface MemorySearchHit {
  record_id: string
  kind: MemoryKind
  subject: string
  object?: string
  text: string
  evidence: string
  valid_from: string
  valid_to?: string
  status?: string
  score?: number
  expanded_from?: string
  /** 被展开拉入时的跳数（1 起）；直接命中缺省。 */
  expanded_hop?: number
  /** 从直接命中出发抵达该记录的实体路径。 */
  expanded_path?: string[]
}

export interface MemoryKeywordPart {
  field: 'subject' | 'object' | 'text'
  keyword: string
  score: number
}

export interface MemoryHitDetail {
  record_id: string
  keyword_parts?: MemoryKeywordPart[]
  promise_boost?: number
  expanded_from?: string
  /** 与查询向量的余弦相似度；向量未启用时缺省。 */
  vector_score?: number
  /** 两路召回中的名次（1 起；缺省 = 未进该路）。 */
  keyword_rank?: number
  vector_rank?: number
  /** RRF 融合分，向量启用时决定最终排序。 */
  fused_score?: number
}

export interface MemoryFilteredItem {
  record_id: string
  subject: string
  text: string
  reason: string
  score?: number
}

export interface MemorySearchPipelineCounters {
  projected_events: number
  deduped_records: number
  stale_records: number
  valid_records: number
  expired_records: number
  candidates: number
  keyword_matched: number
  vector_candidates: number
  fused_ranked: number
  anchors: number
  expanded_records: number
  expanded_hops: number
  final_after_budget: number
}

export interface MemorySearchExplain {
  pipeline: MemorySearchPipelineCounters
  hit_details?: MemoryHitDetail[]
  filtered?: MemoryFilteredItem[]
}

export interface MemorySearchResult {
  story_id: string
  branch_id: string
  keywords?: string[]
  kind?: string
  subject?: string
  match: string
  before_turn_id?: string
  limit: number
  truncated: boolean
  /** 本次检索是否用上了向量召回；false = 结果完全来自关键词路径。 */
  vector_enabled?: boolean
  hits: MemorySearchHit[]
  explain?: MemorySearchExplain
}

// —— 请求 ——

export interface MemoryBrowseOptions {
  branch?: string
  kind?: string
  beforeTurnId?: string
}

export async function browseInteractiveMemory(storyId: string, options: MemoryBrowseOptions = {}): Promise<MemoryLibraryView> {
  const query = new URLSearchParams()
  if (options.branch) query.set('branch', options.branch)
  if (options.kind) query.set('kind', options.kind)
  if (options.beforeTurnId) query.set('before_turn_id', options.beforeTurnId)
  const suffix = query.toString() ? `?${query.toString()}` : ''
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/memory${suffix}`)
}

export interface MemorySearchQuery {
  q?: string
  branch?: string
  kind?: string
  subject?: string
  beforeTurnId?: string
  limit?: number
}

export async function searchInteractiveMemory(storyId: string, query: MemorySearchQuery = {}): Promise<MemorySearchResult> {
  const params = new URLSearchParams()
  if (query.q) params.set('q', query.q)
  if (query.branch) params.set('branch', query.branch)
  if (query.kind) params.set('kind', query.kind)
  if (query.subject) params.set('subject', query.subject)
  if (query.beforeTurnId) params.set('before_turn_id', query.beforeTurnId)
  if (query.limit && query.limit > 0) params.set('limit', String(query.limit))
  const suffix = params.toString() ? `?${params.toString()}` : ''
  return requestJSON(`/api/interactive/stories/${encodeURIComponent(storyId)}/memory/search${suffix}`)
}
