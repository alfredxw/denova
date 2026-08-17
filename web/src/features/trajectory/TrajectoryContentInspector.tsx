import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { Check, ChevronRight, Clipboard, PanelRightClose, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ThemedMarkdownRenderer } from '@/components/common/MarkdownRenderer'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'
import { formatTrajectoryDuration } from './trajectory-analysis'
import type { TrajectoryContentEntry, TrajectoryToolDefinition, TrajectoryToolExchange } from './trajectory-content'
import {
  EmptyInspectorTab,
  formatExactTrajectoryTime,
  formatTrajectoryNumber,
  TrajectoryDefinitionList,
  TrajectoryJSONBlock,
  trajectoryThroughput,
} from './TrajectoryInspectorParts'

interface TrajectoryContentInspectorProps {
  entry: TrajectoryContentEntry | null
  exchange: TrajectoryToolExchange | null
  showHeader?: boolean
  variant?: 'panel' | 'inline'
  onClose: () => void
}

interface TrajectoryContentDetailsProps {
  entry: TrajectoryContentEntry
}

const SUMMARY_PREVIEW_CHARACTERS = 4_000

/** Detail content shared by the desktop pane, compact drawer, and in-ledger expansion. */
export function TrajectoryContentInspector({ entry, exchange, showHeader = true, variant = 'panel', onClose }: TrajectoryContentInspectorProps) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  const targetID = exchange?.id ?? entry?.id ?? ''
  const copyValue = useMemo(() => exchange ? {
    call: exchange.call.raw,
    result_message: exchange.result?.raw ?? null,
    tool_output: exchange.output?.raw ?? null,
  } : entry?.raw ?? null, [entry, exchange])

  useEffect(() => setCopied(false), [targetID])

  if (!entry && !exchange) {
    return <div className="grid h-full min-h-0 place-items-center px-6 text-center text-[11px] text-[var(--nova-text-faint)]">{t('trajectory.inspector.empty')}</div>
  }

  const title = exchange?.call.name || (entry ? localizedEntryLabel(entry, t) : '')
  const kind = exchange ? t('trajectory.conversation.toolCall') : t(`trajectory.records.kind.${entry?.kind}`)
  const meta = exchange
    ? `${exchange.output?.status || exchange.result?.status || exchange.span?.status || 'pending'} · ${exchange.span ? formatTrajectoryDuration(exchange.span.durationMs) : t('trajectory.conversation.noTiming')}`
    : `${t('trajectory.records.request', { index: entry?.requestIndex ?? 0 })} · ${entry?.status || '—'}`

  const copyRecord = async () => {
    try {
      await navigator.clipboard.writeText(JSON.stringify(copyValue, null, 2))
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1_600)
    } catch (error) {
      console.warn('[TrajectoryContentInspector.tsx] failed to copy trajectory content', error)
    }
  }

  const Container = variant === 'inline' ? 'section' : 'aside'
  return (
    <Container
      className={cn(
        'flex h-full min-h-0 min-w-0 flex-col bg-[var(--nova-surface-2)]',
        variant === 'inline' && 'h-[clamp(260px,48vh,440px)] border-t border-[var(--nova-border)]',
      )}
      aria-label={t('trajectory.inspector.title')}
    >
      {showHeader && (
        <div className="flex h-10 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-3">
          <span className="shrink-0 rounded-sm bg-[var(--nova-active)] px-1.5 py-0.5 font-mono text-[8px] font-semibold uppercase tracking-[0.08em] text-[var(--nova-text-muted)]">{kind}</span>
          <div className="min-w-0 flex-1">
            <div className="truncate text-[11px] font-medium text-[var(--nova-text)]">{title}</div>
            <div className="truncate font-mono text-[8px] text-[var(--nova-text-faint)]">{meta}</div>
          </div>
          <Button type="button" size="icon-xs" variant="ghost" className="focus-visible:ring-0" onClick={() => void copyRecord()} aria-label={t('trajectory.inspector.copy')}>
            {copied ? <Check className="text-[var(--nova-success)]" /> : <Clipboard />}
          </Button>
          <Button type="button" size="icon-xs" variant="ghost" className="focus-visible:ring-0" onClick={onClose} aria-label={t('trajectory.inspector.close')}>
            {variant === 'inline' ? <X /> : <PanelRightClose />}
          </Button>
        </div>
      )}
      <div className="min-h-0 flex-1">
        {exchange ? <TrajectoryToolDetails key={exchange.id} exchange={exchange} /> : <TrajectoryContentDetails key={entry?.id} entry={entry!} />}
      </div>
    </Container>
  )
}

/** Type-aware details for exact model-visible content and diagnostics. */
export function TrajectoryContentDetails({ entry }: TrajectoryContentDetailsProps) {
  const { t } = useTranslation()
  const tabs = inspectorTabs(entry)
  return (
    <Tabs key={entry.id} defaultValue={tabs[0]} className="flex h-full min-h-0 flex-col gap-0">
      <TabsList variant="line" className="h-8 w-full max-w-full shrink-0 justify-start overflow-x-auto border-b border-[var(--nova-border-soft)] px-2">
        {tabs.map((tab) => <TabsTrigger key={tab} value={tab} className="shrink-0 text-[9px] focus-visible:ring-0 focus-visible:outline-none">{t(`trajectory.inspector.tab.${tab}`)}</TabsTrigger>)}
      </TabsList>
      {tabs.map((tab) => (
        <TabsContent key={tab} value={tab} className="min-h-0 flex-1 overflow-auto p-3">
          <InspectorTab entry={entry} tab={tab} />
        </TabsContent>
      ))}
    </Tabs>
  )
}

function TrajectoryToolDetails({ exchange }: { exchange: TrajectoryToolExchange }) {
  const { t } = useTranslation()
  const tabs = ['summary', 'arguments', 'response', 'schema', 'timing', 'source'] as const
  return (
    <Tabs defaultValue="summary" className="flex h-full min-h-0 flex-col gap-0">
      <TabsList variant="line" className="h-8 w-full shrink-0 justify-start overflow-x-auto border-b border-[var(--nova-border-soft)] px-2">
        {tabs.map((tab) => (
          <TabsTrigger key={tab} value={tab} className="shrink-0 text-[9px] focus-visible:ring-0 focus-visible:outline-none">
            {tab === 'summary' ? t('trajectory.inspector.tab.summary') : t(`trajectory.conversation.tab.${tab}`)}
          </TabsTrigger>
        ))}
      </TabsList>
      {tabs.map((tab) => (
        <TabsContent key={tab} value={tab} className="min-h-0 flex-1 overflow-auto p-3">
          <ToolInspectorTab exchange={exchange} tab={tab} />
        </TabsContent>
      ))}
    </Tabs>
  )
}

function ToolInspectorTab({ exchange, tab }: { exchange: TrajectoryToolExchange; tab: string }) {
  const { t } = useTranslation()
  const response = exchange.result?.content || exchange.output?.content || exchange.output?.error || ''
  const status = exchange.output?.status || exchange.result?.status || exchange.span?.status || 'pending'
  if (tab === 'summary') {
    const items: Array<readonly [string, string]> = [
      [t('trajectory.field.kind'), t('trajectory.conversation.toolCall')],
      [t('trajectory.field.status'), status],
      [t('trajectory.conversation.callId'), exchange.call.id || exchange.output?.executionID || '—'],
      [t('trajectory.field.source'), exchange.caller ? t('trajectory.records.request', { index: exchange.caller.requestIndex }) : '—'],
    ]
    if (exchange.span) items.push(
      [t('trajectory.field.started'), formatExactTrajectoryTime(exchange.span.startedAt)],
      [t('trajectory.field.duration'), formatTrajectoryDuration(exchange.span.durationMs)],
      [t('trajectory.field.waitBefore'), formatTrajectoryDuration(exchange.span.gapBeforeMs)],
    )
    return (
      <div className="space-y-3">
        <TrajectoryDefinitionList items={items} />
        <InspectorSection title={t('trajectory.conversation.tab.arguments')}>
          <TrajectoryJSONBlock value={parseJSON(exchange.call.arguments)} className="max-h-52" />
        </InspectorSection>
        <InspectorSection title={t('trajectory.conversation.tab.response')}>
          {response ? <SourceText content={response} className="max-h-56" /> : <EmptyInspectorTab />}
        </InspectorSection>
      </div>
    )
  }
  if (tab === 'arguments') return <TrajectoryJSONBlock value={parseJSON(exchange.call.arguments)} />
  if (tab === 'response') return (
    <div className="space-y-2">
      {exchange.output?.truncated && <div className="rounded-[var(--nova-radius)] bg-[var(--nova-warning-bg)] px-2 py-1.5 text-[10px] text-[var(--nova-warning)]">{t('trajectory.conversation.resultTruncated', { returned: exchange.output.returnedBytes, original: exchange.output.originalBytes })}</div>}
      {response ? <SourceText content={response} /> : <EmptyInspectorTab />}
    </div>
  )
  if (tab === 'schema') return exchange.definition ? <TrajectoryJSONBlock value={exchange.definition.parametersError || exchange.definition.parameters} /> : <EmptyInspectorTab />
  if (tab === 'timing') return exchange.span ? <TrajectoryDefinitionList items={[
    [t('trajectory.field.status'), exchange.span.status],
    [t('trajectory.field.started'), formatExactTrajectoryTime(exchange.span.startedAt)],
    [t('trajectory.field.ended'), formatExactTrajectoryTime(exchange.span.endedAt)],
    [t('trajectory.field.duration'), formatTrajectoryDuration(exchange.span.durationMs)],
    [t('trajectory.field.waitBefore'), formatTrajectoryDuration(exchange.span.gapBeforeMs)],
  ]} /> : <EmptyInspectorTab />
  if (tab === 'source') return <TrajectoryJSONBlock value={{ call: exchange.call.raw, result_message: exchange.result?.raw ?? null, tool_output: exchange.output?.raw ?? null }} />
  return <EmptyInspectorTab />
}

function InspectorTab({ entry, tab }: { entry: TrajectoryContentEntry; tab: string }) {
  const { t } = useTranslation()
  const span = entry.span
  if (tab === 'summary') {
    const items: Array<readonly [string, string]> = [
      [t('trajectory.field.source'), `${t('trajectory.records.request', { index: entry.requestIndex })} · ${entry.requestID}`],
      [t('trajectory.field.status'), entry.status],
      [t('trajectory.field.kind'), t(`trajectory.records.kind.${entry.kind}`)],
    ]
    if (entry.kind === 'system') items.push(
      [t('trajectory.field.characters'), formatTrajectoryNumber(entry.content.length)],
      [t('trajectory.field.tools'), formatTrajectoryNumber(entry.tools.length)],
    )
    if (span) items.push(
      [t('trajectory.field.started'), formatExactTrajectoryTime(span.startedAt)],
      [t('trajectory.field.duration'), formatTrajectoryDuration(span.durationMs)],
      ['TTFT', formatTrajectoryDuration(span.ttftMs)],
      [t('trajectory.field.generation'), formatTrajectoryDuration(span.generationMs)],
      [t('trajectory.field.throughput'), trajectoryThroughput(span)],
      [t('trajectory.field.tokens'), formatTrajectoryNumber(span.inputTokens + span.outputTokens)],
    )
    return (
      <div className="space-y-3">
        <TrajectoryDefinitionList items={items} layout="wrap" />
        {(entry.content || entry.reasoning) && (
          <div className="rounded-[var(--nova-radius)] bg-[var(--nova-surface)] p-3">
            <div className="mb-2 text-[9px] font-semibold uppercase tracking-[0.1em] text-[var(--nova-text-faint)]">{t('trajectory.inspector.preview')}</div>
            <ThemedMarkdownRenderer content={summaryPreview(entry.content || entry.reasoning)} className="text-[11px] leading-5" />
            {(entry.content || entry.reasoning).length > SUMMARY_PREVIEW_CHARACTERS && (
              <div className="mt-2 border-t border-[var(--nova-border-soft)] pt-2 text-[9px] text-[var(--nova-text-faint)]">{t('trajectory.inspector.previewTruncated')}</div>
            )}
          </div>
        )}
      </div>
    )
  }
  if (tab === 'prompt') return entry.content ? <SourceText content={entry.content} /> : <EmptyInspectorTab />
  if (tab === 'tools') return <ToolDefinitions tools={entry.tools} />
  if (tab === 'preview') return entry.content ? <ThemedMarkdownRenderer content={entry.content} className="text-[12px] leading-6" /> : <EmptyInspectorTab />
  if (tab === 'raw') return <RawMessageBlocks entry={entry} />
  if (tab === 'source') return <TrajectoryJSONBlock value={entry.source} />
  if (tab === 'payload') return entry.toolCall ? <TrajectoryJSONBlock value={parseJSON(entry.toolCall.arguments)} /> : <EmptyInspectorTab />
  if (tab === 'result') return entry.content ? <SourceText content={entry.content} /> : <EmptyInspectorTab />
  if (tab === 'schema') {
    const definition = entry.tools.find((tool) => tool.name === entry.toolName)
    return definition ? <TrajectoryJSONBlock value={definition.parameters} /> : <EmptyInspectorTab />
  }
  if (tab === 'timing') return span ? (
    <TrajectoryDefinitionList items={[
      [t('trajectory.field.started'), formatExactTrajectoryTime(span.startedAt)],
      [t('trajectory.field.ended'), formatExactTrajectoryTime(span.endedAt)],
      [t('trajectory.field.duration'), formatTrajectoryDuration(span.durationMs)],
      ['TTFT', formatTrajectoryDuration(span.ttftMs)],
      [t('trajectory.field.generation'), formatTrajectoryDuration(span.generationMs)],
      [t('trajectory.field.throughput'), trajectoryThroughput(span)],
    ]} />
  ) : <EmptyInspectorTab />
  return <EmptyInspectorTab />
}

function inspectorTabs(entry: TrajectoryContentEntry) {
  if (entry.kind === 'system') return ['summary', 'prompt', 'tools', 'source']
  if (entry.kind === 'tool') return ['summary', 'payload', 'result', 'schema', 'timing', 'source']
  return ['summary', 'preview', 'raw', 'source']
}

function RawMessageBlocks({ entry }: { entry: TrajectoryContentEntry }) {
  const { t } = useTranslation()
  const blocks = [
    entry.reasoning ? { label: t('trajectory.inspector.thinking'), value: entry.reasoning } : null,
    entry.content ? { label: t('trajectory.inspector.text'), value: entry.content } : null,
    entry.toolCalls.length > 0 ? { label: t('trajectory.inspector.toolCalls'), value: JSON.stringify(entry.toolCalls, null, 2) } : null,
  ].filter((block): block is { label: string; value: string } => block !== null)
  if (blocks.length === 0) return <TrajectoryJSONBlock value={entry.raw} />
  return (
    <div className="space-y-3">
      {blocks.map((block, index) => (
        <div key={block.label}>
          <div className="mb-1.5 font-mono text-[9px] uppercase tracking-[0.08em] text-[var(--nova-text-faint)]">{t('trajectory.inspector.block', { index: index + 1 })} · {block.label}</div>
          <SourceText content={block.value} />
        </div>
      ))}
    </div>
  )
}

function ToolDefinitions({ tools }: { tools: TrajectoryToolDefinition[] }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(() => new Set())
  const sorted = useMemo(() => [...tools].sort((left, right) => left.name.localeCompare(right.name)), [tools])
  if (sorted.length === 0) return <EmptyInspectorTab />
  return (
    <div className="space-y-1">
      {sorted.map((tool) => {
        const open = expanded.has(tool.name)
        return (
          <button
            key={tool.name}
            type="button"
            aria-expanded={open}
            onClick={() => setExpanded((current) => {
              const next = new Set(current)
              if (next.has(tool.name)) next.delete(tool.name)
              else next.add(tool.name)
              return next
            })}
            className="block w-full rounded-[var(--nova-radius)] bg-[var(--nova-surface)] p-2 text-left hover:bg-[var(--nova-hover)] focus-visible:bg-[var(--nova-hover)] focus-visible:outline-none"
          >
            <span className="flex items-start gap-2">
              <ChevronRight className={cn('mt-0.5 size-3 shrink-0 text-[var(--nova-tree-chevron)] transition-transform', open && 'rotate-90')} />
              <span className="min-w-0">
                <span className="block font-mono text-[10px] font-semibold text-[var(--nova-text)]">{tool.name}</span>
                <span className="mt-0.5 block text-[9px] leading-4 text-[var(--nova-text-faint)]">{tool.description || t('trajectory.inspector.noDescription')}</span>
              </span>
            </span>
            {open && (
              <span className="mt-2 block border-t border-[var(--nova-border-soft)] pt-2">
                <span className="mb-1 block text-[9px] font-semibold uppercase tracking-[0.08em] text-[var(--nova-text-faint)]">{t('trajectory.inspector.parameters')}</span>
                <TrajectoryJSONBlock value={tool.parametersError || tool.parameters} className="max-h-80" />
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
}

function InspectorSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section>
      <div className="mb-1.5 text-[9px] font-semibold uppercase tracking-[0.08em] text-[var(--nova-text-faint)]">{title}</div>
      {children}
    </section>
  )
}

function SourceText({ content, className }: { content: string; className?: string }) {
  return <pre className={cn('overflow-auto whitespace-pre-wrap break-words rounded-[var(--nova-radius)] bg-[var(--nova-surface)] p-3 font-mono text-[10px] leading-5 text-[var(--nova-text-muted)]', className)}>{content}</pre>
}

function parseJSON(value: string) {
  if (!value) return null
  try {
    return JSON.parse(value) as unknown
  } catch {
    return value
  }
}

function summaryPreview(value: string) {
  if (value.length <= SUMMARY_PREVIEW_CHARACTERS) return value
  return `${value.slice(0, SUMMARY_PREVIEW_CHARACTERS).trimEnd()}\n\n…`
}

function localizedEntryLabel(entry: TrajectoryContentEntry, t: (key: string, options?: Record<string, unknown>) => string) {
  if (entry.kind === 'system') return t(entry.previousContent || entry.previousTools.length > 0 ? 'trajectory.records.label.systemUpdate' : 'trajectory.records.label.initialSystem')
  if (entry.kind === 'assistant') return entry.label === 'Assistant History' ? t('trajectory.records.label.assistantHistory') : t('trajectory.records.request', { index: entry.requestIndex })
  if (entry.kind === 'user') return t('trajectory.records.label.user')
  if (entry.kind === 'tool') return entry.toolName || t('trajectory.records.label.toolResult')
  return t(entry.label === 'Input Snapshot Changed' ? 'trajectory.records.label.snapshotChanged' : 'trajectory.records.label.context')
}
