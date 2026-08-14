import { useState } from 'react'
import { Bot, Braces, Check, ChevronRight, Clipboard, Database, UserRound, Wrench } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'
import { formatTrajectoryDuration } from './trajectory-analysis'
import type {
  TrajectoryContentEntry,
  TrajectoryConversationNode,
  TrajectoryToolDefinition,
  TrajectoryToolExchange,
} from './trajectory-content'
import { TrajectoryContentDetails } from './TrajectoryContentInspector'
import {
  EmptyInspectorTab,
  formatExactTrajectoryTime,
  TrajectoryDefinitionList,
  TrajectoryJSONBlock,
} from './TrajectoryInspectorParts'

interface ConversationRowsProps {
  nodes: TrajectoryConversationNode[]
  expanded: ReadonlySet<string>
  selectedEntryID: string
  wrap: boolean
  query: string
  onToggle: (id: string) => void
  onSelect: (entryID: string) => void
}

export function TrajectoryConversationRows(props: ConversationRowsProps) {
  const visibleNodes = props.nodes.flatMap((node): TrajectoryConversationNode[] => {
    if (!props.query) return [node]
    if (node.type === 'message') return entrySearchText(node.entry).includes(props.query) ? [node] : []
    const calls = node.calls.filter((call) => toolSearchText(call).includes(props.query))
    return calls.length > 0 ? [{ ...node, calls }] : []
  })
  if (visibleNodes.length === 0) return null
  return (
    <div className="space-y-1.5">
      {visibleNodes.map((node) => node.type === 'message' ? (
        <MessageRow key={node.id} entry={node.entry} open={props.expanded.has(node.id)} selected={props.selectedEntryID === node.entry.id} wrap={props.wrap} onToggle={props.onToggle} onSelect={props.onSelect} />
      ) : (
        <ToolGroupRow key={node.id} node={node} expanded={props.expanded} selectedEntryID={props.selectedEntryID} wrap={props.wrap} onToggle={props.onToggle} />
      ))}
    </div>
  )
}

export function TrajectoryToolDefinitionsRow({
  id,
  tools,
  expanded,
  query,
  onToggle,
}: {
  id: string
  tools: TrajectoryToolDefinition[]
  expanded: ReadonlySet<string>
  query: string
  onToggle: (id: string) => void
}) {
  const { t } = useTranslation()
  const visibleTools = tools.filter((tool) => !query || `${tool.name} ${tool.description} ${JSON.stringify(tool.parameters)}`.toLowerCase().includes(query))
  if (visibleTools.length === 0) return null
  const open = expanded.has(id)
  return (
    <div className="overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)]">
      <button type="button" aria-label={t('trajectory.conversation.toolDefinitions')} aria-expanded={open} onClick={() => onToggle(id)} className="flex w-full cursor-pointer items-center gap-2 px-2.5 py-2 text-left text-[11px] hover:bg-[var(--nova-hover)]">
        <ChevronRight className={cn('size-3 shrink-0 text-[var(--nova-tree-chevron)] transition-transform', open && 'rotate-90')} />
        <span className="font-mono text-[9px] font-semibold uppercase tracking-[0.1em] text-[var(--nova-text-muted)]">{t('trajectory.conversation.toolDefinitions')}</span>
        <span className="text-[10px] text-[var(--nova-text-faint)]">{t('trajectory.conversation.definitionCount', { count: visibleTools.length })}</span>
      </button>
      {open && (
        <div className="space-y-1 border-t border-[var(--nova-border-soft)] bg-[var(--nova-surface-2)] p-1.5">
          {visibleTools.map((tool) => {
            const toolID = `${id}:${tool.name}`
            const toolOpen = expanded.has(toolID)
            return (
              <div key={tool.name} className="overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border-soft)] bg-[var(--nova-surface)]">
                <button type="button" aria-label={`${t('trajectory.conversation.toolDefinitions')} · ${tool.name}`} aria-expanded={toolOpen} onClick={() => onToggle(toolID)} className="flex w-full cursor-pointer items-start gap-2 px-2.5 py-2 text-left hover:bg-[var(--nova-hover)]">
                  <ChevronRight className={cn('mt-0.5 size-3 shrink-0 text-[var(--nova-tree-chevron)] transition-transform', toolOpen && 'rotate-90')} />
                  <span className="rounded-sm bg-[var(--nova-active)] px-1 py-0.5 font-mono text-[8px] font-semibold uppercase text-[var(--nova-text-muted)]">fn</span>
                  <span className="min-w-0">
                    <span className="block font-mono text-[10px] font-semibold text-[var(--nova-text)]">{tool.name}</span>
                    {tool.description && <span className="mt-0.5 block text-[10px] leading-4 text-[var(--nova-text-faint)]">{tool.description}</span>}
                  </span>
                </button>
                {toolOpen && <div className="border-t border-[var(--nova-border-soft)] p-2"><TrajectoryJSONBlock value={tool.parametersError || tool.parameters} className="max-h-80" /></div>}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

export function TrajectoryDebugRows({
  entries,
  expanded,
  query,
  onToggle,
}: {
  entries: TrajectoryContentEntry[]
  expanded: ReadonlySet<string>
  query: string
  onToggle: (id: string) => void
}) {
  const { t } = useTranslation()
  const visible = entries.filter((entry) => !query || entrySearchText(entry).includes(query))
  return (
    <div className="space-y-1.5">
      {visible.map((entry) => {
        const open = expanded.has(entry.id)
        return (
          <article key={entry.id} className="overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)]">
            <button type="button" aria-label={`#${entry.messageIndex + 1} · ${entry.kind}`} aria-expanded={open} onClick={() => onToggle(entry.id)} className="flex w-full cursor-pointer items-center gap-2 px-2.5 py-2 text-left hover:bg-[var(--nova-hover)]">
              <ChevronRight className={cn('size-3 shrink-0 text-[var(--nova-tree-chevron)] transition-transform', open && 'rotate-90')} />
              <span className="font-mono text-[9px] font-semibold uppercase tracking-[0.08em] text-[var(--nova-text-muted)]">#{entry.messageIndex + 1} {entry.kind}</span>
              <span className="min-w-0 flex-1 truncate text-[10px] text-[var(--nova-text-faint)]">{entryPreview(entry) || t('trajectory.records.noText')}</span>
              {entry.toolCalls.length > 0 && <span className="font-mono text-[9px] text-[var(--nova-success)]">{t('trajectory.conversation.calls', { count: entry.toolCalls.length })}</span>}
            </button>
            {open && <div className="border-t border-[var(--nova-border-soft)] p-2"><TrajectoryJSONBlock value={entry.raw} className="max-h-[32rem]" /></div>}
          </article>
        )
      })}
    </div>
  )
}

function MessageRow({
  entry,
  open,
  selected,
  wrap,
  onToggle,
  onSelect,
}: {
  entry: TrajectoryContentEntry
  open: boolean
  selected: boolean
  wrap: boolean
  onToggle: (id: string) => void
  onSelect: (entryID: string) => void
}) {
  const { t } = useTranslation()
  const Icon = contentIcon(entry.kind)
  const label = localizedEntryLabel(entry, t)
  return (
    <article className={cn('overflow-hidden rounded-[var(--nova-radius)] border bg-[var(--nova-surface)]', selected ? 'border-[var(--nova-border-strong)] ring-1 ring-[var(--nova-border-strong)]' : 'border-[var(--nova-border)]')}>
      <button
        type="button"
        aria-label={`${t(`trajectory.records.kind.${entry.kind}`)} · ${label}`}
        aria-expanded={open}
        onClick={() => { if (!open) onSelect(entry.id); onToggle(entry.id) }}
        className="grid w-full cursor-pointer grid-cols-[auto_72px_minmax(0,1fr)_auto] items-start gap-2 px-2.5 py-2 text-left hover:bg-[var(--nova-hover)]"
      >
        <ChevronRight className={cn('mt-0.5 size-3 shrink-0 text-[var(--nova-tree-chevron)] transition-transform', open && 'rotate-90')} />
        <span className={cn('inline-flex items-center gap-1 font-mono text-[8px] font-semibold uppercase tracking-[0.08em]', contentTone(entry.kind))}><Icon className="size-3" />{t(`trajectory.records.kind.${entry.kind}`)}</span>
        <span className="min-w-0">
          <span className="block text-[11px] font-medium text-[var(--nova-text)]">{label}</span>
          <span className={cn('mt-0.5 block text-[10px] leading-4 text-[var(--nova-text-faint)]', wrap ? 'whitespace-pre-wrap break-words' : 'truncate')}>{entryPreview(entry) || t('trajectory.records.noText')}</span>
        </span>
        <span className="max-w-48 truncate pt-0.5 font-mono text-[9px] text-[var(--nova-text-faint)]">{entryMeta(entry, t)}</span>
      </button>
      {open && <div className="border-t border-[var(--nova-border-soft)] bg-[var(--nova-surface-2)]"><TrajectoryContentDetails entry={entry} /></div>}
    </article>
  )
}

function ToolGroupRow({
  node,
  expanded,
  selectedEntryID,
  wrap,
  onToggle,
}: {
  node: Extract<TrajectoryConversationNode, { type: 'tool-group' }>
  expanded: ReadonlySet<string>
  selectedEntryID: string
  wrap: boolean
  onToggle: (id: string) => void
}) {
  const { t } = useTranslation()
  const open = expanded.has(node.id)
  const results = node.calls.filter((call) => call.result || call.output).length
  return (
    <div className="overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)]">
      <button type="button" aria-label={`${t('trajectory.conversation.tools')} · ${t('trajectory.conversation.toolSummary', { calls: node.calls.length, results })}`} aria-expanded={open} onClick={() => onToggle(node.id)} className="flex w-full cursor-pointer items-center gap-2 px-2.5 py-2 text-left hover:bg-[var(--nova-hover)]">
        <ChevronRight className={cn('size-3 shrink-0 text-[var(--nova-tree-chevron)] transition-transform', open && 'rotate-90')} />
        <span className="inline-flex items-center gap-1 font-mono text-[8px] font-semibold uppercase tracking-[0.1em] text-[var(--nova-success)]"><Wrench className="size-3" />{t('trajectory.conversation.tools')}</span>
        <span className="text-[10px] text-[var(--nova-text-faint)]">{t('trajectory.conversation.toolSummary', { calls: node.calls.length, results })}</span>
      </button>
      {open && (
        <div className="space-y-1.5 border-t border-[var(--nova-border-soft)] bg-[var(--nova-surface-2)] py-1.5 pl-3 pr-1.5">
          <div className="space-y-1.5 border-l border-[var(--nova-border-strong)] pl-2">
            {node.calls.map((exchange) => (
              <ToolExchangeRow key={exchange.id} exchange={exchange} open={expanded.has(exchange.id)} selected={selectedEntryID === exchange.caller?.id || selectedEntryID === exchange.result?.id} wrap={wrap} onToggle={onToggle} />
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

function ToolExchangeRow({
  exchange,
  open,
  selected,
  wrap,
  onToggle,
}: {
  exchange: TrajectoryToolExchange
  open: boolean
  selected: boolean
  wrap: boolean
  onToggle: (id: string) => void
}) {
  const { t } = useTranslation()
  const status = exchange.output?.status || exchange.result?.status || exchange.span?.status || 'pending'
  return (
    <article className={cn('overflow-hidden rounded-[var(--nova-radius)] border bg-[var(--nova-surface)]', selected ? 'border-[var(--nova-border-strong)]' : 'border-[var(--nova-border-soft)]')}>
      <button type="button" aria-label={`${t('trajectory.conversation.toolCall')} · ${exchange.call.name || t('trajectory.records.label.toolResult')}`} aria-expanded={open} onClick={() => onToggle(exchange.id)} className="grid w-full cursor-pointer grid-cols-[auto_72px_minmax(0,1fr)_auto] items-start gap-2 px-2.5 py-2 text-left hover:bg-[var(--nova-hover)]">
        <ChevronRight className={cn('mt-0.5 size-3 text-[var(--nova-tree-chevron)] transition-transform', open && 'rotate-90')} />
        <span className="font-mono text-[8px] font-semibold uppercase tracking-[0.08em] text-[var(--nova-success)]">{t('trajectory.conversation.toolCall')}</span>
        <span className="min-w-0">
          <span className="block font-mono text-[10px] font-semibold text-[var(--nova-text)]">{exchange.call.name || t('trajectory.records.label.toolResult')}</span>
          <span className={cn('mt-0.5 block font-mono text-[9px] leading-4 text-[var(--nova-text-faint)]', wrap ? 'whitespace-pre-wrap break-words' : 'truncate')}>{callSummary(exchange.call.arguments) || exchange.call.id}</span>
        </span>
        <span className="flex items-center gap-2 pt-0.5 font-mono text-[9px] text-[var(--nova-text-faint)]"><StatusDot status={status} />{exchange.span ? formatTrajectoryDuration(exchange.span.durationMs) : t('trajectory.conversation.noTiming')}</span>
      </button>
      {open && <ToolExchangeDetails exchange={exchange} />}
    </article>
  )
}

function ToolExchangeDetails({ exchange }: { exchange: TrajectoryToolExchange }) {
  const { t } = useTranslation()
  const response = exchange.result?.content || exchange.output?.content || exchange.output?.error || ''
  const tabs = ['arguments', 'response', 'schema', 'timing', 'source'] as const
  return (
    <div className="border-t border-[var(--nova-border-soft)] bg-[var(--nova-surface-2)]">
      <div className="flex items-center gap-2 border-b border-[var(--nova-border-soft)] px-3 py-1.5 font-mono text-[9px] text-[var(--nova-text-faint)]">
        <span>{t('trajectory.conversation.callId')}</span><span className="min-w-0 select-all truncate text-[var(--nova-text-muted)]">{exchange.call.id || exchange.output?.executionID || '—'}</span>
        <CopyJSONButton value={{ call: exchange.call.raw, result: exchange.result?.raw, output: exchange.output?.raw }} />
      </div>
      <Tabs defaultValue="arguments" className="gap-0">
        <TabsList variant="line" className="h-8 w-full justify-start overflow-x-auto border-b border-[var(--nova-border-soft)] px-2">
          {tabs.map((tab) => <TabsTrigger key={tab} value={tab} className="text-[10px]">{t(`trajectory.conversation.tab.${tab}`)}</TabsTrigger>)}
        </TabsList>
        <TabsContent value="arguments" className="p-3"><TrajectoryJSONBlock value={parseJSON(exchange.call.arguments)} className="max-h-80" /></TabsContent>
        <TabsContent value="response" className="p-3">
          {exchange.output?.truncated && <div className="mb-2 rounded-[var(--nova-radius)] border border-[var(--nova-warning)] bg-[var(--nova-warning-bg)] px-2 py-1.5 text-[10px] text-[var(--nova-warning)]">{t('trajectory.conversation.resultTruncated', { returned: exchange.output.returnedBytes, original: exchange.output.originalBytes })}</div>}
          {response ? <pre className="max-h-80 overflow-auto whitespace-pre-wrap break-words rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3 font-mono text-[10px] leading-5 text-[var(--nova-text-muted)]">{response}</pre> : <div className="py-8 text-center text-[11px] text-[var(--nova-text-faint)]">{t('trajectory.conversation.resultPending')}</div>}
        </TabsContent>
        <TabsContent value="schema" className="p-3">{exchange.definition ? <TrajectoryJSONBlock value={exchange.definition.parametersError || exchange.definition.parameters} className="max-h-80" /> : <EmptyInspectorTab />}</TabsContent>
        <TabsContent value="timing" className="p-3">{exchange.span ? <TrajectoryDefinitionList items={[
          [t('trajectory.field.status'), exchange.span.status],
          [t('trajectory.field.started'), formatExactTrajectoryTime(exchange.span.startedAt)],
          [t('trajectory.field.ended'), formatExactTrajectoryTime(exchange.span.endedAt)],
          [t('trajectory.field.duration'), formatTrajectoryDuration(exchange.span.durationMs)],
          [t('trajectory.field.waitBefore'), formatTrajectoryDuration(exchange.span.gapBeforeMs)],
        ]} /> : <EmptyInspectorTab />}</TabsContent>
        <TabsContent value="source" className="p-3"><TrajectoryJSONBlock value={{ call: exchange.call.raw, result_message: exchange.result?.raw ?? null, tool_output: exchange.output?.raw ?? null }} className="max-h-[32rem]" /></TabsContent>
      </Tabs>
    </div>
  )
}

function CopyJSONButton({ value }: { value: unknown }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(JSON.stringify(value, null, 2))
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1_600)
    } catch (error) {
      console.warn('[TrajectoryConversationRows.tsx] failed to copy tool exchange', error)
    }
  }
  return <Button type="button" size="icon-xs" variant="ghost" className="ml-auto" onClick={() => void copy()} aria-label={t('trajectory.inspector.copy')}>{copied ? <Check className="text-[var(--nova-success)]" /> : <Clipboard />}</Button>
}

function StatusDot({ status }: { status: string }) {
  const error = ['error', 'failed', 'blocked', 'aborted'].includes(status.toLowerCase())
  const pending = status === 'pending'
  return <span className={cn('size-1.5 rounded-full', error ? 'bg-[var(--nova-danger)]' : pending ? 'bg-[var(--nova-warning)]' : 'bg-[var(--nova-success)]')} />
}

function contentIcon(kind: TrajectoryContentEntry['kind']) {
  if (kind === 'system') return Braces
  if (kind === 'user') return UserRound
  if (kind === 'context') return Database
  if (kind === 'tool') return Wrench
  return Bot
}

function contentTone(kind: TrajectoryContentEntry['kind']) {
  if (kind === 'tool') return 'text-[var(--nova-success)]'
  if (kind === 'context') return 'text-[var(--nova-warning)]'
  return 'text-[var(--nova-text-muted)]'
}

function entryMeta(entry: TrajectoryContentEntry, t: (key: string, options?: Record<string, unknown>) => string) {
  if (entry.kind === 'system') return t('trajectory.records.toolsCount', { count: entry.tools.length })
  if (entry.kind === 'assistant' && entry.span) return t('trajectory.records.assistantMeta', { tokens: entry.span.outputTokens, duration: formatTrajectoryDuration(entry.span.durationMs) })
  if (entry.kind === 'tool') return entry.toolCallID || entry.toolName
  return t('trajectory.conversation.characters', { count: entry.content.length })
}

function entryPreview(entry: TrajectoryContentEntry) {
  if (entry.content) return entry.content.replaceAll(/\s+/g, ' ').trim()
  if (entry.reasoning) return entry.reasoning.replaceAll(/\s+/g, ' ').trim()
  if (entry.toolCalls.length > 0) return entry.toolCalls.map((call) => call.name).join(', ')
  if (entry.kind === 'system') return entry.tools.map((tool) => tool.name).join(', ')
  return ''
}

function entrySearchText(entry: TrajectoryContentEntry) {
  return `${entry.kind} ${entry.label} ${entry.content} ${entry.reasoning} ${entry.toolName} ${entry.toolCalls.map((call) => `${call.name} ${call.arguments}`).join(' ')} ${entry.tools.map((tool) => tool.name).join(' ')}`.toLowerCase()
}

function toolSearchText(exchange: TrajectoryToolExchange) {
  return `${exchange.call.id} ${exchange.call.name} ${exchange.call.arguments} ${exchange.result?.content ?? ''} ${exchange.output?.content ?? ''}`.toLowerCase()
}

function callSummary(argumentsValue: string) {
  if (!argumentsValue) return ''
  try {
    const value = JSON.parse(argumentsValue) as unknown
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      const pairs = Object.entries(value as Record<string, unknown>).slice(0, 3).map(([key, item]) => `${key}=${compactValue(item)}`)
      if (pairs.length > 0) return pairs.join(' · ')
    }
  } catch {
    // The raw argument string remains useful when a provider emitted invalid JSON.
  }
  return argumentsValue.replaceAll(/\s+/g, ' ').trim()
}

function compactValue(value: unknown) {
  const encoded = typeof value === 'string' ? value : JSON.stringify(value) ?? String(value)
  return encoded.length > 56 ? `${encoded.slice(0, 53)}…` : encoded
}

function parseJSON(value: string) {
  if (!value) return null
  try {
    return JSON.parse(value) as unknown
  } catch {
    return value
  }
}

function localizedEntryLabel(entry: TrajectoryContentEntry, t: (key: string, options?: Record<string, unknown>) => string) {
  if (entry.kind === 'system') return t(entry.previousContent || entry.previousTools.length > 0 ? 'trajectory.records.label.systemUpdate' : 'trajectory.records.label.initialSystem')
  if (entry.kind === 'assistant') return entry.label === 'Assistant History' ? t('trajectory.records.label.assistantHistory') : t('trajectory.records.request', { index: entry.requestIndex })
  if (entry.kind === 'user') return t('trajectory.records.label.user')
  if (entry.kind === 'tool') return entry.toolName || t('trajectory.records.label.toolResult')
  return t(entry.label === 'Input Snapshot Changed' ? 'trajectory.records.label.snapshotChanged' : 'trajectory.records.label.context')
}
