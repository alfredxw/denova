import { useEffect, useMemo, useRef, useState } from 'react'
import { FileText, Loader2, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { searchWorkspace, type WorkspaceSearchResult } from '@/lib/api'

interface SearchPanelProps {
  workspace: string
  onSelectResult: (result: WorkspaceSearchResult, query: string) => void | Promise<void>
}

interface SearchResultGroup {
  path: string
  results: WorkspaceSearchResult[]
}

const SEARCH_LIMIT = 100
const SEARCH_DEBOUNCE_MS = 260

/** 当前书籍 workspace 的扫描式全局搜索面板。 */
export function SearchPanel({ workspace, onSelectResult }: SearchPanelProps) {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<WorkspaceSearchResult[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const requestSeq = useRef(0)

  const trimmedQuery = query.trim()
  const groups = useMemo(() => groupSearchResults(results), [results])

  useEffect(() => {
    requestSeq.current += 1
    const seq = requestSeq.current
    setError('')

    if (!workspace || !trimmedQuery) {
      setResults([])
      setLoading(false)
      return
    }

    setLoading(true)
    const timer = window.setTimeout(() => {
      searchWorkspace(trimmedQuery, SEARCH_LIMIT)
        .then((items) => {
          if (requestSeq.current !== seq) return
          setResults(items)
        })
        .catch((e) => {
          if (requestSeq.current !== seq) return
          setResults([])
          setError(e instanceof Error ? e.message : t('search.failed'))
        })
        .finally(() => {
          if (requestSeq.current === seq) setLoading(false)
        })
    }, SEARCH_DEBOUNCE_MS)

    return () => window.clearTimeout(timer)
  }, [t, trimmedQuery, workspace])

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="shrink-0 space-y-2 p-1">
        <div className="relative">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--nova-text-faint)]" />
          <Input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={t('search.placeholder')}
            className="h-8 border-[var(--nova-border)] bg-[var(--nova-surface)] pl-8 pr-8 text-xs text-[var(--nova-text)] placeholder:text-[var(--nova-text-faint)]"
          />
          {loading && (
            <Loader2 className="absolute right-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 animate-spin text-[var(--nova-text-faint)]" />
          )}
        </div>
        {trimmedQuery && !loading && results.length > 0 && (
          <div className="px-1 text-[11px] text-[var(--nova-text-faint)]">
            {t('search.resultCount', { count: results.length })}
          </div>
        )}
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-1 pb-2">
        {!workspace ? (
          <SearchEmptyState text={t('search.noWorkspace')} />
        ) : error ? (
          <SearchEmptyState text={error} />
        ) : !trimmedQuery ? (
          <SearchEmptyState text={t('search.empty')} />
        ) : loading && results.length === 0 ? (
          <SearchEmptyState text={t('common.searching')} />
        ) : groups.length === 0 ? (
          <SearchEmptyState text={t('search.noResults')} />
        ) : (
          <div className="space-y-3">
            {groups.map((group) => (
              <section key={group.path} className="space-y-1.5">
                <div className="flex min-w-0 items-center gap-1.5 px-1 text-[11px] font-medium text-[var(--nova-text-faint)]">
                  <FileText className="h-3.5 w-3.5 shrink-0" />
                  <span className="truncate">{group.path}</span>
                  <span className="shrink-0">({group.results.length})</span>
                </div>
                <div className="space-y-1">
                  {group.results.map((result, index) => (
                    <button
                      key={`${result.path}:${result.line}:${result.column}:${index}`}
                      type="button"
                      className="nova-nav-item block w-full border border-transparent bg-[var(--nova-surface)] px-2 py-1.5 text-left hover:border-[var(--nova-border)]"
                      onClick={() => void onSelectResult(result, trimmedQuery)}
                    >
                      <div className="mb-1 flex items-center justify-between gap-2 text-[11px] text-[var(--nova-text-faint)]">
                        <span>{result.line > 0 ? t('search.line', { line: result.line }) : t('search.pathMatch')}</span>
                        {result.column > 0 && <span>{t('search.column', { column: result.column })}</span>}
                      </div>
                      <p className="line-clamp-2 whitespace-pre-wrap break-words text-xs leading-5 text-[var(--nova-text-muted)]">
                        <HighlightedText text={result.preview || result.path} query={trimmedQuery} />
                      </p>
                    </button>
                  ))}
                </div>
              </section>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

function SearchEmptyState({ text }: { text: string }) {
  return (
    <div className="rounded-lg border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-4 text-center text-xs text-[var(--nova-text-faint)]">
      {text}
    </div>
  )
}

function HighlightedText({ text, query }: { text: string; query: string }) {
  const parts = splitByQuery(text, query)
  return (
    <>
      {parts.map((part, index) => part.matched ? (
        <mark key={`${part.text}:${index}`} className="rounded bg-[#f0c674]/25 px-0.5 text-[#f4d58d]">
          {part.text}
        </mark>
      ) : (
        <span key={`${part.text}:${index}`}>{part.text}</span>
      ))}
    </>
  )
}

function splitByQuery(text: string, query: string): Array<{ text: string; matched: boolean }> {
  const normalizedQuery = query.trim().toLowerCase()
  if (!normalizedQuery) return [{ text, matched: false }]
  const normalizedText = text.toLowerCase()
  const parts: Array<{ text: string; matched: boolean }> = []
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

function groupSearchResults(results: WorkspaceSearchResult[]): SearchResultGroup[] {
  const groups = new Map<string, WorkspaceSearchResult[]>()
  for (const result of results) {
    const items = groups.get(result.path) || []
    items.push(result)
    groups.set(result.path, items)
  }
  return Array.from(groups, ([path, groupResults]) => ({ path, results: groupResults }))
}
