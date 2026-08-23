import { useEffect, useMemo, useRef, useState } from 'react'
import { ChevronDown, ChevronRight, FileText, Loader2, Regex, Replace, Search, SlidersHorizontal } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { GroupedVirtuoso } from 'react-virtuoso'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { HighlightedText } from '@/components/common/HighlightedText'
import { replaceWorkspace, searchWorkspace, type WorkspaceSearchResult } from '@/lib/api'
import { workspaceFileName } from '@/lib/workspace-path'

interface SearchPanelProps {
  projectId: string
  onSelectResult: (result: WorkspaceSearchResult, query: string) => void | Promise<void>
  onBeforeReplace?: () => boolean | Promise<boolean>
  /** Notifies the host so open editors and version state can refresh. */
  onWorkspaceChanged?: (paths: string[]) => void | Promise<void>
}

interface SearchResultGroup {
  path: string
  results: WorkspaceSearchResult[]
}

const SEARCH_LIMIT = 100
const SEARCH_DEBOUNCE_MS = 260
const FILE_RESULT_PREVIEW_LIMIT = 3

/** Project-scoped text/path search with regex and recoverable replace-all. */
export function SearchPanel({ projectId, onSelectResult, onBeforeReplace, onWorkspaceChanged }: SearchPanelProps) {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<WorkspaceSearchResult[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [useRegex, setUseRegex] = useState(false)
  const [replaceOpen, setReplaceOpen] = useState(false)
  const [replaceText, setReplaceText] = useState('')
  const [replaceConfirmOpen, setReplaceConfirmOpen] = useState(false)
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(() => new Set())
  const [selectedResultKey, setSelectedResultKey] = useState('')
  const [refreshSeq, setRefreshSeq] = useState(0)
  const requestSeq = useRef(0)

  const trimmedQuery = query.trim()
  const groups = useMemo(() => groupSearchResults(results), [results])
  const visibleResults = useMemo(() => {
    const items: WorkspaceSearchResult[] = []
    const groupCounts = groups.map((group) => {
      const visibleGroupResults = expandedPaths.has(group.path)
        ? group.results
        : group.results.slice(0, FILE_RESULT_PREVIEW_LIMIT)
      items.push(...visibleGroupResults)
      return visibleGroupResults.length
    })
    return { groupCounts, items }
  }, [expandedPaths, groups])
  const canReplace = Boolean(projectId && trimmedQuery && results.length > 0)

  useEffect(() => {
    requestSeq.current += 1
    const seq = requestSeq.current
    setError('')

    if (!projectId || !trimmedQuery) {
      setResults([])
      setLoading(false)
      return
    }

    setLoading(true)
    const timer = window.setTimeout(() => {
      searchWorkspace(projectId, trimmedQuery, SEARCH_LIMIT, { regex: useRegex })
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
  }, [projectId, refreshSeq, t, trimmedQuery, useRegex])

  const handleConfirmReplace = async () => {
    if (onBeforeReplace && !await onBeforeReplace()) return false
    const data = await replaceWorkspace(projectId, { query: trimmedQuery, replacement: replaceText, regex: useRegex })
    const paths = data.files.map((file) => file.path)
    if (data.total_replacements > 0) {
      toast.success(t('search.replaceDone', { count: data.total_replacements, files: data.files.length }))
    } else {
      toast.info(t('search.replaceNoMatches'))
    }
    if (data.skipped.length > 0) {
      toast.warning(t('search.replaceSkipped', { count: data.skipped.length }))
    }
    if (paths.length > 0) {
      await onWorkspaceChanged?.(paths)
    }
    setRefreshSeq((value) => value + 1)
    return true
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="shrink-0 space-y-2 p-1">
        <div className="relative">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--nova-text-faint)]" />
          <Input
            autoFocus
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={t('search.placeholder')}
            className="h-8 border-[var(--nova-border)] bg-[var(--nova-surface)] pl-8 pr-8 text-xs text-[var(--nova-text)] placeholder:text-[var(--nova-text-faint)]"
          />
          {loading && (
            <Loader2 className="absolute right-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 animate-spin text-[var(--nova-text-faint)]" />
          )}
        </div>
        <div className="flex min-h-6 items-center justify-between gap-2 px-1">
          <div aria-live="polite" className="min-w-0 truncate text-[11px] text-[var(--nova-text-faint)]">
            {trimmedQuery && !loading && results.length > 0
              ? t(results.length === SEARCH_LIMIT ? 'search.resultLimit' : 'search.resultCount', { count: results.length })
              : null}
          </div>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="xs"
                className={useRegex || replaceOpen ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'}
              >
                <SlidersHorizontal data-icon="inline-start" />
                {t('search.options')}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-40">
              <DropdownMenuCheckboxItem
                checked={useRegex}
                onCheckedChange={(checked) => setUseRegex(checked === true)}
              >
                <Regex />
                {t('search.toggleRegex')}
              </DropdownMenuCheckboxItem>
              <DropdownMenuItem onSelect={() => setReplaceOpen((value) => !value)}>
                <Replace />
                {t('search.toggleReplace')}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
        {replaceOpen && (
          <div className="flex items-center gap-1">
            <div className="relative min-w-0 flex-1">
              <Replace className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--nova-text-faint)]" />
              <Input
                value={replaceText}
                onChange={(event) => setReplaceText(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter' && canReplace) {
                    event.preventDefault()
                    setReplaceConfirmOpen(true)
                  }
                }}
                placeholder={t('search.replacePlaceholder')}
                className="h-8 border-[var(--nova-border)] bg-[var(--nova-surface)] pl-8 text-xs text-[var(--nova-text)] placeholder:text-[var(--nova-text-faint)]"
              />
            </div>
            <button
              type="button"
              onClick={() => setReplaceConfirmOpen(true)}
              disabled={!canReplace}
              className="shrink-0 rounded px-2 py-1 text-[11px] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:opacity-40"
            >
              {t('search.replaceAll')}
            </button>
          </div>
        )}
      </div>

      <div className="min-h-0 flex-1 px-1 pb-2">
        {!projectId ? (
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
          <GroupedVirtuoso
            className="h-full"
            groupCounts={visibleResults.groupCounts}
            data={visibleResults.items}
            computeItemKey={(index, result) => result ? `result:${searchResultKey(result)}` : `group:${index}`}
            groupContent={(groupIndex) => {
              const group = groups[groupIndex]
              const expandable = group.results.length > FILE_RESULT_PREVIEW_LIMIT
              const expanded = expandedPaths.has(group.path)
              const content = (
                <>
                  {expandable
                    ? expanded
                      ? <ChevronDown className="h-3.5 w-3.5 shrink-0" />
                      : <ChevronRight className="h-3.5 w-3.5 shrink-0" />
                    : <FileText className="h-3.5 w-3.5 shrink-0" />}
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-xs font-medium text-[var(--nova-text)]">
                      {searchResultDisplayName(group.path)}
                    </span>
                    <span className="block truncate text-[10px] font-normal text-[var(--nova-text-faint)]" title={group.path}>
                      {group.path}
                    </span>
                  </span>
                  <span className="shrink-0 rounded-full bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] tabular-nums text-[var(--nova-text-faint)]">
                    {group.results.length}
                  </span>
                </>
              )

              return expandable ? (
                <button
                  type="button"
                  aria-expanded={expanded}
                  aria-label={t(expanded ? 'search.collapseFileResults' : 'search.expandFileResults', {
                    name: searchResultDisplayName(group.path),
                    count: group.results.length,
                  })}
                  className="nova-nav-item flex w-full items-center gap-1.5 bg-[var(--nova-bg)] px-1 py-1.5 text-left"
                  onClick={() => {
                    setExpandedPaths((current) => {
                      const next = new Set(current)
                      if (next.has(group.path)) next.delete(group.path)
                      else next.add(group.path)
                      return next
                    })
                  }}
                >
                  {content}
                </button>
              ) : (
                <div className="flex min-w-0 items-center gap-1.5 bg-[var(--nova-bg)] px-1 py-1.5">
                  {content}
                </div>
              )
            }}
            itemContent={(_, _groupIndex, result) => {
              const key = searchResultKey(result)
              const selected = selectedResultKey === `${trimmedQuery}:${key}`
              return (
                <div className="pb-1">
                  <button
                    type="button"
                    aria-current={selected ? 'true' : undefined}
                    className={`nova-nav-item block w-full border px-2 py-1.5 text-left ${
                      selected
                        ? 'is-active border-[var(--nova-border)]'
                        : 'border-transparent bg-[var(--nova-surface)] hover:border-[var(--nova-border)]'
                    }`}
                    onClick={() => {
                      setSelectedResultKey(`${trimmedQuery}:${key}`)
                      void onSelectResult(result, trimmedQuery)
                    }}
                  >
                    <div className="mb-0.5 text-[10px] tabular-nums text-[var(--nova-text-faint)]">
                      {result.line > 0 ? t('search.line', { line: result.line }) : t('search.pathMatch')}
                    </div>
                    <p className="line-clamp-2 whitespace-pre-wrap break-words text-xs leading-5 text-[var(--nova-text-muted)]">
                      <HighlightedText text={result.preview || result.path} query={useRegex ? result.match_text : trimmedQuery} />
                    </p>
                  </button>
                </div>
              )
            }}
          />
        )}
      </div>

      <ConfirmDialog
        open={replaceConfirmOpen}
        onOpenChange={setReplaceConfirmOpen}
        title={t('search.replaceConfirmTitle')}
        description={t('search.replaceConfirmDescription', { query: trimmedQuery, replacement: replaceText })}
        confirmLabel={t('search.replaceAll')}
        onConfirm={handleConfirmReplace}
      />
    </div>
  )
}

function SearchEmptyState({ text }: { text: string }) {
  return (
    <div className="flex min-h-32 flex-col items-center justify-center gap-2 px-3 py-6 text-center text-xs text-[var(--nova-text-faint)]">
      <Search className="h-5 w-5 opacity-60" />
      <span>{text}</span>
    </div>
  )
}

function searchResultKey(result: WorkspaceSearchResult) {
  return `${result.path}:${result.line}:${result.column}`
}

function searchResultDisplayName(path: string) {
  const fileName = workspaceFileName(path)
  const chapterMatch = fileName.match(/^ch\d+-(第.+?章|Chapter\s+\d+)-(.+)\.md$/i)
  return chapterMatch ? `${chapterMatch[1]} · ${chapterMatch[2]}` : fileName
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
