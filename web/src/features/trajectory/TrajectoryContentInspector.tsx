import { useMemo, useState } from 'react'
import { ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ThemedMarkdownRenderer } from '@/components/common/MarkdownRenderer'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'
import { formatTrajectoryDuration } from './trajectory-analysis'
import type { TrajectoryContentEntry, TrajectoryToolDefinition } from './trajectory-content'
import {
  EmptyInspectorTab,
  formatExactTrajectoryTime,
  formatTrajectoryNumber,
  TrajectoryDefinitionList,
  TrajectoryJSONBlock,
  trajectoryThroughput,
} from './TrajectoryInspectorParts'

interface TrajectoryContentDetailsProps {
  entry: TrajectoryContentEntry
}

const SUMMARY_PREVIEW_CHARACTERS = 4_000

/** Inline type-aware details for exact model-visible content and diagnostics. */
export function TrajectoryContentDetails({ entry }: TrajectoryContentDetailsProps) {
  const { t } = useTranslation()
  const tabs = inspectorTabs(entry)
  return (
    <Tabs key={entry.id} defaultValue={tabs[0]} className="min-h-0 gap-0">
        <TabsList variant="line" className="h-8 w-full max-w-full shrink-0 justify-start overflow-x-auto border-b border-[var(--nova-border-soft)] px-2">
          {tabs.map((tab) => <TabsTrigger key={tab} value={tab} className="shrink-0 text-[10px]">{t(`trajectory.inspector.tab.${tab}`)}</TabsTrigger>)}
        </TabsList>
        {tabs.map((tab) => (
          <TabsContent key={tab} value={tab} className="min-h-0 overflow-auto p-3">
            <InspectorTab entry={entry} tab={tab} />
          </TabsContent>
        ))}
    </Tabs>
  )
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
        <TrajectoryDefinitionList items={items} />
        {(entry.content || entry.reasoning) && (
          <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3">
            <div className="mb-2 text-[9px] font-semibold uppercase tracking-[0.1em] text-[var(--nova-text-faint)]">{t('trajectory.inspector.preview')}</div>
            <ThemedMarkdownRenderer content={summaryPreview(entry.content || entry.reasoning)} className="text-[11px] leading-5" />
            {(entry.content || entry.reasoning).length > SUMMARY_PREVIEW_CHARACTERS && (
              <div className="mt-2 border-t border-[var(--nova-border-soft)] pt-2 text-[9px] text-[var(--nova-text-faint)]">
                {t('trajectory.inspector.previewTruncated')}
              </div>
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
  if (tab === 'result') return entry.content ? (
    <div className="space-y-3">
      <ThemedMarkdownRenderer content={entry.content} className="text-[12px] leading-6" />
      <SourceText content={entry.content} />
    </div>
  ) : <EmptyInspectorTab />
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
    <div className="space-y-1.5">
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
            className="block w-full rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-2.5 text-left hover:bg-[var(--nova-hover)]"
          >
            <span className="flex items-start gap-2">
              <ChevronRight className={cn('mt-0.5 size-3 shrink-0 text-[var(--nova-tree-chevron)] transition-transform', open && 'rotate-90')} />
              <span className="min-w-0">
                <span className="block font-mono text-[10px] font-semibold text-[var(--nova-text)]">{tool.name}</span>
                <span className="mt-0.5 block text-[10px] leading-4 text-[var(--nova-text-faint)]">{tool.description || t('trajectory.inspector.noDescription')}</span>
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

function SourceText({ content }: { content: string }) {
  return <pre className="overflow-auto whitespace-pre-wrap break-words rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3 font-mono text-[10px] leading-5 text-[var(--nova-text-muted)]">{content}</pre>
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
