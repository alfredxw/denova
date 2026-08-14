import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { ChevronRight, ChevronsDownUp, ChevronsUpDown, LockKeyhole, Search, WrapText } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { formatTrajectoryDuration } from './trajectory-analysis'
import type { TrajectoryContentAnalysis, TrajectoryContentSelection, TrajectoryConversationNode, TrajectoryDirection, TrajectoryRequest } from './trajectory-content'
import {
  TrajectoryConversationRows,
  TrajectoryDebugRows,
  TrajectoryToolDefinitionsRow,
} from './TrajectoryConversationRows'

type ConversationMode = 'readable' | 'debug'
type ConversationScope = 'all' | TrajectoryDirection

interface TrajectoryContentLedgerProps {
  content: TrajectoryContentAnalysis
  selection: TrajectoryContentSelection | null
  rangeSpanIDs: ReadonlySet<string> | null
  onSelect: (selection: TrajectoryContentSelection) => void
}

/** Hierarchical request ledger with paired tool calls and exact debug source. */
export function TrajectoryContentLedger({ content, selection, rangeSpanIDs, onSelect }: TrajectoryContentLedgerProps) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<ConversationMode>('readable')
  const [scope, setScope] = useState<ConversationScope>('all')
  const [query, setQuery] = useState('')
  const [wrap, setWrap] = useState(false)
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(() => defaultExpanded(content))
  const normalizedQuery = query.trim().toLowerCase()

  useEffect(() => {
    setExpanded(defaultExpanded(content))
    setMode('readable')
    setScope('all')
    setQuery('')
  }, [content])

  useEffect(() => {
    if (!selection) return
    const request = content.requests.find((candidate) => selection.type === 'entry'
      ? candidate.entries.some((entry) => entry.id === selection.id)
      : [...candidate.inputNodes, ...candidate.outputNodes].some((node) => node.type === 'tool-group' && node.calls.some((call) => call.id === selection.id)))
    if (!request) return
    const parentToolGroup = selection.type === 'tool'
      ? [...request.inputNodes, ...request.outputNodes].find((node) => node.type === 'tool-group' && node.calls.some((call) => call.id === selection.id))
      : null
    setExpanded((current) => new Set([...current, requestKey(request), ...(parentToolGroup ? [parentToolGroup.id] : [])]))
  }, [content.requests, selection])

  const visibleRequests = useMemo(() => content.requests.filter((request) => requestMatches(request, normalizedQuery, rangeSpanIDs)), [content.requests, normalizedQuery, rangeSpanIDs])
  const toggle = (id: string) => setExpanded((current) => {
    const next = new Set(current)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    return next
  })

  if (!content.available) {
    return (
      <section className="flex min-h-0 min-w-0 flex-1 items-center justify-center bg-[var(--nova-surface)] px-6 text-center" aria-label={t('trajectory.records.title')}>
        <div className="max-w-md">
          <span className="mx-auto grid size-9 place-items-center rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)]"><LockKeyhole className="size-4 text-[var(--nova-text-faint)]" /></span>
          <div className="mt-3 text-xs font-medium text-[var(--nova-text)]">{t('trajectory.records.unavailable')}</div>
          <p className="mt-1 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('trajectory.records.unavailableDescription')}</p>
        </div>
      </section>
    )
  }

  return (
    <section className="flex min-h-0 min-w-0 flex-1 flex-col" aria-label={t('trajectory.records.title')}>
      <div className="flex min-h-9 shrink-0 flex-wrap items-center gap-1 border-b border-[var(--nova-border)] px-2 py-1">
        <SegmentedControl values={['readable', 'debug']} active={mode} labelKey="trajectory.conversation.mode" onChange={(value) => setMode(value as ConversationMode)} />
        <div className="flex items-center gap-0.5">
          <Button type="button" size="xs" variant="ghost" className="h-6 px-1.5 text-[9px] focus-visible:ring-0" onClick={() => setExpanded(allExpandableIDs(content))}><ChevronsUpDown />{t('trajectory.ledger.expandAll')}</Button>
          <Button type="button" size="xs" variant="ghost" className="h-6 px-1.5 text-[9px] focus-visible:ring-0" onClick={() => setExpanded(new Set())}><ChevronsDownUp />{t('trajectory.ledger.collapseAll')}</Button>
          <Button type="button" size="xs" variant="ghost" className={cn('h-6 px-1.5 text-[9px] focus-visible:ring-0', wrap && 'bg-[var(--nova-active)]')} aria-pressed={wrap} onClick={() => setWrap((current) => !current)}><WrapText />{t('trajectory.conversation.wrap')}</Button>
        </div>
        <SegmentedControl values={['all', 'input', 'output']} active={scope} labelKey="trajectory.conversation.scope" onChange={(value) => setScope(value as ConversationScope)} />
        <label className="relative ml-auto min-w-40 flex-1 sm:max-w-64">
          <Search className="pointer-events-none absolute left-2 top-1/2 size-3 -translate-y-1/2 text-[var(--nova-text-faint)]" />
          <Input type="search" value={query} onChange={(event) => setQuery(event.currentTarget.value)} placeholder={t('trajectory.records.searchPlaceholder')} aria-label={t('trajectory.records.search')} className="h-6 pl-7 text-[10px] focus-visible:ring-0" />
        </label>
      </div>
      {rangeSpanIDs && <div className="shrink-0 border-b border-[var(--nova-border-soft)] bg-[var(--nova-selection-bg)] px-3 py-1 text-[10px] text-[var(--nova-text-muted)]">{t('trajectory.records.rangeFocus', { count: visibleRequests.length })}</div>}
      <div className="min-h-0 flex-1 overflow-auto bg-[var(--nova-surface-2)] p-1.5 sm:p-2">
        <div className="space-y-1.5">
          {visibleRequests.map((request) => (
            <RequestCard
              key={request.id}
              request={request}
              mode={mode}
              scope={scope}
              expanded={expanded}
              selection={selection}
              wrap={wrap}
              query={normalizedQuery}
              rangeSpanIDs={rangeSpanIDs}
              onToggle={toggle}
              onSelect={onSelect}
            />
          ))}
          {visibleRequests.length === 0 && <div className="grid h-32 place-items-center text-[11px] text-[var(--nova-text-faint)]">{t('trajectory.ledger.noMatches')}</div>}
        </div>
      </div>
    </section>
  )
}

function RequestCard({
  request,
  mode,
  scope,
  expanded,
  selection,
  wrap,
  query,
  rangeSpanIDs,
  onToggle,
  onSelect,
}: {
  request: TrajectoryRequest
  mode: ConversationMode
  scope: ConversationScope
  expanded: ReadonlySet<string>
  selection: TrajectoryContentSelection | null
  wrap: boolean
  query: string
  rangeSpanIDs: ReadonlySet<string> | null
  onToggle: (id: string) => void
  onSelect: (selection: TrajectoryContentSelection) => void
}) {
  const { t } = useTranslation()
  const id = requestKey(request)
  const open = expanded.has(id)
  const span = request.span
  const inputNodes = filterNodesByRange(request.inputNodes, rangeSpanIDs)
  const outputNodes = filterNodesByRange(request.outputNodes, rangeSpanIDs)
  const systemNodes = inputNodes.filter((node) => node.type === 'message' && node.entry.kind === 'system')
  const messageNodes = inputNodes.filter((node) => !(node.type === 'message' && node.entry.kind === 'system'))
  return (
    <article className="overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)]">
      <button type="button" aria-label={t('trajectory.records.request', { index: request.index })} aria-expanded={open} onClick={() => onToggle(id)} className="flex w-full cursor-pointer items-center gap-2 px-2.5 py-1.5 text-left hover:bg-[var(--nova-hover)] focus-visible:bg-[var(--nova-hover)] focus-visible:outline-none">
        <ChevronRight className={cn('size-3 shrink-0 text-[var(--nova-tree-chevron)] transition-transform', open && 'rotate-90')} />
        <span className="font-mono text-[9px] font-semibold uppercase tracking-[0.1em] text-[var(--nova-text)]">{t('trajectory.records.request', { index: request.index })}</span>
        <span className="min-w-0 flex-1 truncate font-mono text-[9px] text-[var(--nova-text-faint)]">{request.id}</span>
        {span && <span className="hidden shrink-0 items-center gap-3 font-mono text-[9px] text-[var(--nova-text-faint)] sm:flex">
          <span>{t('trajectory.conversation.tokensIn', { count: span.inputTokens })}</span>
          <span>{t('trajectory.conversation.tokensOut', { count: span.outputTokens })}</span>
          <span>TTFT {formatTrajectoryDuration(span.ttftMs)}</span>
          <span className="text-[var(--nova-text-muted)]">{formatTrajectoryDuration(span.durationMs)}</span>
        </span>}
      </button>
      {open && (
        <div className="space-y-2 border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-1.5">
          {(scope === 'all' || scope === 'output') && (
            <ConversationSection title={t('trajectory.conversation.output')} count={mode === 'readable' ? outputNodes.length : request.debugOutputEntries.length}>
              {mode === 'readable'
                ? <TrajectoryConversationRows nodes={outputNodes} expanded={expanded} selection={selection} wrap={wrap} query={query} onToggle={onToggle} onSelect={onSelect} />
                : <TrajectoryDebugRows entries={request.debugOutputEntries} selection={selection} query={query} onSelect={onSelect} />}
            </ConversationSection>
          )}
          {(scope === 'all' || scope === 'input') && (
            <ConversationSection title={t('trajectory.conversation.input')} count={mode === 'readable' ? inputNodes.length + 1 : request.debugInputEntries.length + 1}>
              {mode === 'readable' ? (
                <div className="space-y-0.5">
                  <TrajectoryConversationRows nodes={systemNodes} expanded={expanded} selection={selection} wrap={wrap} query={query} onToggle={onToggle} onSelect={onSelect} />
                  <TrajectoryToolDefinitionsRow id={`${id}:definitions`} tools={request.tools} expanded={expanded} query={query} onToggle={onToggle} />
                  <TrajectoryConversationRows nodes={messageNodes} expanded={expanded} selection={selection} wrap={wrap} query={query} onToggle={onToggle} onSelect={onSelect} />
                </div>
              ) : (
                <div className="space-y-0.5">
                  <TrajectoryToolDefinitionsRow id={`${id}:debug-definitions`} tools={request.tools} expanded={expanded} query={query} onToggle={onToggle} />
                  <TrajectoryDebugRows entries={request.debugInputEntries} selection={selection} query={query} onSelect={onSelect} />
                </div>
              )}
            </ConversationSection>
          )}
        </div>
      )}
    </article>
  )
}

function ConversationSection({ title, count, children }: { title: string; count: number; children: ReactNode }) {
  return (
    <section>
      <div className="mb-1 flex items-center gap-2 px-1 font-mono text-[8px] font-semibold uppercase tracking-[0.12em] text-[var(--nova-text-muted)]"><span>{title}</span><span className="text-[var(--nova-text-faint)]">{count}</span></div>
      {children}
    </section>
  )
}

function SegmentedControl({ values, active, labelKey, onChange }: { values: string[]; active: string; labelKey: string; onChange: (value: string) => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex shrink-0 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0.5">
      {values.map((value) => <Button key={value} type="button" size="xs" variant="ghost" className={cn('h-5 px-1.5 text-[9px] focus-visible:ring-0', active === value && 'bg-[var(--nova-active)]')} aria-pressed={active === value} onClick={() => onChange(value)}>{t(`${labelKey}.${value}`)}</Button>)}
    </div>
  )
}

function defaultExpanded(content: TrajectoryContentAnalysis) {
  const ids = new Set<string>()
  for (const request of content.requests) {
    ids.add(requestKey(request))
    for (const node of [...request.inputNodes, ...request.outputNodes]) if (node.type === 'tool-group') ids.add(node.id)
  }
  return ids
}

function allExpandableIDs(content: TrajectoryContentAnalysis) {
  const ids = new Set<string>()
  for (const request of content.requests) {
    const requestID = requestKey(request)
    ids.add(requestID)
    ids.add(`${requestID}:definitions`)
    ids.add(`${requestID}:debug-definitions`)
    for (const tool of request.tools) {
      ids.add(`${requestID}:definitions:${tool.name}`)
      ids.add(`${requestID}:debug-definitions:${tool.name}`)
    }
    for (const node of [...request.inputNodes, ...request.outputNodes]) {
      if (node.type === 'tool-group') ids.add(node.id)
    }
  }
  return ids
}

function filterNodesByRange(nodes: TrajectoryConversationNode[], rangeSpanIDs: ReadonlySet<string> | null) {
  if (!rangeSpanIDs) return nodes
  return nodes.flatMap((node): TrajectoryConversationNode[] => {
    if (node.type === 'message') return !node.entry.span || rangeSpanIDs.has(node.entry.span.id) ? [node] : []
    const calls = node.calls.filter((call) => !call.span || rangeSpanIDs.has(call.span.id))
    return calls.length > 0 ? [{ ...node, calls }] : []
  })
}

function requestMatches(request: TrajectoryRequest, query: string, rangeSpanIDs: ReadonlySet<string> | null) {
  const nodes = [...filterNodesByRange(request.inputNodes, rangeSpanIDs), ...filterNodesByRange(request.outputNodes, rangeSpanIDs)]
  if (!query) return nodes.length > 0 || request.tools.length > 0
  const source = JSON.stringify({
    id: request.id,
    nodes,
    tools: request.tools,
    input: request.debugInputEntries.map((entry) => entry.raw),
    output: request.debugOutputEntries.map((entry) => entry.raw),
  }).toLowerCase()
  return source.includes(query)
}

function requestKey(request: TrajectoryRequest) {
  return `request:${request.id}`
}
