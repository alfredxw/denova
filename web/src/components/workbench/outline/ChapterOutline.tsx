import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ChevronDown, ChevronRight, FileText } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { FileNode } from '@/hooks/useWorkspace'
import type { ChapterSummary, DocumentPreview } from '@/lib/api'
import { BookSettingsShortcuts } from '@/components/workbench/BookSettingsShortcuts'
import {
  ChapterOutlineList,
  type ChapterOutlineListHandle,
  type ChapterOutlineVolume,
} from './ChapterOutlineList'
import { OutlineFileActions } from './OutlineFileActions'

export interface OutlineRevealRequest {
  path: string
  nonce: number
}

interface ChapterOutlineProps {
  projectId: string
  tree: FileNode[]
  chapters: ChapterSummary[]
  ideas?: DocumentPreview
  outline?: DocumentPreview
  chapterPlans: DocumentPreview[]
  selectedFile: string | null
  loreTabActive?: boolean
  revealRequest?: OutlineRevealRequest | null
  onSelectFile: (path: string) => void | Promise<void>
  onOpenLoreTab?: () => void | Promise<boolean | void>
  onReferenceFile?: (path: string) => void
  onRevealFile?: (path: string) => void | Promise<void>
  onRenameItem?: (path: string, newName: string) => Promise<void>
  onDeleteItem?: (path: string) => Promise<void>
  onRequestBookSettingCreate?: (item: { path: string; title: string }) => void
  onSetChapterConfirmed: (path: string, confirmed: boolean) => void | Promise<void>
}

const BOOK_SETTINGS_HEADER_PINNED_KEY = 'nova.outline.book-settings-header-pinned'

// A fixed threshold also works when content is less than two viewport heights tall.
const BACK_TO_TOP_THRESHOLD_PX = 320

/**
 * Writing outline with a pinnable book-settings frame and windowed chapter navigation.
 * External chapter selections are located automatically without mounting the full book.
 */
export function ChapterOutline({
  projectId,
  tree,
  chapters,
  ideas,
  outline,
  chapterPlans,
  selectedFile,
  loreTabActive,
  revealRequest,
  onSelectFile,
  onOpenLoreTab,
  onReferenceFile,
  onRevealFile,
  onRenameItem,
  onDeleteItem,
  onRequestBookSettingCreate,
  onSetChapterConfirmed,
}: ChapterOutlineProps) {
  const { t } = useTranslation()
  const [headerPinned, setHeaderPinned] = useState(readBookSettingsHeaderPinned)
  const [collapsedVolumes, setCollapsedVolumes] = useState<Set<string>>(() => new Set())
  const [chapterPlanHistoryExpanded, setChapterPlanHistoryExpanded] = useState(false)
  const volumes = useMemo(() => groupChaptersByVolume(chapters, t), [chapters, t])
  const latestChapterPlan = chapterPlans[chapterPlans.length - 1]
  const historicalChapterPlans = useMemo(() => chapterPlans.slice(0, -1), [chapterPlans])
  const latestChapter = chapters[chapters.length - 1]

  useEffect(() => {
    try {
      window.localStorage.setItem(BOOK_SETTINGS_HEADER_PINNED_KEY, String(headerPinned))
    } catch (error) {
      console.warn('Failed to save the book-settings header pin preference', error)
    }
  }, [headerPinned])

  useEffect(() => {
    if (selectedFile && historicalChapterPlans.some((plan) => plan.path === selectedFile)) {
      setChapterPlanHistoryExpanded(true)
    }
  }, [historicalChapterPlans, selectedFile])

  const toggleVolume = useCallback((key: string) => {
    setCollapsedVolumes(prev => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }, [])

  // Keep row callbacks stable so selecting one chapter cannot invalidate every mounted row.
  const callbackRef = useRef({
    onSelectFile,
    onOpenLoreTab,
    onReferenceFile,
    onRevealFile,
    onRenameItem,
    onDeleteItem,
    onRequestBookSettingCreate,
    onSetChapterConfirmed,
  })
  callbackRef.current = {
    onSelectFile,
    onOpenLoreTab,
    onReferenceFile,
    onRevealFile,
    onRenameItem,
    onDeleteItem,
    onRequestBookSettingCreate,
    onSetChapterConfirmed,
  }

  const listRef = useRef<ChapterOutlineListHandle>(null)
  const panelSelectionRef = useRef(false)
  const lastAutoLocatedRef = useRef<string | null>(null)
  const locateFrameRef = useRef<number | null>(null)
  const [backToTopVisible, setBackToTopVisible] = useState(false)

  const handleSelectFileFromPanel = useCallback((path: string) => {
    panelSelectionRef.current = true
    void callbackRef.current.onSelectFile(path)
  }, [])

  const handleSetChapterConfirmed = useCallback((path: string, confirmed: boolean) => {
    return callbackRef.current.onSetChapterConfirmed(path, confirmed)
  }, [])
  const handleReferenceFile = useCallback((path: string) => {
    callbackRef.current.onReferenceFile?.(path)
  }, [])
  const handleRevealFile = useCallback((path: string) => {
    return callbackRef.current.onRevealFile?.(path)
  }, [])
  const handleRenameItem = useCallback((path: string, newName: string) => {
    return callbackRef.current.onRenameItem?.(path, newName) ?? Promise.resolve()
  }, [])
  const handleDeleteItem = useCallback((path: string) => {
    return callbackRef.current.onDeleteItem?.(path) ?? Promise.resolve()
  }, [])
  const handleOpenLoreTab = useCallback(() => callbackRef.current.onOpenLoreTab?.(), [])
  const handleRequestBookSettingCreate = useCallback((item: { path: string; title: string }) => {
    callbackRef.current.onRequestBookSettingCreate?.(item)
  }, [])

  const chapterVolumeByPath = useMemo(() => {
    const result = new Map<string, string>()
    for (const volume of volumes) {
      for (const chapter of volume.chapters) result.set(chapter.path, volume.key)
    }
    return result
  }, [volumes])

  const cancelScheduledLocate = useCallback(() => {
    if (locateFrameRef.current === null) return
    cancelAnimationFrame(locateFrameRef.current)
    locateFrameRef.current = null
  }, [])

  const locateChapter = useCallback((path: string) => {
    const volumeKey = chapterVolumeByPath.get(path)
    if (!volumeKey) return
    setCollapsedVolumes((prev) => {
      if (!prev.has(volumeKey)) return prev
      const next = new Set(prev)
      next.delete(volumeKey)
      return next
    })
    // A collapsed volume needs one render before Virtuoso can resolve the row index.
    cancelScheduledLocate()
    locateFrameRef.current = requestAnimationFrame(() => {
      locateFrameRef.current = requestAnimationFrame(() => {
        locateFrameRef.current = null
        listRef.current?.scrollToChapter(path)
      })
    })
  }, [cancelScheduledLocate, chapterVolumeByPath])

  useEffect(() => cancelScheduledLocate, [cancelScheduledLocate])

  const handleScrollTopChange = useCallback((scrollTop: number) => {
    setBackToTopVisible(scrollTop > BACK_TO_TOP_THRESHOLD_PX)
  }, [])

  const selectedIsChapter = selectedFile !== null && chapterVolumeByPath.has(selectedFile)

  // 章节切换来自面板外部（编辑器 tab、搜索、切回大纲 tab 重挂载等）时，自动把当前章节滚到视野中央
  useEffect(() => {
    const fromPanel = panelSelectionRef.current
    panelSelectionRef.current = false
    if (!selectedFile || chapters.length === 0 || !selectedIsChapter) return
    if (revealRequest?.path === selectedFile) return
    if (fromPanel) {
      lastAutoLocatedRef.current = selectedFile
      return
    }
    if (lastAutoLocatedRef.current === selectedFile) return
    lastAutoLocatedRef.current = selectedFile
    locateChapter(selectedFile)
  }, [chapters.length, locateChapter, revealRequest?.path, selectedFile, selectedIsChapter])

  const latestChapterPath = latestChapter?.path ?? null

  useEffect(() => {
    if (!revealRequest?.path) return
    locateChapter(revealRequest.path)
  }, [locateChapter, revealRequest?.nonce, revealRequest?.path])

  const scrollToTop = useCallback(() => listRef.current?.scrollToTop(), [])
  const locateLatestChapter = useCallback(() => {
    if (latestChapterPath) locateChapter(latestChapterPath)
  }, [latestChapterPath, locateChapter])
  const toggleHeaderPinned = useCallback(() => setHeaderPinned((pinned) => !pinned), [])

  const stableReferenceFile = onReferenceFile ? handleReferenceFile : undefined
  const stableRevealFile = onRevealFile ? handleRevealFile : undefined
  const stableRenameItem = onRenameItem ? handleRenameItem : undefined
  const stableDeleteItem = onDeleteItem ? handleDeleteItem : undefined
  const chapterCountLabel = useCallback((count: number) => t('common.chapters', { count }), [t])

  const bookSettingsHeaderFrame = (
    <div data-testid="book-settings-header-frame" className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] p-2">
      <BookSettingsShortcuts
        projectId={projectId}
        tree={tree}
        outline={outline}
        ideas={ideas}
        chapterPlans={chapterPlans}
        selectedFile={selectedFile}
        loreTabActive={loreTabActive}
        headerPinned={headerPinned}
        onSelectFile={handleSelectFileFromPanel}
        onOpenLoreTab={onOpenLoreTab ? handleOpenLoreTab : undefined}
        latestChapterAvailable={Boolean(latestChapterPath)}
        backToTopAvailable={backToTopVisible}
        onLocateLatestChapter={locateLatestChapter}
        onBackToTop={scrollToTop}
        onToggleHeaderPinned={toggleHeaderPinned}
        onReferenceFile={stableReferenceFile}
        onRevealFile={stableRevealFile}
        onRenameItem={stableRenameItem}
        onDeleteItem={stableDeleteItem}
        onRequestCreate={onRequestBookSettingCreate ? handleRequestBookSettingCreate : undefined}
      />
    </div>
  )

  const navigationHeader = (
    <>
      {!headerPinned ? bookSettingsHeaderFrame : null}
      <div className="p-2">
        <div className="space-y-3">
          {chapterPlans.length === 0 ? (
            <section className="flex items-center justify-between gap-2 px-1 py-1 text-[11px] font-medium text-[var(--nova-text-faint)]">
              <span>{t('planning.chapterPlans')}</span>
              <span>{t('planning.chapterPlansEmpty')}</span>
            </section>
          ) : (
            <section className="space-y-1">
              {latestChapterPlan ? (
                <div className="space-y-1">
                  <div className="px-1 text-[11px] font-medium text-[var(--nova-text-faint)]">
                    {t('planning.currentChapterPlan')}
                  </div>
                  <PlanningListItem
                    document={latestChapterPlan}
                    selected={selectedFile === latestChapterPlan.path}
                    onSelectFile={handleSelectFileFromPanel}
                    onReferenceFile={stableReferenceFile}
                    onRevealFile={stableRevealFile}
                    onRenameItem={stableRenameItem}
                    onDeleteItem={stableDeleteItem}
                  />
                </div>
              ) : null}
              {historicalChapterPlans.length > 0 ? (
                <>
                  <button
                    type="button"
                    className="nova-nav-item flex w-full items-center gap-2 rounded-[var(--nova-radius)] px-2 py-1.5 text-left text-[11px] text-[var(--nova-text-muted)]"
                    aria-label={`${t('planning.chapterPlanHistory')} ${t('planning.chapterPlanCount', { count: historicalChapterPlans.length })}`}
                    aria-expanded={chapterPlanHistoryExpanded}
                    onClick={() => setChapterPlanHistoryExpanded((expanded) => !expanded)}
                  >
                    {chapterPlanHistoryExpanded ? (
                      <ChevronDown className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                    ) : (
                      <ChevronRight className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                    )}
                    <span className="min-w-0 flex-1 truncate">{t('planning.chapterPlanHistory')}</span>
                    <span className="shrink-0 text-[var(--nova-text-faint)]">{t('planning.chapterPlanCount', { count: historicalChapterPlans.length })}</span>
                  </button>
                  {chapterPlanHistoryExpanded ? (
                    <div className="space-y-1 pl-4">
                      {historicalChapterPlans.map((plan) => (
                        <PlanningListItem
                          key={plan.path}
                          document={plan}
                          selected={selectedFile === plan.path}
                          onSelectFile={handleSelectFileFromPanel}
                          onReferenceFile={stableReferenceFile}
                          onRevealFile={stableRevealFile}
                          onRenameItem={stableRenameItem}
                          onDeleteItem={stableDeleteItem}
                        />
                      ))}
                    </div>
                  ) : null}
                </>
              ) : null}
            </section>
          )}

          <section className="space-y-1.5">
            <div className="px-1 text-[11px] font-medium text-[var(--nova-text-faint)]">{t('planning.volumeChapters')}</div>
            {volumes.length === 0 ? <PlanningEmptyState text={t('planning.noChapters')} /> : null}
          </section>
        </div>
      </div>
    </>
  )

  return (
    <div className="flex h-full min-h-0 flex-col">
      {headerPinned ? bookSettingsHeaderFrame : null}
      <ChapterOutlineList
        ref={listRef}
        header={navigationHeader}
        navigationLabel={t('planning.outlineNavigation')}
        volumes={volumes}
        collapsedVolumes={collapsedVolumes}
        selectedFile={selectedFile}
        chapterCountLabel={chapterCountLabel}
        onToggleVolume={toggleVolume}
        onScrollTopChange={handleScrollTopChange}
        onSelectFile={handleSelectFileFromPanel}
        onSetChapterConfirmed={handleSetChapterConfirmed}
        onReferenceFile={stableReferenceFile}
        onRevealFile={stableRevealFile}
        onRenameItem={stableRenameItem}
        onDeleteItem={stableDeleteItem}
      />
    </div>
  )
}

const PlanningListItem = memo(function PlanningListItem({
  document,
  selected,
  onSelectFile,
  onReferenceFile,
  onRevealFile,
  onRenameItem,
  onDeleteItem,
}: {
  document: DocumentPreview
  selected: boolean
  onSelectFile: (path: string) => void | Promise<void>
  onReferenceFile?: (path: string) => void
  onRevealFile?: (path: string) => void | Promise<void>
  onRenameItem?: (path: string, newName: string) => Promise<void>
  onDeleteItem?: (path: string) => Promise<void>
}) {
  return (
    <OutlineFileActions
      path={document.path}
      onReferenceFile={onReferenceFile}
      onRevealFile={onRevealFile}
      onRenameItem={onRenameItem}
      onDeleteItem={onDeleteItem}
    >
      <button
        type="button"
        className={`nova-nav-item w-full border px-2 py-1 pr-8 text-left !text-sm !leading-normal ${
          selected
            ? 'is-active border-[var(--nova-border)]'
            : 'border-transparent bg-[var(--nova-surface)]'
        }`}
        onClick={() => onSelectFile(document.path)}
      >
        <div className="flex min-w-0 items-center gap-2">
          <FileText className={`h-3.5 w-3.5 shrink-0 ${selected ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'}`} />
          <span className="min-w-0 flex-1 truncate font-medium">{document.title}</span>
        </div>
      </button>
    </OutlineFileActions>
  )
})

function PlanningEmptyState({ text }: { text: string }) {
  return (
    <div className="rounded border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-2 text-[11px] text-[var(--nova-text-faint)]">
      {text}
    </div>
  )
}

function groupChaptersByVolume(chapters: ChapterSummary[], t: (key: string) => string): ChapterOutlineVolume[] {
  const map = new Map<string, ChapterOutlineVolume>()
  for (const chapter of chapters) {
    const key = chapter.volume_path || chapter.volume || 'chapters'
    const label = chapter.volume || t('planning.unvolumed')
    const existing = map.get(key)
    if (existing) {
      existing.chapters.push(chapter)
    } else {
      map.set(key, { key, label, chapters: [chapter] })
    }
  }
  return Array.from(map.values())
}

function readBookSettingsHeaderPinned() {
  if (typeof window === 'undefined') return true
  try {
    return window.localStorage.getItem(BOOK_SETTINGS_HEADER_PINNED_KEY) !== 'false'
  } catch (error) {
    console.warn('Failed to read the book-settings header pin preference', error)
    return true
  }
}
