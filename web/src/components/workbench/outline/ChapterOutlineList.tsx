import {
  forwardRef,
  useCallback,
  useImperativeHandle,
  useMemo,
  useRef,
  type ReactNode,
  type UIEvent,
} from 'react'
import { BookOpen, ChevronDown, ChevronRight } from 'lucide-react'
import { Virtuoso, type Components, type VirtuosoHandle } from 'react-virtuoso'
import type { ChapterSummary } from '@/lib/api'
import { ChapterOutlineItem } from './ChapterOutlineItem'
import { OutlineFileActions, type OutlineFileMenuOperations } from './OutlineFileActions'

export interface ChapterOutlineVolume {
  index?: number
  key: string
  label: string
  chapters: ChapterSummary[]
  children?: ChapterOutlineVolume[]
  directChapters?: ChapterSummary[]
  depth?: number
  hidden?: boolean
  order?: string[]
  breadcrumb?: string
}

export interface ChapterOutlineListHandle {
  scrollToChapter: (path: string) => void
  scrollToTop: () => void
}

interface ChapterOutlineListProps {
  header: ReactNode
  navigationLabel: string
  volumes: ChapterOutlineVolume[]
  collapsedVolumes: ReadonlySet<string>
  selectedFile: string | null
  chapterCountLabel: (count: number) => string
  onToggleVolume: (key: string) => void
  onScrollTopChange: (scrollTop: number) => void
  onSelectFile: (path: string) => void
  onSetChapterConfirmed: (path: string, confirmed: boolean) => void | Promise<void>
  onReferenceFile?: (path: string) => void
  onRevealFile?: (path: string) => void | Promise<void>
  onRenameItem?: (path: string, newName: string) => Promise<void>
  onDeleteItem?: (path: string) => Promise<void>
  onCreateChapterInVolume?: (volumePath: string) => void
  fileOperations: OutlineFileMenuOperations
}

interface VolumeRow {
  kind: 'volume'
  key: string
  volume: ChapterOutlineVolume
  expanded: boolean
}

interface ChapterRow {
  kind: 'chapter'
  depth: number
  key: string
  chapter: ChapterSummary
}

type ChapterOutlineRow = VolumeRow | ChapterRow

interface ChapterOutlineListContext {
  header: ReactNode
  selectedFile: string | null
  chapterCountLabel: (count: number) => string
  onToggleVolume: (key: string) => void
  onSelectFile: (path: string) => void
  onSetChapterConfirmed: (path: string, confirmed: boolean) => void | Promise<void>
  onReferenceFile?: (path: string) => void
  onRevealFile?: (path: string) => void | Promise<void>
  onRenameItem?: (path: string, newName: string) => Promise<void>
  onDeleteItem?: (path: string) => Promise<void>
  onCreateChapterInVolume?: (volumePath: string) => void
  fileOperations: OutlineFileMenuOperations
}

const OUTLINE_LIST_COMPONENTS: Components<ChapterOutlineRow, ChapterOutlineListContext> = {
  Header: ChapterOutlineListHeader,
  Footer: ChapterOutlineListFooter,
}

/**
 * Windowed chapter navigation. Only visible chapter rows mount their menus and React subtree;
 * volume headers remain part of the same scroll coordinate space for reliable navigation.
 */
export const ChapterOutlineList = forwardRef<ChapterOutlineListHandle, ChapterOutlineListProps>(function ChapterOutlineList({
  header,
  navigationLabel,
  volumes,
  collapsedVolumes,
  selectedFile,
  chapterCountLabel,
  onToggleVolume,
  onScrollTopChange,
  onSelectFile,
  onSetChapterConfirmed,
  onReferenceFile,
  onRevealFile,
  onRenameItem,
  onDeleteItem,
  onCreateChapterInVolume,
  fileOperations,
}, forwardedRef) {
  const virtuosoRef = useRef<VirtuosoHandle>(null)
  const scrollerRef = useRef<HTMLElement | null>(null)
  const rows = useMemo(() => flattenVisibleRows(volumes, collapsedVolumes), [collapsedVolumes, volumes])
  const rowIndexByPath = useMemo(() => {
    const result = new Map<string, number>()
    rows.forEach((row, index) => {
      if (row.kind === 'chapter') result.set(row.chapter.path, index)
    })
    return result
  }, [rows])
  const rowIndexByPathRef = useRef(rowIndexByPath)
  rowIndexByPathRef.current = rowIndexByPath

  useImperativeHandle(forwardedRef, () => ({
    scrollToChapter: (path) => {
      const visibleElement = Array.from(scrollerRef.current?.querySelectorAll<HTMLElement>('[data-chapter-path]') ?? [])
        .find((element) => element.dataset.chapterPath === path)
      if (visibleElement) {
        visibleElement.scrollIntoView({ block: 'center', behavior: 'auto' })
        return
      }
      const index = rowIndexByPathRef.current.get(path)
      if (index === undefined) return
      // Smooth traversal mounts and measures intermediate virtual rows, repeatedly
      // correcting the target position. An index jump reaches the destination once.
      virtuosoRef.current?.scrollToIndex({ index, align: 'center', behavior: 'auto' })
    },
    scrollToTop: () => {
      scrollerRef.current?.scrollTo({ top: 0, behavior: 'auto' })
    },
  }), [])

  const context = useMemo<ChapterOutlineListContext>(() => ({
    header,
    selectedFile,
    chapterCountLabel,
    onToggleVolume,
    onSelectFile,
    onSetChapterConfirmed,
    onReferenceFile,
    onRevealFile,
    onRenameItem,
    onDeleteItem,
    onCreateChapterInVolume,
    fileOperations,
  }), [chapterCountLabel, fileOperations, header, onCreateChapterInVolume, onDeleteItem, onReferenceFile, onRenameItem, onRevealFile, onSelectFile, onSetChapterConfirmed, onToggleVolume, selectedFile])

  const handleScrollerRef = useCallback((element: HTMLElement | Window | null) => {
    scrollerRef.current = element instanceof HTMLElement ? element : null
  }, [])
  const handleScroll = useCallback((event: UIEvent<HTMLElement>) => {
    onScrollTopChange(event.currentTarget.scrollTop)
  }, [onScrollTopChange])

  return (
    <Virtuoso
      ref={virtuosoRef}
      scrollerRef={handleScrollerRef}
      data={rows}
      context={context}
      components={OUTLINE_LIST_COMPONENTS}
      computeItemKey={(_index, row) => row.key}
      itemContent={renderChapterOutlineRow}
      initialItemCount={Math.min(rows.length, 24)}
      overscan={{ main: 240, reverse: 120 }}
      increaseViewportBy={{ top: 160, bottom: 320 }}
      onScroll={handleScroll}
      role="navigation"
      aria-label={navigationLabel}
      className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden [overflow-anchor:none]"
    />
  )
})

function ChapterOutlineListHeader({ context }: { context: ChapterOutlineListContext }) {
  return <>{context.header}</>
}

function ChapterOutlineListFooter() {
  return <div className="h-2" aria-hidden="true" />
}

function renderChapterOutlineRow(_index: number, row: ChapterOutlineRow, context: ChapterOutlineListContext) {
  if (row.kind === 'volume') {
    const { volume, expanded } = row
    const countLabel = context.chapterCountLabel(volume.chapters.length)
    return (
      <div className="pb-1 pr-2 pt-1.5" style={{ paddingLeft: 8 + (volume.depth || 0) * 16 }}>
        <OutlineFileActions
          path={volume.key}
          onCreateChapter={context.onCreateChapterInVolume}
          createChapterLabel={volume.depth === 1 ? 'bookStructure.section' : 'planning.create.chapter'}
          onRenameItem={context.onRenameItem}
          onDeleteItem={context.onDeleteItem}
          fileOperations={context.fileOperations}
        >
          <button
            type="button"
            className="nova-nav-item flex w-full items-center gap-2 border border-transparent bg-[var(--nova-surface)] px-2 py-1.5 pr-8 text-left"
            aria-label={`${volume.label} ${countLabel}`}
            aria-expanded={expanded}
            onClick={() => context.onToggleVolume(volume.key)}
          >
            {expanded ? (
              <ChevronDown className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
            ) : (
              <ChevronRight className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
            )}
            <BookOpen className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
            <span className="min-w-0 flex-1 truncate text-xs font-medium text-[var(--nova-text)]">{volume.label}</span>
            <span className="shrink-0 text-[11px] text-[var(--nova-text-faint)]">{countLabel}</span>
          </button>
        </OutlineFileActions>
      </div>
    )
  }

  const { chapter } = row
  return (
    <div className="pb-1 pr-2" style={{ paddingLeft: 8 + row.depth * 16 }}>
      <ChapterOutlineItem
        chapter={chapter}
        active={context.selectedFile === chapter.path}
        onSelectFile={context.onSelectFile}
        onSetChapterConfirmed={context.onSetChapterConfirmed}
        onReferenceFile={context.onReferenceFile}
        onRevealFile={context.onRevealFile}
        onRenameItem={context.onRenameItem}
        onDeleteItem={context.onDeleteItem}
        fileOperations={context.fileOperations}
      />
    </div>
  )
}

function flattenVisibleRows(volumes: ChapterOutlineVolume[], collapsedVolumes: ReadonlySet<string>) {
  const rows: ChapterOutlineRow[] = []
  const visit = (groups: ChapterOutlineVolume[]) => {
    for (const volume of groups) {
      const expanded = !collapsedVolumes.has(volume.key)
      if (!volume.hidden) rows.push({ kind: 'volume', key: `volume:${volume.key}`, volume, expanded })
      if (!expanded && !volume.hidden) continue
      // Children and direct prose can coexist in released books. Preserve the
      // server's reading order within each container.
      const entries = [
        ...(volume.children || []).map((group) => ({ path: group.key, group })),
        ...(volume.directChapters ?? volume.chapters).map((chapter) => ({ path: chapter.path, chapter })),
      ]
      if (volume.order) {
        const positions = new Map(volume.order.map((path, index) => [path, index]))
        entries.sort((a, b) => (positions.get(a.path) ?? 0) - (positions.get(b.path) ?? 0))
      }
      for (const entry of entries) {
        if ('group' in entry) visit([entry.group])
        else rows.push({ kind: 'chapter', key: `chapter:${entry.chapter.path}`, chapter: entry.chapter, depth: volume.hidden ? 0 : (volume.depth || 0) + 1 })
      }
    }
  }
  visit(volumes)
  return rows
}
