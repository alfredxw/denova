import type { ReactNode } from 'react'
import { Bot, Braces, ChevronRight, Database, UserRound, Wrench } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { formatTrajectoryDuration } from './trajectory-analysis'
import type {
  TrajectoryContentEntry,
  TrajectoryContentSelection,
  TrajectoryConversationNode,
  TrajectoryToolDefinition,
  TrajectoryToolExchange,
} from './trajectory-content'
import { TrajectoryJSONBlock } from './TrajectoryInspectorParts'

interface ConversationRowsProps {
  nodes: TrajectoryConversationNode[]
  expanded: ReadonlySet<string>
  selection: TrajectoryContentSelection | null
  inlineInspector?: ReactNode
  wrap: boolean
  query: string
  onToggle: (id: string) => void
  onSelect: (selection: TrajectoryContentSelection) => void
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
    <div className="space-y-0.5">
      {visibleNodes.map((node) => node.type === 'message' ? (
        <MessageRow
          key={node.id}
          entry={node.entry}
          selected={props.selection?.type === 'entry' && props.selection.id === node.entry.id}
          inlineInspector={props.inlineInspector}
          wrap={props.wrap}
          onSelect={props.onSelect}
        />
      ) : (
        <ToolGroupRow
          key={node.id}
          node={node}
          expanded={props.expanded}
          selection={props.selection}
          inlineInspector={props.inlineInspector}
          wrap={props.wrap}
          onToggle={props.onToggle}
          onSelect={props.onSelect}
        />
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
      <button type="button" aria-label={t('trajectory.conversation.toolDefinitions')} aria-expanded={open} onClick={() => onToggle(id)} className={denseParentRowClassName}>
        <ChevronRight className={cn('size-3 shrink-0 text-[var(--nova-tree-chevron)] transition-transform', open && 'rotate-90')} />
        <RoleBadge label={t('trajectory.conversation.toolDefinitions')} />
        <span className="text-[9px] text-[var(--nova-text-faint)]">{t('trajectory.conversation.definitionCount', { count: visibleTools.length })}</span>
      </button>
      {open && (
        <div className="space-y-0.5 border-t border-[var(--nova-border-soft)] bg-[var(--nova-surface-2)] p-1">
          {visibleTools.map((tool) => {
            const toolID = `${id}:${tool.name}`
            const toolOpen = expanded.has(toolID)
            return (
              <div key={tool.name} className="overflow-hidden rounded-[var(--nova-radius)]">
                <button type="button" aria-label={`${t('trajectory.conversation.toolDefinitions')} · ${tool.name}`} aria-expanded={toolOpen} onClick={() => onToggle(toolID)} className="flex min-h-7 w-full cursor-pointer items-center gap-2 rounded-[var(--nova-radius)] px-1.5 py-1 text-left hover:bg-[var(--nova-hover)] focus-visible:bg-[var(--nova-hover)] focus-visible:outline-none">
                  <ChevronRight className={cn('size-3 shrink-0 text-[var(--nova-tree-chevron)] transition-transform', toolOpen && 'rotate-90')} />
                  <span className="rounded-sm bg-[var(--nova-active)] px-1 py-0.5 font-mono text-[8px] font-semibold uppercase text-[var(--nova-text-muted)]">fn</span>
                  <span className="flex min-w-0 items-baseline gap-2">
                    <span className="shrink-0 font-mono text-[10px] font-semibold text-[var(--nova-text)]">{tool.name}</span>
                    {tool.description && <span className="truncate text-[9px] text-[var(--nova-text-faint)]">{tool.description}</span>}
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
  selection,
  inlineInspector,
  query,
  onSelect,
}: {
  entries: TrajectoryContentEntry[]
  selection: TrajectoryContentSelection | null
  inlineInspector?: ReactNode
  query: string
  onSelect: (selection: TrajectoryContentSelection) => void
}) {
  const { t } = useTranslation()
  const visible = entries.filter((entry) => !query || entrySearchText(entry).includes(query))
  return (
    <div className="space-y-0.5">
      {visible.map((entry) => {
        const selected = selection?.type === 'entry' && selection.id === entry.id
        return (
          <div key={entry.id} className={rowContainerClassName}>
            <button
              type="button"
              aria-label={`#${entry.messageIndex + 1} · ${entry.kind}`}
              aria-current={selected ? 'true' : undefined}
              onClick={() => onSelect({ type: 'entry', id: entry.id })}
              className={cn(denseLeafRowClassName, selected && selectedLeafClassName)}
            >
              <RoleBadge label={`#${entry.messageIndex + 1} ${entry.kind}`} />
              <span className="min-w-0 truncate text-[9px] text-[var(--nova-text-faint)]">{entryPreview(entry) || t('trajectory.records.noText')}</span>
              {entry.toolCalls.length > 0 && <span className="font-mono text-[8px] text-[var(--nova-success)]">{t('trajectory.conversation.calls', { count: entry.toolCalls.length })}</span>}
            </button>
            {selected && inlineInspector}
          </div>
        )
      })}
    </div>
  )
}

function MessageRow({
  entry,
  selected,
  inlineInspector,
  wrap,
  onSelect,
}: {
  entry: TrajectoryContentEntry
  selected: boolean
  inlineInspector?: ReactNode
  wrap: boolean
  onSelect: (selection: TrajectoryContentSelection) => void
}) {
  const { t } = useTranslation()
  const Icon = contentIcon(entry.kind)
  const label = localizedEntryLabel(entry, t)
  return (
    <div className={rowContainerClassName}>
      <button
        type="button"
        aria-label={`${t(`trajectory.records.kind.${entry.kind}`)} · ${label}`}
        aria-current={selected ? 'true' : undefined}
        onClick={() => onSelect({ type: 'entry', id: entry.id })}
        className={cn(denseLeafRowClassName, wrap && 'items-start', selected && selectedLeafClassName)}
      >
        <RoleBadge icon={<Icon className="size-2.5" />} label={t(`trajectory.records.kind.${entry.kind}`)} tone={contentTone(entry.kind)} />
        <span className={cn('flex min-w-0 items-baseline gap-2', wrap && 'flex-wrap')}>
          <span className="shrink-0 text-[10px] font-medium leading-4 text-[var(--nova-text)]">{label}</span>
          <span className={cn('min-w-0 text-[9px] leading-4 text-[var(--nova-text-faint)]', wrap ? 'whitespace-pre-wrap break-words' : 'truncate')}>{entryPreview(entry) || t('trajectory.records.noText')}</span>
        </span>
        <span className="max-w-40 truncate font-mono text-[8px] text-[var(--nova-text-faint)]">{entryMeta(entry, t)}</span>
      </button>
      {selected && inlineInspector}
    </div>
  )
}

function ToolGroupRow({
  node,
  expanded,
  selection,
  inlineInspector,
  wrap,
  onToggle,
  onSelect,
}: {
  node: Extract<TrajectoryConversationNode, { type: 'tool-group' }>
  expanded: ReadonlySet<string>
  selection: TrajectoryContentSelection | null
  inlineInspector?: ReactNode
  wrap: boolean
  onToggle: (id: string) => void
  onSelect: (selection: TrajectoryContentSelection) => void
}) {
  const { t } = useTranslation()
  const open = expanded.has(node.id)
  const results = node.calls.filter((call) => call.result || call.output).length
  return (
    <div className="overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)]">
      <button type="button" aria-label={`${t('trajectory.conversation.tools')} · ${t('trajectory.conversation.toolSummary', { calls: node.calls.length, results })}`} aria-expanded={open} onClick={() => onToggle(node.id)} className={denseParentRowClassName}>
        <ChevronRight className={cn('size-3 shrink-0 text-[var(--nova-tree-chevron)] transition-transform', open && 'rotate-90')} />
        <RoleBadge icon={<Wrench className="size-2.5" />} label={t('trajectory.conversation.tools')} tone="text-[var(--nova-success)]" />
        <span className="text-[9px] text-[var(--nova-text-faint)]">{t('trajectory.conversation.toolSummary', { calls: node.calls.length, results })}</span>
      </button>
      {open && (
        <div className="border-t border-[var(--nova-border-soft)] bg-[var(--nova-surface-2)] py-1 pl-3 pr-1">
          <div className="space-y-0.5 border-l border-[var(--nova-border)] pl-1.5">
            {node.calls.map((exchange) => (
              <ToolExchangeRow
                key={exchange.id}
                exchange={exchange}
                selected={selection?.type === 'tool' && selection.id === exchange.id}
                inlineInspector={inlineInspector}
                wrap={wrap}
                onSelect={onSelect}
              />
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

function ToolExchangeRow({
  exchange,
  selected,
  inlineInspector,
  wrap,
  onSelect,
}: {
  exchange: TrajectoryToolExchange
  selected: boolean
  inlineInspector?: ReactNode
  wrap: boolean
  onSelect: (selection: TrajectoryContentSelection) => void
}) {
  const { t } = useTranslation()
  const status = exchange.output?.status || exchange.result?.status || exchange.span?.status || 'pending'
  return (
    <div className={rowContainerClassName}>
      <button
        type="button"
        aria-label={`${t('trajectory.conversation.toolCall')} · ${exchange.call.name || t('trajectory.records.label.toolResult')}`}
        aria-current={selected ? 'true' : undefined}
        onClick={() => onSelect({ type: 'tool', id: exchange.id })}
        className={cn(denseLeafRowClassName, wrap && 'items-start', selected && selectedLeafClassName)}
      >
        <RoleBadge label={t('trajectory.conversation.toolCall')} tone="text-[var(--nova-success)]" />
        <span className={cn('flex min-w-0 items-baseline gap-2', wrap && 'flex-wrap')}>
          <span className="shrink-0 font-mono text-[10px] font-semibold leading-4 text-[var(--nova-text)]">{exchange.call.name || t('trajectory.records.label.toolResult')}</span>
          <span className={cn('min-w-0 font-mono text-[8px] leading-4 text-[var(--nova-text-faint)]', wrap ? 'whitespace-pre-wrap break-words' : 'truncate')}>{callSummary(exchange.call.arguments) || exchange.call.id}</span>
        </span>
        <span className="flex items-center gap-1.5 font-mono text-[8px] text-[var(--nova-text-faint)]"><StatusDot status={status} />{exchange.span ? formatTrajectoryDuration(exchange.span.durationMs) : t('trajectory.conversation.noTiming')}</span>
      </button>
      {selected && inlineInspector}
    </div>
  )
}

const rowContainerClassName = 'overflow-hidden rounded-[var(--nova-radius)]'
const denseParentRowClassName = 'flex min-h-7 w-full cursor-pointer items-center gap-2 px-1.5 py-1 text-left hover:bg-[var(--nova-hover)] focus-visible:bg-[var(--nova-hover)] focus-visible:outline-none'
const denseLeafRowClassName = 'grid min-h-7 w-full cursor-pointer grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2 rounded-[var(--nova-radius)] px-1.5 py-1 text-left transition-colors hover:bg-[var(--nova-hover)] focus-visible:bg-[var(--nova-hover)] focus-visible:outline-none'
const selectedLeafClassName = 'bg-[var(--nova-active)] text-[var(--nova-text)]'

function RoleBadge({ icon, label, tone }: { icon?: ReactNode; label: string; tone?: string }) {
  return (
    <span className={cn('inline-flex h-[18px] min-w-14 shrink-0 items-center justify-center gap-1 rounded-[4px] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-1 font-mono text-[8px] font-semibold uppercase tracking-[0.06em] text-[var(--nova-text-muted)]', tone)}>
      {icon}{label}
    </span>
  )
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

function localizedEntryLabel(entry: TrajectoryContentEntry, t: (key: string, options?: Record<string, unknown>) => string) {
  if (entry.kind === 'system') return t(entry.previousContent || entry.previousTools.length > 0 ? 'trajectory.records.label.systemUpdate' : 'trajectory.records.label.initialSystem')
  if (entry.kind === 'assistant') return entry.label === 'Assistant History' ? t('trajectory.records.label.assistantHistory') : t('trajectory.records.request', { index: entry.requestIndex })
  if (entry.kind === 'user') return t('trajectory.records.label.user')
  if (entry.kind === 'tool') return entry.toolName || t('trajectory.records.label.toolResult')
  return t(entry.label === 'Input Snapshot Changed' ? 'trajectory.records.label.snapshotChanged' : 'trajectory.records.label.context')
}
