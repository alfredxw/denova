import { useEffect, useMemo, useRef, useState } from 'react'
import { FileText, Loader2, Regex, Replace, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Input } from '@/components/ui/input'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { HighlightedText } from '@/components/common/HighlightedText'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { replaceWorkspace, searchWorkspace, type WorkspaceSearchResult } from '@/lib/api'

interface SearchPanelProps {
  workspace: string
  onSelectResult: (result: WorkspaceSearchResult, query: string) => void | Promise<void>
  /** 全局替换成功后通知外层刷新受影响的打开文件与版本信息 */
  onWorkspaceChanged?: (paths: string[]) => void | Promise<void>
}

interface SearchResultGroup {
  path: string
  results: WorkspaceSearchResult[]
}

const SEARCH_LIMIT = 100
const SEARCH_DEBOUNCE_MS = 260

/** 当前书籍 workspace 的扫描式全局搜索面板，支持正则匹配与全局替换。 */
export function SearchPanel({ workspace, onSelectResult, onWorkspaceChanged }: SearchPanelProps) {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<WorkspaceSearchResult[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [useRegex, setUseRegex] = useState(false)
  const [replaceOpen, setReplaceOpen] = useState(false)
  const [replaceText, setReplaceText] = useState('')
  const [replaceConfirmOpen, setReplaceConfirmOpen] = useState(false)
  const [refreshSeq, setRefreshSeq] = useState(0)
  const requestSeq = useRef(0)

  const trimmedQuery = query.trim()
  const groups = useMemo(() => groupSearchResults(results), [results])
  const canReplace = Boolean(workspace && trimmedQuery && results.length > 0)

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
      searchWorkspace(trimmedQuery, SEARCH_LIMIT, { regex: useRegex })
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
  }, [t, trimmedQuery, useRegex, workspace, refreshSeq])

  const handleConfirmReplace = async () => {
    const data = await replaceWorkspace({ query: trimmedQuery, replacement: replaceText, regex: useRegex, workspace })
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
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="shrink-0 space-y-2 p-1">
        <div className="flex items-center gap-1">
          <div className="relative min-w-0 flex-1">
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
          <TooltipIconButton
            label={t('search.toggleRegex')}
            size="icon-xs"
            tooltipSide="top"
            aria-pressed={useRegex}
            className={useRegex ? 'nova-nav-item is-active shrink-0' : 'shrink-0 text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'}
            onClick={() => setUseRegex((value) => !value)}
          >
            <Regex className="h-3.5 w-3.5" />
          </TooltipIconButton>
          <TooltipIconButton
            label={t('search.toggleReplace')}
            size="icon-xs"
            tooltipSide="top"
            aria-pressed={replaceOpen}
            className={replaceOpen ? 'nova-nav-item is-active shrink-0' : 'shrink-0 text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'}
            onClick={() => setReplaceOpen((value) => !value)}
          >
            <Replace className="h-3.5 w-3.5" />
          </TooltipIconButton>
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
                        <HighlightedText text={result.preview || result.path} query={useRegex ? result.match_text : trimmedQuery} />
                      </p>
                    </button>
                  ))}
                </div>
              </section>
            ))}
          </div>
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
    <div className="rounded-lg border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-4 text-center text-xs text-[var(--nova-text-faint)]">
      {text}
    </div>
  )
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
