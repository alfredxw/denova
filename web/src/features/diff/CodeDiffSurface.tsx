import { useCallback, useMemo, useRef, type ReactNode } from 'react'
import { DEFAULT_THEMES, parseDiffFromFile } from '@pierre/diffs'
import {
  CodeView,
  type CodeViewHandle,
  type CodeViewItem,
  type DiffLineAnnotation,
  type FileDiffMetadata,
  type SelectedLineRange,
} from '@pierre/diffs/react'
import { ChevronDown, ChevronLeft, ChevronRight, Copy } from 'lucide-react'
import { useTheme } from 'next-themes'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { InlineCollapsiblePane } from '@/components/layout/panel-motion'
import { Button } from '@/components/ui/button'
import { DiffFileIcon, DiffFileIconSprite } from './DiffFileIcon'
import { DiffFileNavigator } from './DiffFileNavigator'
import type { DiffFileDocument, DiffFileNavigationItem, DiffLayout } from './types'
import type { MultiFileDiffNavigation } from './use-multi-file-diff-navigation'
import '@/features/changes/review/review-diff.css'

export const LARGE_DIFF_LINE_LIMIT = 3_000

// Pierre intentionally lets its default gutter action straddle the line-number
// and code columns. Review keeps the action inside the number gutter so it can
// never cover changed content.
const REVIEW_GUTTER_UTILITY_CSS = `
[data-gutter-utility-slot] {
  overflow: hidden;
}

[data-utility-button] {
  margin-right: 0;
}
`

export interface CodeDiffSurfaceFile extends DiffFileDocument {
  /** Plain text shown in place of a binary or otherwise unavailable diff. */
  unavailableContent?: string
}

interface CodeDiffSurfaceProps<Annotation> {
  files: readonly CodeDiffSurfaceFile[]
  navigatorFiles: readonly DiffFileNavigationItem[]
  navigation: MultiFileDiffNavigation
  layout: DiffLayout
  ariaLabel: string
  empty: ReactNode
  annotationsByPath?: ReadonlyMap<string, readonly DiffLineAnnotation<Annotation>[]>
  annotationRevisionByPath?: ReadonlyMap<string, string>
  renderHeaderMeta?: (file: CodeDiffSurfaceFile) => ReactNode
  renderHeaderAction?: (file: CodeDiffSurfaceFile) => ReactNode
  renderAnnotation?: (annotation: DiffLineAnnotation<Annotation>, file: CodeDiffSurfaceFile) => ReactNode
  onLineSelectionEnd?: (file: CodeDiffSurfaceFile, range: SelectedLineRange) => void
}

interface ParsedFile {
  source: CodeDiffSurfaceFile
  diff: FileDiffMetadata
  stats: { additions: number; deletions: number }
}

interface ParsedFileCacheEntry extends ParsedFile {
  before: string
  after: string
  beforeExists: boolean
  afterExists: boolean
}

/** Virtualized diff renderer shared by change review and version history. */
export function CodeDiffSurface<Annotation>({ files, navigatorFiles, navigation, layout, ariaLabel, empty, annotationsByPath, annotationRevisionByPath, renderHeaderMeta, renderHeaderAction, renderAnnotation, onLineSelectionEnd }: CodeDiffSurfaceProps<Annotation>) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const codeViewRef = useRef<CodeViewHandle<Annotation> | null>(null)
  const parseCacheRef = useRef(new Map<string, ParsedFileCacheEntry>())
  const versionCacheRef = useRef(new Map<string, { signature: string; version: number }>())

  const parsedFiles = useMemo(() => files.map((file) => {
    const beforeExists = file.before_exists !== false
    const afterExists = file.after_exists !== false
    const cached = parseCacheRef.current.get(file.path)
    if (cached
      && cached.before === file.before_content
      && cached.after === file.after_content
      && cached.beforeExists === beforeExists
      && cached.afterExists === afterExists) {
      return { ...cached, source: file }
    }
    const diff = parseDiffFromFile(
      beforeExists ? { name: file.path, contents: file.before_content, cacheKey: `${file.path}:before:${file.base_revision}` } : null,
      afterExists ? { name: file.path, contents: file.after_content, cacheKey: `${file.path}:after:${file.revision}` } : null,
    )
    const stats = diff.hunks.reduce((total, hunk) => ({
      additions: total.additions + hunk.additionLines,
      deletions: total.deletions + hunk.deletionLines,
    }), { additions: 0, deletions: 0 })
    const entry: ParsedFileCacheEntry = { source: file, diff, stats, before: file.before_content, after: file.after_content, beforeExists, afterExists }
    parseCacheRef.current.set(file.path, entry)
    return entry
  }), [files])

  const totalRenderedLines = useMemo(() => parsedFiles.reduce(
    (total, file) => total + Math.max(file.diff.splitLineCount, file.diff.unifiedLineCount),
    0,
  ), [parsedFiles])
  const singleFileMode = totalRenderedLines > LARGE_DIFF_LINE_LIMIT
  const activeIndex = Math.max(0, files.findIndex((file) => file.path === navigation.activePath))
  const visibleFiles = singleFileMode ? parsedFiles.slice(activeIndex, activeIndex + 1) : parsedFiles
  const fileByPath = useMemo(() => new Map(files.map((file) => [file.path, file])), [files])
  const parsedByPath = useMemo(() => new Map(parsedFiles.map((file) => [file.source.path, file])), [parsedFiles])
  const treeFiles = useMemo(() => navigatorFiles.map((file) => ({
    ...file,
    additions: parsedByPath.get(file.path)?.stats.additions ?? file.additions,
    deletions: parsedByPath.get(file.path)?.stats.deletions ?? file.deletions,
  })), [navigatorFiles, parsedByPath])
  const navigationByPath = useMemo(() => new Map(treeFiles.map((file) => [file.path, file])), [treeFiles])

  const items = useMemo<readonly CodeViewItem<Annotation>[]>(() => visibleFiles.map(({ source, diff }) => {
    const annotations = annotationsByPath?.get(source.path)
    const signature = [
      source.base_revision,
      source.revision,
      navigation.collapsedPaths.has(source.path) ? 'collapsed' : 'expanded',
      annotationRevisionByPath?.get(source.path) ?? '',
      source.unavailableContent ?? '',
    ].join(':')
    const previous = versionCacheRef.current.get(source.path)
    const version = previous?.signature === signature ? previous.version : (previous?.version ?? 0) + 1
    versionCacheRef.current.set(source.path, { signature, version })
    if (source.unavailableContent !== undefined) {
      return {
        id: source.path,
        type: 'file',
        file: { name: source.path, contents: source.unavailableContent, cacheKey: `${source.path}:unavailable:${source.revision}` },
        collapsed: navigation.collapsedPaths.has(source.path),
        version,
      }
    }
    return {
      id: source.path,
      type: 'diff',
      fileDiff: diff,
      annotations: annotations ? [...annotations] : undefined,
      collapsed: navigation.collapsedPaths.has(source.path),
      version,
    }
  }), [annotationRevisionByPath, annotationsByPath, navigation.collapsedPaths, visibleFiles])

  const selectFile = useCallback((path: string) => {
    navigation.selectFile(path)
    if (!singleFileMode) codeViewRef.current?.scrollTo({ type: 'item', id: path, align: 'start', behavior: 'instant' })
  }, [navigation, singleFileMode])

  const syncActiveFile = useCallback((scrollTop: number, viewer: NonNullable<ReturnType<CodeViewHandle<Annotation>['getInstance']>>) => {
    if (singleFileMode || files.length === 0) return
    const activationLine = scrollTop + 48
    let nextPath = files[0].path
    for (const file of files) {
      const top = viewer.getTopForItem(file.path)
      if (top === undefined || top > activationLine) break
      nextPath = file.path
    }
    if (nextPath !== navigation.activePath) navigation.setActivePath(nextPath)
  }, [files, navigation, singleFileMode])

  const copyPath = useCallback(async (path: string) => {
    try {
      await navigator.clipboard.writeText(path)
      toast.success(t('changes.copyPathSuccess'))
    } catch (error) {
      console.error('[features/diff/CodeDiffSurface.tsx] copying changed file path failed', { path, error })
      toast.error(t('changes.copyPathFailed'))
    }
  }, [t])

  const renderCustomHeader = useCallback((item: CodeViewItem<Annotation>) => {
    const file = fileByPath.get(item.id)
    if (!file) return null
    const parsed = parsedByPath.get(item.id)
    const collapsed = navigation.collapsedPaths.has(file.path)
    const directory = file.path.includes('/') ? file.path.slice(0, file.path.lastIndexOf('/') + 1) : ''
    const basename = file.path.slice(directory.length)
    const kind = navigationByPath.get(file.path)?.kind ?? 'modified'
    return (
      <div data-code-diff-header={file.path} className={`nova-code-diff-header group/file-header flex min-h-8 min-w-0 items-center border-l-2 pr-1.5 transition-colors hover:bg-[var(--nova-hover)] focus-within:bg-[var(--nova-hover)] ${navigation.activePath === file.path ? 'border-l-[var(--nova-accent-blue)]' : 'border-l-transparent'}`}>
        <button
          type="button"
          aria-expanded={!collapsed}
          aria-label={t(collapsed ? 'changes.expandFile' : 'changes.collapseFile', { path: file.path })}
          onClick={() => navigation.toggleFile(file.path)}
          className="flex min-w-0 flex-1 items-center gap-1.5 self-stretch px-2 text-left focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-[var(--nova-accent-blue)]"
        >
          <ChevronDown className={`size-3 shrink-0 text-[var(--nova-text-faint)] transition-transform ${collapsed ? '-rotate-90' : ''}`} />
          <DiffFileIcon path={file.path} kind={kind} />
          <span className="min-w-0 flex-1 truncate font-mono text-[11px]">
            <span className="text-[var(--nova-text-faint)]">{directory}</span>
            <span className="text-[var(--nova-text)]">{basename}</span>
          </span>
        </button>
        {parsed && <span className="mr-1.5 font-mono text-[10px] text-[var(--nova-success)]">+{parsed.stats.additions}</span>}
        {parsed && <span className="mr-1.5 font-mono text-[10px] text-[var(--nova-danger)]">−{parsed.stats.deletions}</span>}
        <span className="nova-code-diff-header-meta contents">{renderHeaderMeta?.(file)}</span>
        {file.after_exists === false && <span className="mr-2 text-[10px] text-[var(--nova-danger)]">{t('changes.fileDeleted')}</span>}
        <span className="nova-code-diff-header-actions flex shrink-0 items-center gap-0.5">
          <Button type="button" size="icon-xs" variant="ghost" onClick={() => void copyPath(file.path)} aria-label={t('changes.copyPath')}>
            <Copy />
          </Button>
          {renderHeaderAction?.(file)}
        </span>
      </div>
    )
  }, [copyPath, fileByPath, navigation, navigationByPath, parsedByPath, renderHeaderAction, renderHeaderMeta, t])

  const options = useMemo(() => ({
    theme: DEFAULT_THEMES,
    themeType: resolvedTheme === 'light' ? 'light' as const : 'dark' as const,
    diffStyle: layout,
    overflow: 'wrap' as const,
    diffIndicators: 'bars' as const,
    hunkSeparators: 'line-info-basic' as const,
    collapsedContextThreshold: 8,
    expansionLineCount: 20,
    lineDiffType: 'word-alt' as const,
    lineHoverHighlight: 'line' as const,
    enableGutterUtility: Boolean(onLineSelectionEnd),
    enableLineSelection: Boolean(onLineSelectionEnd),
    unsafeCSS: onLineSelectionEnd ? REVIEW_GUTTER_UTILITY_CSS : undefined,
    stickyHeaders: true,
    pointerEventsOnScroll: true,
    layout: { paddingTop: 0, paddingBottom: 0, gap: 0 },
    itemMetrics: { lineHeight: 20, diffHeaderHeight: 32, spacing: 0 },
    onGutterUtilityClick: onLineSelectionEnd ? (range: SelectedLineRange, context: { item: CodeViewItem<Annotation> }) => {
      const file = fileByPath.get(context.item.id)
      if (file) onLineSelectionEnd(file, range)
    } : undefined,
    onLineSelectionEnd: onLineSelectionEnd ? (range: SelectedLineRange | null, context: { item: CodeViewItem<Annotation> }) => {
      const file = fileByPath.get(context.item.id)
      if (file && range) onLineSelectionEnd(file, range)
      codeViewRef.current?.clearSelectedLines()
    } : undefined,
  }), [fileByPath, layout, onLineSelectionEnd, resolvedTheme])

  const previousFile = activeIndex > 0 ? files[activeIndex - 1] : null
  const nextFile = activeIndex < files.length - 1 ? files[activeIndex + 1] : null

  return (
    <div className="nova-review-container min-h-0 flex-1">
      <DiffFileIconSprite />
      <div className="nova-review-layout flex h-full min-h-0">
        <main className="min-h-0 min-w-0 flex-1 overflow-hidden bg-[var(--nova-bg)]" aria-label={ariaLabel}>
          {files.length ? (
            <div className="flex h-full min-h-0 flex-col">
              {singleFileMode && (
                <div role="status" className="flex min-h-10 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 text-[11px] text-[var(--nova-text-muted)]">
                  <span className="min-w-0 flex-1 truncate">{t('changes.largeDiffSingleFile')}</span>
                  <span className="font-mono text-[10px] tabular-nums text-[var(--nova-text-faint)]">{activeIndex + 1}/{files.length}</span>
                  <Button type="button" size="icon-xs" variant="ghost" disabled={!previousFile} onClick={() => previousFile && selectFile(previousFile.path)} aria-label={t('changes.previousFile')}><ChevronLeft /></Button>
                  <Button type="button" size="icon-xs" variant="ghost" disabled={!nextFile} onClick={() => nextFile && selectFile(nextFile.path)} aria-label={t('changes.nextFile')}><ChevronRight /></Button>
                </div>
              )}
              <CodeView<Annotation>
                ref={codeViewRef}
                items={items}
                options={options}
                className="nova-code-diff-view min-h-0 flex-1"
                renderCustomHeader={renderCustomHeader}
                renderAnnotation={renderAnnotation ? (annotation, item) => {
                  if (!('side' in annotation)) return null
                  const file = fileByPath.get(item.id)
                  return file ? renderAnnotation(annotation, file) : null
                } : undefined}
                onScroll={syncActiveFile}
              />
            </div>
          ) : empty}
        </main>
        <InlineCollapsiblePane visible={navigation.navigatorVisible} side="right" size="clamp(14rem, 19vw, 17rem)" className="nova-review-file-navigator-motion">
          <DiffFileNavigator files={treeFiles} selectedPath={navigation.activePath} onSelect={selectFile} />
        </InlineCollapsiblePane>
      </div>
    </div>
  )
}
