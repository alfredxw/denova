import { cn } from '@/lib/utils'

export interface HighlightPart {
  text: string
  matched: boolean
}

/**
 * Splits text into segments where each segment is flagged as a query match or not.
 * Matching is case-insensitive and runs left-to-right, so overlapping matches are
 * handled greedily (the earliest possible match wins at each position).
 */
export function splitByQuery(text: string, query: string): HighlightPart[] {
  const normalizedQuery = query.trim().toLocaleLowerCase()
  if (!normalizedQuery) return [{ text, matched: false }]
  const normalizedText = text.toLocaleLowerCase()
  const parts: HighlightPart[] = []
  let cursor = 0
  while (cursor < text.length) {
    const index = normalizedText.indexOf(normalizedQuery, cursor)
    if (index < 0) break
    if (index > cursor) parts.push({ text: text.slice(cursor, index), matched: false })
    parts.push({ text: text.slice(index, index + normalizedQuery.length), matched: true })
    cursor = index + normalizedQuery.length
  }
  if (cursor < text.length) parts.push({ text: text.slice(cursor), matched: false })
  return parts.length > 0 ? parts : [{ text, matched: false }]
}

export interface HighlightedTextProps {
  text: string
  query: string
  className?: string
  highlightClassName?: string
}

/**
 * Renders text with case-insensitive query matches wrapped in a `<mark>` element.
 * When the query is empty or absent the text is rendered unchanged.
 */
export function HighlightedText({ text, query, className, highlightClassName = 'rounded bg-[var(--nova-warning-bg)] px-0.5 text-[var(--nova-warning)]' }: HighlightedTextProps) {
  const parts = splitByQuery(text, query)
  return (
    <>
      {parts.map((part, index) => part.matched ? (
        <mark key={`${part.text}:${index}`} className={cn(highlightClassName)}>
          {part.text}
        </mark>
      ) : (
        <span key={`${part.text}:${index}`} className={className}>
          {part.text}
        </span>
      ))}
    </>
  )
}
