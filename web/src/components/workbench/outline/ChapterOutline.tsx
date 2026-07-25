import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ArrowDownToLine, ArrowUp, BookOpen, ChevronDown, ChevronRight, Crosshair, FileText } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { FileNode } from '@/hooks/useWorkspace'
import type { ChapterSummary, DocumentPreview } from '@/lib/api'
import { BookSettingsShortcuts } from '@/components/workbench/BookSettingsShortcuts'
import { ChapterOutlineItem } from './ChapterOutlineItem'

interface ChapterOutlineProps {
  workspace: string
  tree: FileNode[]
  chapters: ChapterSummary[]
  ideas?: DocumentPreview
  outline?: DocumentPreview
  chapterPlans: DocumentPreview[]
  selectedFile: string | null
  onSelectFile: (path: string) => void | Promise<void>
  onRequestBookSettingCreate: (item: { path: string; title: string }) => void
  onSetChapterConfirmed: (path: string, confirmed: boolean) => void | Promise<void>
}

const floaterButtonBaseClass =
  'pointer-events-auto flex h-8 items-center justify-center rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)]/85 text-[var(--nova-text-faint)] shadow-[0_10px_24px_rgba(0,0,0,0.16)] backdrop-blur-xl transition duration-200 hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--nova-ring)]'

const BOOK_SETTINGS_HEADER_PINNED_KEY = 'nova.outline.book-settings-header-pinned'

// 下滑超过该距离后出现「回到顶部」浮标。不能用视口高度做阈值：
// 内容高度不足 2 倍视口时 scrollTop 永远到不了一个视口高。
const BACK_TO_TOP_THRESHOLD_PX = 320

/**
 * 写作页「大纲」tab：可固定的书籍设定与最新细纲入口独立于章节滚动区。
 * 长目录提供最新章、已打开章节与顶部定位；来自面板外部的章节切换会自动定位到当前章节。
 */
export function ChapterOutline({
  workspace,
  tree,
  chapters,
  ideas,
  outline,
  chapterPlans,
  selectedFile,
  onSelectFile,
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
  const [activeChapterOffscreen, setActiveChapterOffscreen] = useState(false)
  const [latestChapterOffscreen, setLatestChapterOffscreen] = useState(false)

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

  // 下滑超过固定距离后展示「回到顶部」浮标。
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
    if (fromPanel) {
      lastAutoLocatedRef.current = selectedFile
      return
    }
    if (lastAutoLocatedRef.current === selectedFile) return
    lastAutoLocatedRef.current = selectedFile
    const behavior: ScrollBehavior = didLocateOnceRef.current ? 'smooth' : 'auto'
    didLocateOnceRef.current = true
    locateChapter(selectedFile, behavior)
  }, [chapters.length, locateChapter, selectedFile, selectedIsChapter])

  const selectedChapterPath = selectedIsChapter ? selectedFile : null
  const latestChapterPath = latestChapter?.path ?? null

  // 同时观测已打开章节和最新章，卷折叠导致节点未渲染时也视为离开视区。
  useEffect(() => {
    const container = scrollContainerRef.current
    const activeTarget = selectedChapterPath ? findChapterElement(selectedChapterPath) : null
    const latestTarget = latestChapterPath ? findChapterElement(latestChapterPath) : null
    setActiveChapterOffscreen(Boolean(selectedChapterPath && !activeTarget))
    setLatestChapterOffscreen(Boolean(latestChapterPath && !latestTarget))
    if (!container || typeof IntersectionObserver === 'undefined') return
    const observer = new IntersectionObserver((entries) => {
      for (const entry of entries) {
        if (entry.target === activeTarget) setActiveChapterOffscreen(!entry.isIntersecting)
        if (entry.target === latestTarget) setLatestChapterOffscreen(!entry.isIntersecting)
      }
    }, { root: container })
    if (activeTarget) observer.observe(activeTarget)
    if (latestTarget && latestTarget !== activeTarget) observer.observe(latestTarget)
    return () => observer.disconnect()
  }, [chapters, collapsedVolumes, findChapterElement, latestChapterPath, selectedChapterPath])

  const scrollToTop = () => {
    scrollContainerRef.current?.scrollTo({ top: 0, behavior: 'smooth' })
  }

  const bookSettingsHeader = (
    <div className="space-y-2.5">
      <BookSettingsShortcuts
        workspace={workspace}
        tree={tree}
        outline={outline}
        ideas={ideas}
        chapterPlans={chapterPlans}
        selectedFile={selectedFile}
        headerPinned={headerPinned}
        onSelectFile={handleSelectFileFromPanel}
        onToggleHeaderPinned={() => setHeaderPinned((pinned) => !pinned)}
        onRequestCreate={onRequestBookSettingCreate}
      />
      {latestChapterPlan ? (
        <button
          type="button"
          title={latestChapterPlan.title}
          aria-current={selectedFile === latestChapterPlan.path ? 'page' : undefined}
          className={`nova-nav-item flex w-full items-center gap-2 border px-2 py-1.5 text-left ${
            selectedFile === latestChapterPlan.path
              ? 'is-active border-[var(--nova-border)]'
              : 'border-transparent bg-[var(--nova-surface-2)]'
          }`}
          onClick={() => handleSelectFileFromPanel(latestChapterPlan.path)}
        >
          <FileText className={`h-3.5 w-3.5 shrink-0 ${selectedFile === latestChapterPlan.path ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'}`} />
          <span className="min-w-0 flex-1 truncate text-[11px] font-medium text-[var(--nova-text)]">{latestChapterPlan.title}</span>
        </button>
      ) : null}
    </div>
  )

  return (
    <div ref={rootRef} className="flex h-full min-h-0 flex-col">
      {headerPinned ? (
        <div className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 pb-2">
          {bookSettingsHeader}
        </div>
      ) : null}
      <div
        ref={scrollContainerRef}
        role="navigation"
        aria-label={t('planning.outlineNavigation')}
        className="min-h-0 flex-1 overflow-y-auto p-2"
      >
        <div className="space-y-3">
          {!headerPinned ? (
            <div className="border-b border-[var(--nova-border)] pb-3">
              {bookSettingsHeader}
            </div>
          ) : null}

          {chapterPlans.length === 0 ? (
            <section className="flex items-center justify-between gap-2 px-1 py-1 text-[11px] font-medium text-[var(--nova-text-faint)]">
              <span>{t('planning.chapterPlans')}</span>
              <span>{t('planning.chapterPlansEmpty')}</span>
            </section>
          ) : historicalChapterPlans.length > 0 ? (
            <section className="space-y-1">
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
                    <PlanningListItem key={plan.path} document={plan} selected={selectedFile === plan.path} onSelectFile={handleSelectFileFromPanel} />
                  ))}
                </div>
              ) : null}
            </section>
          ) : null}

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

          <div className="pointer-events-none sticky bottom-2 z-20 h-0">
            <div className="absolute bottom-0 right-0 flex items-center gap-1.5">
              <button
                type="button"
                aria-label={t('planning.locateLatestChapter')}
                aria-hidden={!latestChapterOffscreen}
                tabIndex={latestChapterOffscreen ? 0 : -1}
                title={t('planning.locateLatestChapter')}
                onClick={() => { if (latestChapterPath) locateChapter(latestChapterPath, 'smooth') }}
                className={`${floaterButtonBaseClass} gap-1 px-2 text-[10px] ${latestChapterOffscreen ? 'opacity-90 hover:opacity-100' : 'pointer-events-none translate-y-1 opacity-0'}`}
              >
                <ArrowDownToLine className="h-3.5 w-3.5" />
                <span>{t('planning.latestChapter')}</span>
              </button>
              <button
                type="button"
                aria-label={t('planning.locateCurrentChapter')}
                aria-hidden={!activeChapterOffscreen}
                tabIndex={activeChapterOffscreen ? 0 : -1}
                title={t('planning.locateCurrentChapter')}
                onClick={() => { if (selectedChapterPath) locateChapter(selectedChapterPath, 'smooth') }}
                className={`${floaterButtonBaseClass} w-8 ${activeChapterOffscreen ? 'opacity-80 hover:opacity-100' : 'pointer-events-none translate-y-1 opacity-0'}`}
              >
                <Crosshair className="h-3.5 w-3.5" />
              </button>
              <button
                type="button"
                aria-label={t('planning.backToTop')}
                aria-hidden={!backToTopVisible}
                tabIndex={backToTopVisible ? 0 : -1}
                title={t('planning.backToTop')}
                onClick={scrollToTop}
                className={`${floaterButtonBaseClass} w-8 ${backToTopVisible ? 'opacity-80 hover:opacity-100' : 'pointer-events-none translate-y-1 opacity-0'}`}
              >
                <ArrowUp className="h-3.5 w-3.5" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

function PlanningListItem({
  document,
  selected,
  onSelectFile,
}: {
  document: DocumentPreview
  selected: boolean
  onSelectFile: (path: string) => void | Promise<void>
}) {
  return (
    <button
      type="button"
      title={document.title}
      className={`nova-nav-item w-full border px-3 py-2 text-left ${
        selected
          ? 'is-active border-[var(--nova-border)]'
          : 'border-transparent bg-[var(--nova-surface)]'
      }`}
      onClick={() => onSelectFile(document.path)}
    >
      <div className="flex min-w-0 items-center gap-2">
        <FileText className={`h-3.5 w-3.5 shrink-0 ${selected ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'}`} />
        <span className="min-w-0 flex-1 truncate text-xs font-medium">{document.title}</span>
      </div>
    </button>
  )
}

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
