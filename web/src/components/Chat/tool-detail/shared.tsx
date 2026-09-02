import type { ComponentProps, ReactNode } from 'react'
import type { TFunction } from 'i18next'
import { cn } from '@/lib/utils'
import { useToolNavigation, type ToolNavigationTarget } from '../tool-navigation'

export const detailPreClass = 'm-0 min-w-0 max-w-full whitespace-pre-wrap [overflow-wrap:anywhere]'

export interface ToolDetailRenderProps {
  input: Record<string, unknown>
  rawInput: string
  result: string
  t: TFunction
}

export type ToolDetailRenderer = (props: ToolDetailRenderProps) => ReactNode
export type ToolDetailSummarizer = (props: ToolDetailRenderProps) => string

interface SplitToolDetailAdapter {
  layout: 'input' | 'output'
  renderInput: ToolDetailRenderer
  renderOutput: ToolDetailRenderer
  summarize?: ToolDetailSummarizer
}

interface UnifiedToolDetailAdapter {
  layout: 'unified'
  render: ToolDetailRenderer
  summarize?: ToolDetailSummarizer
}

export type ToolDetailAdapter = SplitToolDetailAdapter | UnifiedToolDetailAdapter

export function DetailPre({ children, className, ...props }: { children: ReactNode; className?: string } & ComponentProps<'pre'>) {
  return <pre className={cn(detailPreClass, className)} {...props}>{children}</pre>
}

export function MetaLine({ items }: { items: Array<string | null | undefined> }) {
  const visible = items.filter((item): item is string => Boolean(item))
  if (visible.length === 0) return null
  return (
    <div className="flex min-w-0 flex-wrap gap-x-2 gap-y-0.5 text-[10px] text-[var(--nova-text-faint)]">
      {visible.map((item, index) => <span key={`${item}-${index}`}>{item}</span>)}
    </div>
  )
}

export function DetailStack({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn('space-y-2', className)}>{children}</div>
}

export function DetailBlock({ title, children, tone = 'normal' }: { title?: string; children: ReactNode; tone?: 'normal' | 'muted' | 'danger' }) {
  return (
    <div className={cn(
      'min-w-0 space-y-0.5',
      tone === 'muted' && 'text-[var(--nova-text-muted)]',
      tone === 'danger' && 'text-[var(--nova-danger)]',
    )}>
      {title ? <div className="text-[10px] text-[var(--nova-text-faint)]">{title}</div> : null}
      {children}
    </div>
  )
}

export function ToolResourceLink({ target, children, className }: { target: ToolNavigationTarget; children: ReactNode; className?: string }) {
  const navigation = useToolNavigation()
  if (!navigation) return <span className={className}>{children}</span>
  return (
    <button
      type="button"
      className={cn('min-w-0 cursor-pointer text-left font-mono text-inherit hover:text-[var(--nova-text)] hover:underline focus-visible:text-[var(--nova-text)] focus-visible:underline focus-visible:outline-none', className)}
      onClick={(event) => {
        event.stopPropagation()
        navigation.open(target)
      }}
    >
      {children}
    </button>
  )
}

export function ExternalLink({ href, children }: { href: string; children: ReactNode }) {
  if (!isHTTPURL(href)) return <span>{children}</span>
  return (
    <a
      href={href}
      target="_blank"
      rel="noreferrer"
      className="text-[var(--nova-text)] hover:underline focus-visible:underline focus-visible:outline-none"
      onClick={(event) => event.stopPropagation()}
    >
      {children}
    </a>
  )
}

export function ValuePreview({ value }: { value: unknown }) {
  return <DetailPre>{formatValue(value)}</DetailPre>
}

export function EmptyValue({ t }: { t: TFunction }) {
  return <span className="text-[var(--nova-text-faint)]">{t('chat.tool.noReturn')}</span>
}

export function parseRecord(value: string): Record<string, unknown> | null {
  try {
    return parseRecordValue(JSON.parse(value))
  } catch {
    return null
  }
}

export function parseRecordValue(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : null
}

export function recordValue(value: unknown): Record<string, unknown> {
  return parseRecordValue(value) || {}
}

export function recordArray(value: unknown) {
  return Array.isArray(value) ? value.map(parseRecordValue).filter((item): item is Record<string, unknown> => item !== null) : []
}

export function stringValue(value: unknown) {
  return typeof value === 'string' ? value : ''
}

export function stringArray(value: unknown) {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string') : []
}

export function numberValue(value: unknown) {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}

export function booleanValue(value: unknown) {
  return typeof value === 'boolean' ? value : undefined
}

export function numericMeta(name: string, value: unknown, suffix = '') {
  const number = numberValue(value)
  return number === undefined ? '' : `${name}=${number}${suffix}`
}

export function booleanMeta(name: string, value: unknown) {
  return typeof value === 'boolean' ? `${name}=${value}` : ''
}

export function fieldMeta(name: string, value: unknown) {
  const text = stringValue(value)
  return text ? `${name}=${inlinePreview(text)}` : ''
}

export function inlinePreview(value: string, limit = 48) {
  return value.length <= limit ? value : `${value.slice(0, limit - 3)}...`
}

export function formatValue(value: unknown) {
  if (typeof value === 'string') return value
  if (value === undefined || value === null) return ''
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

export function formatMaybeJSON(value: string) {
  if (!value) return ''
  try {
    return JSON.stringify(JSON.parse(value), null, 2)
  } catch {
    return value
  }
}

export function isHTTPURL(value: string) {
  return /^https?:\/\//i.test(value.trim())
}

export function splitCanonicalResult(result: string) {
  const newline = result.indexOf('\n')
  const metadataText = newline >= 0 ? result.slice(0, newline) : result
  return {
    metadata: parseRecord(metadataText),
    payload: newline >= 0 ? result.slice(newline + 1) : '',
  }
}
