import type { ComponentProps, ReactNode } from 'react'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import { ToolContent } from '@/components/ai-elements/tool'
import { cn } from '@/lib/utils'
import type { ToolResultSeverity } from '@/lib/tool-result-envelope'

const specializedToolNames = new Set(['read', 'glob', 'grep', 'bash', 'pwsh', 'write', 'edit'])

const detailPreClass = 'm-0 min-w-0 max-w-full whitespace-pre-wrap [overflow-wrap:anywhere]'

type ToolDetailLayout = 'input' | 'output'

interface ToolCallDetailProps {
  name: string
  rawArgs: string
  result: string
  resultSeverity: ToolResultSeverity
}

export function hasSpecializedToolDetail(name: string) {
  return specializedToolNames.has(name)
}

export function ToolCallDetail({ name, rawArgs, result, resultSeverity }: ToolCallDetailProps) {
  const { t } = useTranslation()
  const input = parseRecord(rawArgs)
  const layout: ToolDetailLayout = name === 'write' || name === 'edit' ? 'input' : 'output'
  const hasResult = result.length > 0
  const outputTone = resultSeverity === 'error'
    ? 'text-[var(--nova-danger)]'
    : resultSeverity === 'warning'
      ? 'text-[var(--nova-warning)]'
      : 'text-[var(--nova-accent-green)]'

  return (
    <ToolContent
      data-nova-tool-detail={name}
      className="max-h-48 min-w-0 max-w-full space-y-0 overflow-x-hidden overflow-y-hidden border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 font-mono text-[11px] leading-relaxed"
    >
      <div className="flex max-h-48 min-h-0 flex-col">
        <section
          data-nova-tool-detail-input
          className={cn(
            'min-w-0 px-3 py-2.5 text-[var(--nova-text-muted)]',
            layout === 'input'
              ? 'min-h-0 flex-1 overflow-y-auto'
              : hasResult
                ? 'max-h-20 shrink-0 overflow-y-auto'
                : 'max-h-48 overflow-y-auto',
          )}
        >
          {renderToolInput(name, input, rawArgs, t)}
        </section>
        {hasResult && (
          <section
            data-nova-tool-detail-output
            className={cn(
              'min-w-0 border-t border-[var(--nova-border)] px-3 py-2.5',
              layout === 'input'
                ? 'max-h-20 shrink-0 overflow-y-auto'
                : 'min-h-0 flex-1 overflow-y-auto',
              outputTone,
            )}
          >
            {renderToolOutput(name, result, t)}
          </section>
        )}
      </div>
    </ToolContent>
  )
}

function renderToolInput(name: string, input: Record<string, unknown> | null, rawArgs: string, t: TFunction): ReactNode {
  if (!input) return <DetailPre>{formatMaybeJSON(rawArgs)}</DetailPre>

  switch (name) {
    case 'read':
      return <PathInput path={stringValue(input.path) || rawArgs} meta={readMeta(input)} />
    case 'glob':
      return <GlobInput input={input} />
    case 'grep':
      return <CommandInput command={stringValue(input.command) || rawArgs} meta={grepMeta(input)} />
    case 'bash':
    case 'pwsh':
      return <CommandInput command={stringValue(input.command) || rawArgs} meta={shellMeta(input)} />
    case 'write':
      return <WriteInput input={input} t={t} />
    case 'edit':
      return <EditInput input={input} t={t} />
  }
}

function renderToolOutput(name: string, result: string, t: TFunction): ReactNode {
  switch (name) {
    case 'read':
      return <ReadOutput result={result} t={t} />
    case 'glob':
    case 'grep':
      return <CanonicalOutput result={result} schema="workspace.search.v1" t={t} />
    case 'bash':
    case 'pwsh':
      return <CanonicalOutput result={result} schema="process.result.v1" t={t} />
    case 'write':
    case 'edit':
      return <MutationOutput result={result} t={t} />
  }
}

function ReadOutput({ result, t }: { result: string; t: TFunction }) {
  const parts = splitCanonicalResult(result)
  if (parts.metadata?.schema !== 'resource.read.v1') return <DetailPre>{formatMaybeJSON(result)}</DetailPre>

  const recovery = recordValue(parts.metadata.recovery)
  const content = parts.payload.replace(/\n+$/, '') || stringValue(recovery.suggestion) || t('chat.tool.noReturn')
  const lines = parseReadLines(content)
  if (!lines) return <DetailPre>{content}</DetailPre>

  return (
    <div data-nova-read-output className="grid min-w-0 grid-cols-[max-content_minmax(0,1fr)] gap-x-2 text-[var(--nova-text)]">
      {lines.map(line => (
        <div key={line.number} className="contents">
          <span
            aria-hidden
            data-nova-read-line-number
            className="select-none text-right text-[var(--nova-text-faint)]"
          >
            {line.number}
          </span>
          <DetailPre data-nova-read-line-content>{line.content}</DetailPre>
        </div>
      ))}
    </div>
  )
}

function PathInput({ path, meta }: { path: string; meta: string[] }) {
  return (
    <div className="space-y-1.5">
      <DetailPre className="text-[var(--nova-text)]">{path}</DetailPre>
      <MetaLine items={meta} />
    </div>
  )
}

function GlobInput({ input }: { input: Record<string, unknown> }) {
  const paths = stringArray(input.paths)
  return (
    <div className="space-y-1.5">
      <DetailPre className="text-[var(--nova-text)]">{paths.length > 0 ? paths.join('\n') : '.'}</DetailPre>
      <MetaLine items={globMeta(input)} />
    </div>
  )
}

function CommandInput({ command, meta }: { command: string; meta: string[] }) {
  return (
    <div className="space-y-1.5">
      <DetailPre className="text-[var(--nova-text)]">{command}</DetailPre>
      <MetaLine items={meta} />
    </div>
  )
}

function WriteInput({ input, t }: { input: Record<string, unknown>; t: TFunction }) {
  const content = stringValue(input.content)
  return (
    <div className="space-y-2">
      <DetailPre className="border-b border-[var(--nova-border)] pb-2 text-[var(--nova-text)]">
        {stringValue(input.path)}
      </DetailPre>
      <DetailPre className={content ? undefined : 'text-[var(--nova-text-faint)]'}>
        {content || t('chat.tool.emptyContent')}
      </DetailPre>
    </div>
  )
}

function EditInput({ input, t }: { input: Record<string, unknown>; t: TFunction }) {
  const operation = stringValue(input.operation)
  const edits = Array.isArray(input.edits) ? input.edits.map(parseRecordValue).filter(item => item !== null) : []

  return (
    <div className="space-y-2">
      <div className="flex min-w-0 flex-wrap items-baseline gap-x-2 border-b border-[var(--nova-border)] pb-2">
        <span className="min-w-0 break-all text-[var(--nova-text)]">{stringValue(input.path)}</span>
        {operation === 'delete' && <span className="text-[10px] text-[var(--nova-danger)]">{t('chat.tool.edit.delete')}</span>}
      </div>
      {edits.map((edit, index) => (
        <div key={index} className={cn('space-y-1.5', index > 0 && 'border-t border-[var(--nova-border)] pt-2')}>
          <div className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-2 border-l-2 border-red-500/45 bg-red-500/5 px-2 py-1.5 text-[var(--nova-danger)]">
            <span aria-hidden>−</span>
            <DetailPre>{stringValue(edit.old_string)}</DetailPre>
          </div>
          <div className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-2 border-l-2 border-emerald-500/45 bg-emerald-500/5 px-2 py-1.5 text-[var(--nova-accent-green)]">
            <span aria-hidden>+</span>
            <DetailPre>{stringValue(edit.new_string)}</DetailPre>
          </div>
          {edit.replace_all === true && (
            <div className="text-[10px] text-[var(--nova-text-faint)]">{t('chat.tool.edit.replaceAll')}</div>
          )}
        </div>
      ))}
    </div>
  )
}

function CanonicalOutput({ result, schema, t }: { result: string; schema: string; t: TFunction }) {
  const parts = splitCanonicalResult(result)
  if (parts.metadata?.schema !== schema) return <DetailPre>{formatMaybeJSON(result)}</DetailPre>

  const recovery = recordValue(parts.metadata.recovery)
  let content = parts.payload.trimEnd() || stringValue(recovery.suggestion) || t('chat.tool.noReturn')
  if (content === '[Command executed successfully with no output]') content = t('chat.tool.noReturn')
  return <DetailPre>{content}</DetailPre>
}

function MutationOutput({ result, t }: { result: string; t: TFunction }) {
  const receipt = parseRecord(result)
  const schema = stringValue(receipt?.schema)
  if (schema !== 'workspace_change.tool_result.v1' && schema !== 'workspace_change.tool_noop.v1') {
    return <DetailPre>{formatMaybeJSON(result)}</DetailPre>
  }

  if (schema === 'workspace_change.tool_noop.v1') {
    return <span>{t('chat.tool.change.unchanged')}</span>
  }

  const stats = recordValue(receipt?.file_stats)
  const details = [
    t('chat.tool.change.applied'),
    numberValue(stats.lines) === undefined ? '' : t('chat.tool.change.lines', { count: numberValue(stats.lines) }),
    numberValue(stats.characters) === undefined ? '' : t('chat.tool.change.characters', { count: numberValue(stats.characters) }),
  ].filter(Boolean)
  return <span>{details.join(' · ')}</span>
}

function MetaLine({ items }: { items: string[] }) {
  if (items.length === 0) return null
  return <div className="flex min-w-0 flex-wrap gap-x-2 gap-y-0.5 text-[10px] text-[var(--nova-text-faint)]">{items.map(item => <span key={item}>{item}</span>)}</div>
}

function DetailPre({ children, className, ...props }: { children: ReactNode; className?: string } & ComponentProps<'pre'>) {
  return <pre className={cn(detailPreClass, className)} {...props}>{children}</pre>
}

function parseReadLines(content: string) {
  const parsed: Array<{ number: string; content: string }> = []
  for (const line of content.split('\n')) {
    const separator = line.indexOf('\t')
    if (separator < 1 || !/^\d+$/.test(line.slice(0, separator))) return null
    parsed.push({ number: line.slice(0, separator), content: line.slice(separator + 1) })
  }
  return parsed
}

function readMeta(input: Record<string, unknown>) {
  return [
    numericMeta('offset', input.offset),
    numericMeta('byte_offset', input.byte_offset),
    numericMeta('limit', input.limit),
    numericMeta('depth', input.depth),
    booleanMeta('hidden', input.hidden),
  ].filter(Boolean)
}

function globMeta(input: Record<string, unknown>) {
  return [
    booleanMeta('hidden', input.hidden),
    booleanMeta('gitignore', input.gitignore),
    numericMeta('limit', input.limit),
    stringValue(input.cursor) ? `cursor=${inlinePreview(stringValue(input.cursor))}` : '',
  ].filter(Boolean)
}

function grepMeta(input: Record<string, unknown>) {
  const cursor = stringValue(input.cursor)
  return cursor ? [`cursor=${inlinePreview(cursor)}`] : []
}

function shellMeta(input: Record<string, unknown>) {
  const env = recordValue(input.env)
  const envText = Object.entries(env).map(([key, value]) => `${key}=${String(value)}`).join(' ')
  return [
    stringValue(input.cwd) ? `cwd=${stringValue(input.cwd)}` : '',
    input.pty === true ? 'PTY' : '',
    numericMeta('timeout', input.timeout_seconds, 's'),
    envText ? `env ${envText}` : '',
  ].filter(Boolean)
}

function splitCanonicalResult(result: string) {
  const newline = result.indexOf('\n')
  const metadataText = newline >= 0 ? result.slice(0, newline) : result
  return {
    metadata: parseRecord(metadataText),
    payload: newline >= 0 ? result.slice(newline + 1) : '',
  }
}

function parseRecord(value: string): Record<string, unknown> | null {
  try {
    return parseRecordValue(JSON.parse(value))
  } catch {
    return null
  }
}

function parseRecordValue(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : null
}

function recordValue(value: unknown): Record<string, unknown> {
  return parseRecordValue(value) || {}
}

function stringValue(value: unknown) {
  return typeof value === 'string' ? value : ''
}

function stringArray(value: unknown) {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string') : []
}

function numberValue(value: unknown) {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}

function numericMeta(name: string, value: unknown, suffix = '') {
  const number = numberValue(value)
  return number === undefined ? '' : `${name}=${number}${suffix}`
}

function booleanMeta(name: string, value: unknown) {
  return typeof value === 'boolean' ? `${name}=${value}` : ''
}

function inlinePreview(value: string) {
  return value.length <= 48 ? value : `${value.slice(0, 45)}...`
}

export function formatMaybeJSON(value: string) {
  if (!value) return ''
  try {
    return JSON.stringify(JSON.parse(value), null, 2)
  } catch {
    return value
  }
}
