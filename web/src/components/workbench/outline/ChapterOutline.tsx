import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { BookOpen, ChevronDown, ChevronRight, FileText } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { FileNode } from '@/hooks/useWorkspace'
import type { ChapterSummary, DocumentPreview } from '@/lib/api'
import { BookSettingsShortcuts } from '@/components/workbench/BookSettingsShortcuts'
import { ChapterOutlineItem } from './ChapterOutlineItem'
import { OutlineFileActions } from './OutlineFileActions'

export interface OutlineRevealRequest {
  path: string
  nonce: number
}

interface ChapterOutlineProps {
  workspace: string
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

// 下滑超过该距离后启用顶部「回到顶部」动作。不能用视口高度做阈值：
// 内容高度不足 2 倍视口时 scrollTop 永远到不了一个视口高。
const BACK_TO_TOP_THRESHOLD_PX = 320

/**
 * 写作页「大纲」tab：书籍设定可固定，当前细纲与章节留在同一滚动目录中。
 * 长目录在固定区提供最新章与回顶；来自面板外部的章节切换会自动定位到当前章节。
 */
export function ChapterOutline({
  workspace,
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
      console.warn('保存作品目录固定区偏好失败', error)
    }
  }, [headerPinned])

  useEffect(() => {
    if (selectedFile && historicalChapterPlans.some((plan) => plan.path === selectedFile)) {
      setChapterPlanHistoryExpanded(true)
    }
  }, [historicalChapterPlans, selectedFile])

  const toggleVolume = (key: string) => {
    setCollapsedVolumes(prev => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  // ---- 滚动定位：最新章 / 已打开章节 / 顶部 ----
  const rootRef = useRef<HTMLDivElement>(null)
  const scrollContainerRef = useRef<HTMLDivElement>(null)
  // 面板内点击选中的章节本来就在视野里，标记后跳过自动定位
  const panelSelectionRef = useRef(false)
  const lastAutoLocatedRef = useRef<string | null>(null)
  const didLocateOnceRef = useRef(false)
  const [backToTopVisible, setBackToTopVisible] = useState(false)

  const handleSelectFileFromPanel = useCallback((path: string) => {
    panelSelectionRef.current = true
    void onSelectFile(path)
  }, [onSelectFile])

  const findChapterElement = useCallback((path: string) => {
    const root = rootRef.current
    if (!root) return null
    for (const element of root.querySelectorAll<HTMLElement>('[data-chapter-path]')) {
      if (element.dataset.chapterPath === path) return element
    }
    return null
  }, [])

  const locateChapter = useCallback((path: string, behavior: ScrollBehavior) => {
    const chapter = chapters.find((item) => item.path === path)
    if (!chapter) return
    const volumeKey = chapter.volume_path || chapter.volume || 'chapters'
    setCollapsedVolumes((prev) => {
      if (!prev.has(volumeKey)) return prev
      const next = new Set(prev)
      next.delete(volumeKey)
      return next
    })
    // 目标章节可能因卷折叠刚被渲染，等两帧确保 DOM 提交后再滚动
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        findChapterElement(path)?.scrollIntoView({ block: 'center', behavior })
      })
    })
  }, [chapters, findChapterElement])

  // 下滑超过固定距离后启用「回到顶部」。
  useEffect(() => {
    const container = scrollContainerRef.current
    if (!container) return
    let raf = 0
    const update = () => {
      raf = 0
      setBackToTopVisible(container.scrollTop > BACK_TO_TOP_THRESHOLD_PX)
    }
    const handleScroll = () => {
      if (raf !== 0) return
      raf = requestAnimationFrame(update)
    }
    container.addEventListener('scroll', handleScroll, { passive: true })
    update()
    return () => {
      container.removeEventListener('scroll', handleScroll)
      if (raf !== 0) cancelAnimationFrame(raf)
    }
  }, [])

  const selectedIsChapter = selectedFile !== null && chapters.some((chapter) => chapter.path === selectedFile)

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
    const behavior: ScrollBehavior = didLocateOnceRef.current ? 'smooth' : 'auto'
    didLocateOnceRef.current = true
    locateChapter(selectedFile, behavior)
  }, [chapters.length, locateChapter, revealRequest?.path, selectedFile, selectedIsChapter])

  const latestChapterPath = latestChapter?.path ?? null

  useEffect(() => {
    if (!revealRequest?.path) return
    locateChapter(revealRequest.path, 'smooth')
  }, [locateChapter, revealRequest?.nonce, revealRequest?.path])

  const scrollToTop = () => {
    scrollContainerRef.current?.scrollTo({ top: 0, behavior: 'smooth' })
  }

  const bookSettingsHeaderFrame = (
    <div data-testid="book-settings-header-frame" className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] p-2">
      <BookSettingsShortcuts
        workspace={workspace}
        tree={tree}
        outline={outline}
        ideas={ideas}
        chapterPlans={chapterPlans}
        selectedFile={selectedFile}
        loreTabActive={loreTabActive}
        headerPinned={headerPinned}
        onSelectFile={handleSelectFileFromPanel}
        onOpenLoreTab={onOpenLoreTab}
        latestChapterAvailable={Boolean(latestChapterPath)}
        backToTopAvailable={backToTopVisible}
        onLocateLatestChapter={() => { if (latestChapterPath) locateChapter(latestChapterPath, 'smooth') }}
        onBackToTop={scrollToTop}
        onToggleHeaderPinned={() => setHeaderPinned((pinned) => !pinned)}
        onReferenceFile={onReferenceFile}
        onRevealFile={onRevealFile}
        onRenameItem={onRenameItem}
        onDeleteItem={onDeleteItem}
        onRequestCreate={onRequestBookSettingCreate}
      />
    </div>
  )

  return (
    <div ref={rootRef} className="flex h-full min-h-0 flex-col">
      {headerPinned ? bookSettingsHeaderFrame : null}
      <div
        ref={scrollContainerRef}
        role="navigation"
        aria-label={t('planning.outlineNavigation')}
        className="min-h-0 flex-1 overflow-y-auto"
      >
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
                    onReferenceFile={onReferenceFile}
                    onRevealFile={onRevealFile}
                    onRenameItem={onRenameItem}
                    onDeleteItem={onDeleteItem}
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
                          onReferenceFile={onReferenceFile}
                          onRevealFile={onRevealFile}
                          onRenameItem={onRenameItem}
                          onDeleteItem={onDeleteItem}
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
            {volumes.length === 0 ? (
              <PlanningEmptyState text={t('planning.noChapters')} />
            ) : (
              <div className="space-y-1.5">
                {volumes.map((volume) => {
                  const expanded = !collapsedVolumes.has(volume.key)
                  return (
                    <div key={volume.key} className="space-y-1">
                      <button
                        type="button"
                        className="nova-nav-item flex w-full items-center gap-2 border border-transparent bg-[var(--nova-surface)] px-2 py-1.5 text-left"
                        aria-label={`${volume.label} ${t('common.chapters', { count: volume.chapters.length })}`}
                        onClick={() => toggleVolume(volume.key)}
                      >
                        {expanded ? (
                          <ChevronDown className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
                        ) : (
                          <ChevronRight className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
                        )}
                        <BookOpen className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
                        <span className="min-w-0 flex-1 truncate text-xs font-medium text-[var(--nova-text)]">{volume.label}</span>
                        <span className="shrink-0 text-[11px] text-[var(--nova-text-faint)]">{t('common.chapters', { count: volume.chapters.length })}</span>
                      </button>
                      {expanded ? (
                        <div className="space-y-1 pl-4">
                          {volume.chapters.map((chapter) => (
                            <ChapterOutlineItem
                              key={chapter.path}
                              chapter={chapter}
                              active={selectedFile === chapter.path}
                              onSelectFile={handleSelectFileFromPanel}
                              onSetChapterConfirmed={onSetChapterConfirmed}
                              onReferenceFile={onReferenceFile}
                              onRevealFile={onRevealFile}
                              onRenameItem={onRenameItem}
                              onDeleteItem={onDeleteItem}
                            />
                          ))}
                        </div>
                      ) : null}
                    </div>
                  )
                })}
              </div>
            )}
          </section>

          </div>
        </div>
      </div>
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
        title={document.title}
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

function groupChaptersByVolume(chapters: ChapterSummary[], t: (key: string) => string) {
  const map = new Map<string, { key: string; label: string; chapters: ChapterSummary[] }>()
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
    console.warn('读取作品目录固定区偏好失败', error)
    return true
  }
}
