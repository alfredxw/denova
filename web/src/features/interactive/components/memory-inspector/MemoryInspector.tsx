import { useCallback, useEffect, useMemo, useState } from 'react'
import { Brain, Loader2, RefreshCw, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  browseInteractiveMemory,
  searchInteractiveMemory,
  type MemoryKind,
  type MemoryLibraryEntry,
  type MemoryLibraryView,
  type MemorySearchResult,
} from '@/lib/api-client/interactive-memory'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

// 叙事记忆检视器:记忆库浏览 / 检索调试 / 流水线健康 三个标签页。
// 数据来自 narrative_memory 事件投影;检索调试跑与 Agent 工具同一代码
// 路径,但额外返回 Explain(得分分解、过滤原因、计数瀑布)。
export function MemoryInspector({ storyId, branchId }: { storyId: string; branchId: string }) {
  const { t } = useTranslation()
  const [tab, setTab] = useState<'library' | 'debug' | 'health'>('library')

  const [library, setLibrary] = useState<MemoryLibraryView | null>(null)
  const [libraryLoading, setLibraryLoading] = useState(false)
  const [libraryError, setLibraryError] = useState('')
  const [kindFilter, setKindFilter] = useState<'' | MemoryKind>('')

  const [query, setQuery] = useState('')
  const [searchResult, setSearchResult] = useState<MemorySearchResult | null>(null)
  const [searchLoading, setSearchLoading] = useState(false)
  const [searchError, setSearchError] = useState('')

  const loadLibrary = useCallback(async () => {
    if (!storyId) return
    setLibraryLoading(true)
    setLibraryError('')
    try {
      const view = await browseInteractiveMemory(storyId, { branch: branchId, kind: kindFilter || undefined })
      setLibrary(view)
    } catch (err) {
      setLibraryError(err instanceof Error ? err.message : String(err))
    } finally {
      setLibraryLoading(false)
    }
  }, [storyId, branchId, kindFilter])

  useEffect(() => {
    void loadLibrary()
  }, [loadLibrary])

  const runSearch = useCallback(async () => {
    if (!storyId) return
    setSearchLoading(true)
    setSearchError('')
    try {
      const result = await searchInteractiveMemory(storyId, { q: query, branch: branchId })
      setSearchResult(result)
    } catch (err) {
      setSearchError(err instanceof Error ? err.message : String(err))
    } finally {
      setSearchLoading(false)
    }
  }, [storyId, branchId, query])

  const grouped = useMemo(() => groupByKind(library?.entries || []), [library])

  return (
    <section data-testid="memory-inspector" className="flex h-full min-h-0 flex-col bg-[var(--nova-panel,#1c1c22)] text-[var(--nova-text,#e8e8ea)]">
      <header className="flex shrink-0 items-center gap-2 border-b border-[var(--nova-border,#333)] px-4 py-3">
        <Brain className="h-4 w-4 text-[var(--nova-accent,#7c9cff)]" />
        <h2 className="text-sm font-semibold">{t('memoryInspector.title', '叙事记忆')}</h2>
        <nav className="ml-4 flex items-center gap-1">
          {([
            ['library', t('memoryInspector.tabs.library', '记忆库')],
            ['debug', t('memoryInspector.tabs.debug', '检索调试')],
            ['health', t('memoryInspector.tabs.health', '流水线健康')],
          ] as const).map(([key, label]) => (
            <button
              key={key}
              type="button"
              data-testid={`memory-tab-${key}`}
              className={`rounded px-2.5 py-1 text-xs transition-colors ${tab === key ? 'bg-[var(--nova-hover,#2a2a33)] font-medium' : 'text-[var(--nova-text-muted,#999)] hover:bg-[var(--nova-hover,#2a2a33)]'}`}
              onClick={() => setTab(key)}
            >
              {label}
            </button>
          ))}
        </nav>
        <Button
          variant="outline"
          size="xs"
          className="ml-auto gap-1.5"
          onClick={() => void loadLibrary()}
          disabled={libraryLoading}
        >
          {libraryLoading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
          {t('memoryInspector.refresh', '刷新')}
        </Button>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
        {libraryError && <p className="mb-3 rounded border border-red-800 bg-red-950/40 px-3 py-2 text-xs text-red-300">{libraryError}</p>}

        {tab === 'library' && (
          <div className="space-y-4" data-testid="memory-library">
            <div className="flex flex-wrap items-center gap-1.5">
              {(['', 'knowledge', 'promise', 'object_state', 'relationship', 'reveal', 'beat'] as const).map((kind) => (
                <button
                  key={kind || 'all'}
                  type="button"
                  onClick={() => setKindFilter(kind)}
                  className={`rounded-full border px-2.5 py-0.5 text-xs ${kindFilter === kind ? 'border-[var(--nova-accent,#7c9cff)] text-[var(--nova-accent,#7c9cff)]' : 'border-[var(--nova-border,#333)] text-[var(--nova-text-muted,#999)]'}`}
                >
                  {kind === '' ? t('memoryInspector.allKinds', '全部') : kindLabel(kind, t)}
                </button>
              ))}
            </div>
            {Object.entries(grouped).map(([kind, entries]) => (
              <div key={kind}>
                <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-[var(--nova-text-muted,#999)]">
                  {kindLabel(kind as MemoryKind, t)} · {entries.length}
                </h3>
                <ul className="space-y-1.5">
                  {entries.map((entry) => (
                    <li key={entry.id} data-testid="memory-entry" className="rounded border border-[var(--nova-border,#333)] bg-[var(--nova-panel-2,#232329)] px-3 py-2 text-xs leading-relaxed">
                      <div className="flex items-center gap-2">
                        <span className="font-medium">{entry.subject}</span>
                        {entry.object && <span className="text-[var(--nova-text-muted,#999)]">→ {entry.object}</span>}
                        {entry.valid_to
                          ? <span className="rounded bg-amber-950/60 px-1.5 py-0.5 text-[10px] text-amber-300">{t('memoryInspector.overturnedAt', '已推翻')} @{entry.valid_to.slice(0, 12)}</span>
                          : <span className="rounded bg-emerald-950/60 px-1.5 py-0.5 text-[10px] text-emerald-300">{t('memoryInspector.valid', '有效')}</span>}
                        {entry.kind === 'promise' && entry.status && (
                          <span className={`rounded px-1.5 py-0.5 text-[10px] ${entry.status === 'paid' ? 'bg-sky-950/60 text-sky-300' : 'bg-violet-950/60 text-violet-300'}`}>
                            {entry.status === 'paid' ? t('memoryInspector.paid', '已兑现') : t('memoryInspector.open', '悬置')}
                          </span>
                        )}
                      </div>
                      <p className="mt-1 text-[var(--nova-text,#e8e8ea)]">{entry.text}</p>
                      <p className="mt-1 text-[11px] text-[var(--nova-text-muted,#999)]">
                        {t('memoryInspector.evidence', '证据')}: {entry.evidence} · {t('memoryInspector.sourceTurn', '来源')} {entry.source_turn_id.slice(0, 12)}
                      </p>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
            {!libraryLoading && (library?.entries || []).length === 0 && (
              <p className="py-8 text-center text-xs text-[var(--nova-text-muted,#999)]">{t('memoryInspector.empty', '暂无叙事记忆:回合提交后抽取,或等待 Publish 管道接入。')}</p>
            )}
          </div>
        )}

        {tab === 'debug' && (
          <div className="space-y-4" data-testid="memory-debug">
            <div className="flex gap-2">
              <Input
                data-testid="memory-debug-input"
                value={query}
                placeholder={t('memoryInspector.searchPlaceholder', '输入关键词,空格分隔;留空浏览当前有效记忆')}
                onChange={(e) => setQuery(e.target.value)}
                onKeyDown={(e) => { if (e.key === 'Enter') void runSearch() }}
              />
              <Button size="sm" className="gap-1.5" onClick={() => void runSearch()} disabled={searchLoading}>
                {searchLoading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Search className="h-3.5 w-3.5" />}
                {t('memoryInspector.search', '检索')}
              </Button>
            </div>
            {searchError && <p className="rounded border border-red-800 bg-red-950/40 px-3 py-2 text-xs text-red-300">{searchError}</p>}
            {searchResult && <MemoryDebugResult result={searchResult} />}
          </div>
        )}

        {tab === 'health' && library && <MemoryHealth view={library} />}
      </div>
    </section>
  )
}

function MemoryDebugResult({ result }: { result: MemorySearchResult }) {
  const { t } = useTranslation()
  const p = result.explain?.pipeline
  return (
    <div className="space-y-4">
      {p && (
        <div className="rounded border border-[var(--nova-border,#333)] p-3" data-testid="memory-pipeline">
          <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-[var(--nova-text-muted,#999)]">{t('memoryInspector.pipeline', '流水线')}</h3>
          <MemoryPipelineRow label={t('memoryInspector.pipelineEvents', '事件投影')} value={p.projected_events} max={p.projected_events} />
          <MemoryPipelineRow label={t('memoryInspector.pipelineDeduped', 'epoch 去重')} value={p.deduped_records} max={p.projected_events} note={p.stale_records > 0 ? `-${p.stale_records}` : undefined} />
          <MemoryPipelineRow label={t('memoryInspector.pipelineValid', '有效期过滤')} value={p.valid_records} max={p.projected_events} note={p.expired_records > 0 ? `-${p.expired_records}` : undefined} />
          <MemoryPipelineRow label={t('memoryInspector.pipelineKeyword', '关键词命中')} value={p.keyword_matched} max={p.projected_events} />
          {result.vector_enabled && (
            <>
              <MemoryPipelineRow label={t('memoryInspector.pipelineVector', '向量召回')} value={p.vector_candidates} max={p.projected_events} />
              <MemoryPipelineRow label={t('memoryInspector.pipelineFused', 'RRF 融合')} value={p.fused_ranked} max={p.projected_events} />
            </>
          )}
          <MemoryPipelineRow label={t('memoryInspector.pipelineExpansion', '一跳展开')} value={p.expanded_records} max={p.projected_events} note={p.anchors > 0 ? `+${p.anchors} ${t('memoryInspector.anchors', '锚点')}` : undefined} />
          <MemoryPipelineRow label={t('memoryInspector.pipelineBudget', '预算截断')} value={p.final_after_budget} max={p.projected_events} note={result.truncated ? t('memoryInspector.truncated', '已截断') : undefined} />
        </div>
      )}
      <div>
        <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-[var(--nova-text-muted,#999)]">{t('memoryInspector.hits', '命中')} · {result.hits.length}</h3>
        <ul className="space-y-1.5">
          {result.hits.map((hit) => {
            const detail = result.explain?.hit_details?.find((d) => d.record_id === hit.record_id)
            return (
              <li key={hit.record_id} data-testid="memory-hit" className="rounded border border-[var(--nova-border,#333)] bg-[var(--nova-panel-2,#232329)] px-3 py-2 text-xs">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="rounded bg-[var(--nova-hover,#2a2a33)] px-1.5 py-0.5 text-[10px]">{kindLabel(hit.kind, t)}</span>
                  <span className="font-medium">{hit.subject}</span>
                  {hit.object && <span className="text-[var(--nova-text-muted,#999)]">→ {hit.object}</span>}
                  <span className="text-[var(--nova-text-muted,#999)]">score {hit.score}</span>
                  {hit.expanded_from && <span className="rounded bg-violet-950/60 px-1.5 py-0.5 text-[10px] text-violet-300">{t('memoryInspector.expandedFrom', '展开自')} {hit.expanded_from}</span>}
                  {detail?.vector_rank ? (
                    <span className="rounded bg-sky-950/60 px-1.5 py-0.5 text-[10px] text-sky-300" title={t('memoryInspector.vectorScore', '余弦相似度')}>
                      {t('memoryInspector.vectorRank', '向量')} #{detail.vector_rank} · {detail.vector_score?.toFixed(3)}
                    </span>
                  ) : null}
                </div>
                <p className="mt-1">{hit.text}</p>
                <p className="mt-1 text-[11px] text-[var(--nova-text-muted,#999)]">{t('memoryInspector.evidence', '证据')}: {hit.evidence} · {hit.valid_from.slice(0, 12)}{hit.valid_to ? ` → ${hit.valid_to.slice(0, 12)}` : ''}</p>
                {detail?.keyword_parts && detail.keyword_parts.length > 0 && (
                  <p className="mt-1 text-[11px] text-[var(--nova-text-muted,#999)]">
                    {detail.keyword_parts.map((part) => `${part.field}:"${part.keyword}"+${part.score}`).join(' · ')}
                    {detail.promise_boost ? ` · ${t('memoryInspector.promiseBoost', '伏笔')}+${detail.promise_boost}` : ''}
                  </p>
                )}
                {result.vector_enabled && detail?.fused_score ? (
                  <p className="mt-1 text-[11px] text-[var(--nova-text-muted,#999)]">
                    {t('memoryInspector.fusedScore', '融合分')} {detail.fused_score.toFixed(4)}
                    {detail.keyword_rank ? ` · ${t('memoryInspector.keywordRank', '关键词')} #${detail.keyword_rank}` : ''}
                    {detail.vector_rank ? ` · ${t('memoryInspector.vectorRank', '向量')} #${detail.vector_rank}` : ''}
                  </p>
                ) : null}
              </li>
            )
          })}
        </ul>
      </div>
      {(result.explain?.filtered || []).length > 0 && (
        <div>
          <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-[var(--nova-text-muted,#999)]">{t('memoryInspector.filtered', '落选对照')} · {result.explain?.filtered?.length}</h3>
          <ul className="space-y-1">
            {result.explain?.filtered?.map((item) => (
              <li key={`${item.record_id}-${item.reason}`} data-testid="memory-filtered" className="flex items-center gap-2 rounded border border-dashed border-[var(--nova-border,#333)] px-3 py-1.5 text-[11px]">
                <span className="rounded bg-[var(--nova-hover,#2a2a33)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted,#999)]">{filterReasonLabel(item.reason, t)}</span>
                <span className="font-medium">{item.subject}</span>
                <span className="min-w-0 flex-1 truncate text-[var(--nova-text-muted,#999)]">{item.text}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}

function MemoryPipelineRow({ label, value, max, note }: { label: string; value: number; max: number; note?: string }) {
  const width = max > 0 ? Math.max(2, Math.round((value / max) * 100)) : 0
  return (
    <div className="mb-1.5 flex items-center gap-2 text-[11px]">
      <span className="w-20 shrink-0 text-[var(--nova-text-muted,#999)]">{label}</span>
      <div className="h-3 flex-1 overflow-hidden rounded bg-[var(--nova-hover,#2a2a33)]">
        <div className="h-full rounded bg-[var(--nova-accent,#7c9cff)] opacity-70" style={{ width: `${width}%` }} />
      </div>
      <span className="w-20 shrink-0 text-right tabular-nums">
        {value}
        {note && <span className="ml-1 text-[var(--nova-text-muted,#999)]">{note}</span>}
      </span>
    </div>
  )
}

function MemoryHealth({ view }: { view: MemoryLibraryView }) {
  const { t } = useTranslation()
  const stats = view.stats
  const coverage = Math.round(stats.coverage * 100)
  return (
    <div className="space-y-4" data-testid="memory-health">
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        <HealthTile label={t('memoryInspector.coverage', '覆盖率')} value={`${coverage}%`} sub={`${stats.turns_with_memory}/${stats.total_turns}`} />
        <HealthTile label={t('memoryInspector.totalRecords', '记忆总数')} value={String(stats.records)} sub={`${stats.events} ${t('memoryInspector.events', '事件')}`} />
        <HealthTile label={t('memoryInspector.openPromises', '悬置伏笔')} value={String(stats.open_promises)} sub={`${stats.paid_promises} ${t('memoryInspector.paidPromises', '已兑现')}`} />
        <HealthTile label={t('memoryInspector.expired', '已推翻')} value={String(stats.expired_records)} />
      </div>
      <div className="rounded border border-[var(--nova-border,#333)] p-3">
        <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-[var(--nova-text-muted,#999)]">{t('memoryInspector.kindDistribution', '类型分布')}</h3>
        <div className="space-y-1">
          {Object.entries(stats.kind_counts).map(([kind, count]) => (
            <MemoryPipelineRow key={kind} label={kindLabel(kind as MemoryKind, t)} value={count} max={stats.records} />
          ))}
        </div>
      </div>
      {stats.last_publish && (
        <div className="rounded border border-[var(--nova-border,#333)] p-3 text-[11px]">
          <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-[var(--nova-text-muted,#999)]">{t('memoryInspector.lastPublish', '最近抽取')}</h3>
          <p className="text-[var(--nova-text-muted,#999)]">
            {stats.last_publish.model || '-'} · {stats.last_publish.duration_ms ?? '-'}ms · {t('memoryInspector.tokens', 'token')} {stats.last_publish.prompt_tokens ?? '-'}/{stats.last_publish.completion_tokens ?? '-'}
          </p>
          {(stats.last_publish.dropped_records || []).length > 0 && (
            <p className="mt-1 text-amber-400">{t('memoryInspector.dropped', '丢弃')}: {stats.last_publish.dropped_records?.length}</p>
          )}
        </div>
      )}
    </div>
  )
}

function HealthTile({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="rounded border border-[var(--nova-border,#333)] bg-[var(--nova-panel-2,#232329)] px-3 py-2.5">
      <p className="text-[10px] uppercase tracking-wide text-[var(--nova-text-muted,#999)]">{label}</p>
      <p className="mt-0.5 text-lg font-semibold tabular-nums">{value}</p>
      {sub && <p className="text-[10px] text-[var(--nova-text-muted,#999)]">{sub}</p>}
    </div>
  )
}

function groupByKind(entries: MemoryLibraryEntry[]): Record<string, MemoryLibraryEntry[]> {
  const order: MemoryKind[] = ['knowledge', 'promise', 'object_state', 'relationship', 'reveal', 'beat']
  const groups: Record<string, MemoryLibraryEntry[]> = {}
  for (const kind of order) {
    const matched = entries.filter((entry) => entry.kind === kind)
    if (matched.length > 0) groups[kind] = matched
  }
  return groups
}

function kindLabel(kind: MemoryKind, t: TFunction): string {
  const map: Record<MemoryKind, string> = {
    knowledge: t('memoryInspector.kinds.knowledge', '认知'),
    reveal: t('memoryInspector.kinds.reveal', '揭示'),
    promise: t('memoryInspector.kinds.promise', '伏笔'),
    object_state: t('memoryInspector.kinds.object_state', '物品'),
    relationship: t('memoryInspector.kinds.relationship', '关系'),
    beat: t('memoryInspector.kinds.beat', '节拍'),
  }
  return map[kind] || kind
}

function filterReasonLabel(reason: string, t: TFunction): string {
  const map: Record<string, string> = {
    expired: t('memoryInspector.reasons.expired', '已推翻'),
    kind_mismatch: t('memoryInspector.reasons.kindMismatch', '类型不符'),
    subject_mismatch: t('memoryInspector.reasons.subjectMismatch', '实体不符'),
    no_keyword_match: t('memoryInspector.reasons.noKeyword', '关键词未命中'),
    low_score: t('memoryInspector.reasons.lowScore', '得分不足'),
    budget_cut: t('memoryInspector.reasons.budgetCut', '预算截断'),
  }
  return map[reason] || reason
}
