import { useEffect, useMemo, useState } from 'react'
import { DndContext, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { ArrowDownToLine, ArrowUp, BookMarked, CircleDashed, GripVertical, MessageCircle, Pin, PinOff, Search, Settings2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { FileNode } from '@/hooks/useWorkspace'
import type { DocumentPreview } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { OutlineFileActions } from './outline/OutlineFileActions'
import { flattenFileTree } from './workbench-utils'

const STORAGE_PREFIX = 'nova.outline.pinned-settings:'
const PINNED_STORAGE_VERSION = 3
const LEGACY_DEFAULT_PINNED_PATHS = ['setting/outline.md', 'CREATOR.md', 'setting/progress.md']
const LORE_TAB_PIN_KEY = '@lore-tab'
const DEFAULT_PINNED_PATHS = [...LEGACY_DEFAULT_PINNED_PATHS, 'ideas.md', LORE_TAB_PIN_KEY, 'setting/character-states.md']
const CURRENT_CHAPTER_PLAN_PIN_KEY = '@current-chapter-plan'

interface BookSettingFileItem {
  kind: 'file'
  path: string
  title: string
  exists: boolean
  pinKey?: string
}

interface BookSettingLoreItem {
  kind: 'lore'
  title: string
  exists: true
  pinKey: typeof LORE_TAB_PIN_KEY
  description: string
}

type BookSettingItem = BookSettingFileItem | BookSettingLoreItem

interface BookSettingsShortcutsProps {
  workspace: string
  tree: FileNode[]
  outline?: DocumentPreview
  ideas?: DocumentPreview
  chapterPlans: DocumentPreview[]
  selectedFile: string | null
  headerPinned: boolean
  onSelectFile: (path: string) => void | Promise<void>
  onOpenLoreTab?: () => void | Promise<boolean | void>
  loreTabActive?: boolean
  onToggleHeaderPinned: () => void
  latestChapterAvailable?: boolean
  backToTopAvailable?: boolean
  onLocateLatestChapter?: () => void
  onBackToTop?: () => void
  onReferenceFile?: (path: string) => void
  onRevealFile?: (path: string) => void | Promise<void>
  onRenameItem?: (path: string, newName: string) => Promise<void>
  onDeleteItem?: (path: string) => Promise<void>
  onRequestCreate?: (item: { path: string; title: string }) => void
}

/** 工作区级书籍设定收藏：动态发现文件，并持久化 Pin 与排序偏好。 */
export function BookSettingsShortcuts({
  workspace,
  tree,
  outline,
  ideas,
  chapterPlans,
  selectedFile,
  headerPinned,
  onSelectFile,
  onOpenLoreTab,
  loreTabActive = false,
  onToggleHeaderPinned,
  latestChapterAvailable = false,
  backToTopAvailable = false,
  onLocateLatestChapter,
  onBackToTop,
  onReferenceFile,
  onRevealFile,
  onRenameItem,
  onDeleteItem,
  onRequestCreate,
}: BookSettingsShortcutsProps) {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const [pinnedPaths, setPinnedPaths] = useState<string[]>(() => readPinnedPaths(workspace))
  const [missingItem, setMissingItem] = useState<BookSettingItem | null>(null)
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }))
  const candidates = useMemo(() => discoverBookSettings({ tree, outline, ideas, chapterPlans, t }), [chapterPlans, ideas, outline, t, tree])
  const candidatesByPinKey = useMemo(() => new Map(candidates.map((item) => [bookSettingPinKey(item), item])), [candidates])
  const visibleMissingItem = missingItem?.kind === 'file'
    && !candidatesByPinKey.get(bookSettingPinKey(missingItem))?.exists
    ? missingItem
    : null

  useEffect(() => {
    setPinnedPaths(readPinnedPaths(workspace))
    setMissingItem(null)
  }, [workspace])

  useEffect(() => {
    if (!workspace) return
    window.localStorage.setItem(STORAGE_PREFIX + workspace, JSON.stringify({ version: PINNED_STORAGE_VERSION, paths: pinnedPaths }))
  }, [pinnedPaths, workspace])

  const pinnedItems = pinnedPaths.map((key) => candidatesByPinKey.get(key)).filter((item): item is BookSettingItem => Boolean(item))
  const orderedCandidates = [...pinnedItems, ...candidates.filter((item) => !pinnedPaths.includes(bookSettingPinKey(item)))]
  const normalizedQuery = query.trim().toLocaleLowerCase()
  const filteredCandidates = normalizedQuery
    ? orderedCandidates.filter((item) => bookSettingSearchText(item).toLocaleLowerCase().includes(normalizedQuery))
    : orderedCandidates

  const togglePinned = (path: string) => {
    setPinnedPaths((current) => current.includes(path) ? current.filter((item) => item !== path) : [...current, path])
  }

  const selectItem = (item: BookSettingItem) => {
    if (item.kind === 'lore') {
      setMissingItem(null)
      void onOpenLoreTab?.()
      return
    }
    if (!item.exists) {
      setMissingItem(item)
      return
    }
    setMissingItem(null)
    void onSelectFile(item.path)
  }

  const handleDragEnd = (event: DragEndEvent) => {
    if (!event.over || event.active.id === event.over.id) return
    setPinnedPaths((current) => {
      const from = current.indexOf(String(event.active.id))
      const to = current.indexOf(String(event.over?.id))
      return from < 0 || to < 0 ? current : arrayMove(current, from, to)
    })
  }

  return (
    <section className="space-y-1.5">
      <div className="nova-book-settings-header-row flex items-center justify-between gap-2 px-1">
        <span className="shrink-0 whitespace-nowrap text-[11px] font-medium text-[var(--nova-text-faint)]">{t('planning.bookSettings')}</span>
        <div className="nova-book-settings-header-actions flex shrink-0 items-center gap-0.5">
          {onLocateLatestChapter && latestChapterAvailable ? (
            <TooltipIconButton
              label={t('planning.locateLatestChapter')}
              size="icon-sm"
              tooltipSide="bottom"
              className="text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
              disabled={!latestChapterAvailable}
              onClick={onLocateLatestChapter}
            >
              <ArrowDownToLine className="h-3.5 w-3.5" />
            </TooltipIconButton>
          ) : null}
          {onBackToTop ? (
            <TooltipIconButton
              label={t('planning.backToTop')}
              size="icon-sm"
              tooltipSide="bottom"
              disabled={!backToTopAvailable}
              className="text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:opacity-35"
              onClick={onBackToTop}
            >
              <ArrowUp className="h-3.5 w-3.5" />
            </TooltipIconButton>
          ) : null}
          <TooltipIconButton
            label={t(headerPinned ? 'planning.unpinBookSettingsHeader' : 'planning.pinBookSettingsHeader')}
            size="icon-sm"
            tooltipSide="bottom"
            aria-pressed={headerPinned}
            className="text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
            onClick={onToggleHeaderPinned}
          >
            {headerPinned ? <PinOff className="h-3.5 w-3.5" /> : <Pin className="h-3.5 w-3.5" />}
          </TooltipIconButton>
          <Popover>
            <PopoverTrigger asChild>
              <Button variant="ghost" size="sm" className="h-7 gap-1 px-2 text-[10px] text-[var(--nova-text-faint)]" aria-label={t('planning.manageBookSettings')} title={t('planning.manageBookSettings')}>
                <Settings2 className="h-3 w-3" />
                <span className="nova-book-settings-manage-label">{t('planning.manageBookSettings')}</span>
              </Button>
            </PopoverTrigger>
            <PopoverContent align="start" className="w-80 border-[var(--nova-border)] bg-[var(--nova-menu-bg)]">
              <div className="space-y-2">
                <div>
                  <div className="text-xs font-medium text-[var(--nova-text)]">{t('planning.manageBookSettingsTitle')}</div>
                  <p className="mt-0.5 text-[10px] text-[var(--nova-text-faint)]">{t('planning.manageBookSettingsDescription')}</p>
                </div>
                <div className="relative">
                  <Search className="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--nova-text-faint)]" />
                  <Input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={t('planning.searchBookSettings')} className="h-8 pl-7 text-xs" />
                </div>
                <div className="max-h-72 space-y-1 overflow-y-auto pr-1">
                  <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
                    <SortableContext items={pinnedPaths} strategy={verticalListSortingStrategy}>
                      {filteredCandidates.map((item) => (
                        <SortableSettingRow
                          key={bookSettingPinKey(item)}
                          item={item}
                          pinned={pinnedPaths.includes(bookSettingPinKey(item))}
                          selected={isBookSettingSelected(item, selectedFile, loreTabActive)}
                          onSelect={selectItem}
                          onTogglePinned={togglePinned}
                        />
                      ))}
                    </SortableContext>
                  </DndContext>
                  {filteredCandidates.length === 0 && (
                    <div className="py-4 text-center text-xs text-[var(--nova-text-faint)]">{t('planning.noMatchingBookSettings')}</div>
                  )}
                </div>
              </div>
            </PopoverContent>
          </Popover>
        </div>
      </div>
      {pinnedItems.length > 0 ? (
        <div data-testid="book-setting-shortcuts" className="grid grid-cols-[repeat(auto-fill,minmax(4rem,1fr))] gap-1">
          {pinnedItems.map((item) => {
            const selected = isBookSettingSelected(item, selectedFile, loreTabActive)
            const shortcut = (
              <button
                type="button"
                data-book-setting-state={item.exists ? 'ready' : 'missing'}
                aria-label={item.kind === 'file' && !item.exists ? t('planning.bookSettingMissingTooltip', { title: item.title, path: item.path }) : undefined}
                className={`nova-nav-item relative w-full max-w-full px-2.5 py-1 text-center text-[11px] font-medium ${selected ? 'is-active' : item.exists ? 'bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]' : 'border border-dashed border-[color-mix(in_srgb,var(--nova-warning)_45%,var(--nova-border))] bg-[color-mix(in_srgb,var(--nova-warning-bg)_42%,var(--nova-surface-2))] text-[var(--nova-text-muted)] hover:border-[var(--nova-warning)] hover:bg-[var(--nova-warning-bg)]'}`}
                title={item.kind === 'file' && !item.exists ? t('planning.bookSettingMissingTooltip', { title: item.title, path: item.path }) : item.title}
                onClick={() => selectItem(item)}
              >
                <span className="flex items-center justify-center gap-1 truncate">
                  {item.kind === 'lore' ? <BookMarked aria-hidden="true" className="h-3 w-3 shrink-0 text-emerald-500" /> : null}
                  <span className="truncate">{item.title}</span>
                </span>
                {!item.exists ? <CircleDashed aria-hidden="true" className="absolute right-1.5 top-1/2 h-3 w-3 -translate-y-1/2 text-[color-mix(in_srgb,var(--nova-warning)_68%,var(--nova-text-faint))]" /> : null}
              </button>
            )
            if (item.kind === 'lore') return <div key={bookSettingPinKey(item)}>{shortcut}</div>
            return (
              <OutlineFileActions
                key={bookSettingPinKey(item)}
                path={item.path}
                showTrigger={false}
                onReferenceFile={item.exists ? onReferenceFile : undefined}
                onRevealFile={item.exists ? onRevealFile : undefined}
                onRenameItem={item.exists ? onRenameItem : undefined}
                onDeleteItem={item.exists ? onDeleteItem : undefined}
              >
                {shortcut}
              </OutlineFileActions>
            )
          })}
        </div>
      ) : (
        <div className="rounded border border-dashed border-[var(--nova-border)] px-2 py-2 text-center text-[10px] text-[var(--nova-text-faint)]">
          {t('planning.noPinnedBookSettings')}
        </div>
      )}
      {visibleMissingItem ? (
        <div role="status" className="rounded-[var(--nova-radius)] border border-[var(--nova-warning)]/25 bg-[var(--nova-warning-bg)] px-2.5 py-2">
          <div className="text-[11px] font-medium text-[var(--nova-warning)]">{t('planning.bookSettingMissingTitle', { title: visibleMissingItem.title })}</div>
          <div className="mt-0.5 text-[10px] leading-4 text-[var(--nova-text-muted)]">{t('planning.bookSettingMissingDescription', { path: visibleMissingItem.path })}</div>
          {onRequestCreate ? (
            <Button type="button" variant="ghost" size="xs" className="mt-1.5 h-6 gap-1 px-1.5 text-[10px] text-[var(--nova-warning)] hover:bg-[var(--nova-hover)]" onClick={() => onRequestCreate(visibleMissingItem)}>
              <MessageCircle className="h-3 w-3" />
              {t('planning.bookSettingAskAgent')}
            </Button>
          ) : null}
        </div>
      ) : null}
    </section>
  )
}

function SortableSettingRow({ item, pinned, selected, onSelect, onTogglePinned }: {
  item: BookSettingItem
  pinned: boolean
  selected: boolean
  onSelect: (item: BookSettingItem) => void
  onTogglePinned: (path: string) => void
}) {
  const { t } = useTranslation()
  const pinKey = bookSettingPinKey(item)
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: pinKey, disabled: !pinned })
  const missingTooltip = item.kind === 'file'
    ? t('planning.bookSettingMissingTooltip', { title: item.title, path: item.path })
    : undefined
  return (
    <div ref={setNodeRef} style={{ transform: CSS.Transform.toString(transform), transition }} className={`flex items-center gap-1 rounded-md border px-1 py-1 ${selected ? 'border-[var(--nova-border)] bg-[var(--nova-active)]' : item.exists ? 'border-transparent bg-[var(--nova-surface)]' : 'border-dashed border-[color-mix(in_srgb,var(--nova-warning)_45%,var(--nova-border))] bg-[color-mix(in_srgb,var(--nova-warning-bg)_32%,var(--nova-surface))]'} ${isDragging ? 'z-10 opacity-70 shadow-lg' : ''}`}>
      <button type="button" disabled={!pinned} aria-label={t('planning.reorderBookSetting', { title: item.title })} className="cursor-grab p-1 text-[var(--nova-text-faint)] disabled:cursor-default disabled:opacity-20" {...attributes} {...listeners}>
        <GripVertical className="h-3.5 w-3.5" />
      </button>
      <button type="button" data-book-setting-state={item.exists ? 'ready' : 'missing'} aria-label={item.exists ? undefined : missingTooltip} title={item.exists ? undefined : missingTooltip} className="min-w-0 flex-1 px-1 text-left" onClick={() => onSelect(item)}>
        <span className="flex items-center gap-1 truncate text-xs text-[var(--nova-text)]">
          {item.kind === 'lore' ? <BookMarked aria-hidden="true" className="h-3 w-3 shrink-0 text-emerald-500" /> : null}
          <span className="truncate">{item.title}</span>
        </span>
        <span className="flex min-w-0 items-center gap-1 truncate text-[10px] text-[var(--nova-text-faint)]">
          {!item.exists ? <CircleDashed aria-hidden="true" className="h-3 w-3 shrink-0 text-[color-mix(in_srgb,var(--nova-warning)_68%,var(--nova-text-faint))]" /> : null}
          <span className="truncate">{item.kind === 'file' ? item.path : item.description}</span>
        </span>
      </button>
      <button type="button" aria-label={pinned ? t('planning.unpinBookSetting', { title: item.title }) : t('planning.pinBookSetting', { title: item.title })} className="rounded p-1.5 text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" onClick={() => onTogglePinned(pinKey)}>
        {pinned ? <PinOff className="h-3.5 w-3.5" /> : <Pin className="h-3.5 w-3.5" />}
      </button>
    </div>
  )
}

function discoverBookSettings({ tree, outline, ideas, chapterPlans, t }: {
  tree: FileNode[]
  outline?: DocumentPreview
  ideas?: DocumentPreview
  chapterPlans: DocumentPreview[]
  t: (key: string) => string
}): BookSettingItem[] {
  const paths = flattenFileTree(tree)
  const existingPaths = new Set(paths)
  const outlinePath = outline?.path ?? 'setting/outline.md'
  const ideasPath = ideas?.path ?? 'ideas.md'
  const latestChapterPlan = chapterPlans[chapterPlans.length - 1]
  const known = new Map<string, BookSettingItem>([
    [outlinePath, { kind: 'file', path: outlinePath, title: t('planning.outlineTab'), exists: Boolean(outline) || existingPaths.has(outlinePath) }],
    ['CREATOR.md', { kind: 'file', path: 'CREATOR.md', title: t('planning.creatorRulesTab'), exists: existingPaths.has('CREATOR.md') }],
    ['setting/progress.md', { kind: 'file', path: 'setting/progress.md', title: t('planning.writingProgressTab'), exists: existingPaths.has('setting/progress.md') }],
    [ideasPath, { kind: 'file', path: ideasPath, title: t('planning.ideas'), exists: Boolean(ideas) || existingPaths.has(ideasPath) }],
    [LORE_TAB_PIN_KEY, { kind: 'lore', title: t('planning.loreSettings'), exists: true, pinKey: LORE_TAB_PIN_KEY, description: t('tab.loreTitle') }],
    ['setting/character-states.md', { kind: 'file', path: 'setting/character-states.md', title: t('planning.characterStates'), exists: existingPaths.has('setting/character-states.md') }],
  ])
  if (latestChapterPlan) {
    known.set(CURRENT_CHAPTER_PLAN_PIN_KEY, {
      kind: 'file',
      path: latestChapterPlan.path,
      title: t('planning.currentChapterPlan'),
      exists: true,
      pinKey: CURRENT_CHAPTER_PLAN_PIN_KEY,
    })
  }
  const chapterPlanPaths = new Set(chapterPlans.map((plan) => plan.path))
  for (const path of paths) {
    if (!isBookSettingPath(path, chapterPlanPaths)) continue
    const current = known.get(path)
    known.set(path, current ? { ...current, exists: true } : { kind: 'file', path, title: titleFromPath(path), exists: true })
  }
  return [...known.values()]
}

function isBookSettingPath(path: string, chapterPlanPaths: Set<string>) {
  const normalized = path.toLocaleLowerCase()
  return normalized.endsWith('.md')
    && !normalized.startsWith('chapters/')
    && !normalized.startsWith('interactive/')
    && !normalized.startsWith('.nova/')
    && !normalized.startsWith('.denova/')
    && !chapterPlanPaths.has(path)
}

function titleFromPath(path: string) {
  const name = path.split('/').pop() || path
  return name.replace(/\.md$/i, '').replace(/[-_]+/g, ' ')
}

function bookSettingPinKey(item: BookSettingItem) {
  return item.kind === 'lore' ? item.pinKey : item.pinKey ?? item.path
}

function bookSettingSearchText(item: BookSettingItem) {
  return item.kind === 'file' ? `${item.title} ${item.path}` : `${item.title} ${item.description}`
}

function isBookSettingSelected(item: BookSettingItem, selectedFile: string | null, loreTabActive: boolean) {
  return item.kind === 'lore' ? loreTabActive : item.exists && selectedFile === item.path
}

function readPinnedPaths(workspace: string) {
  if (!workspace) return DEFAULT_PINNED_PATHS
  try {
    const parsed = JSON.parse(window.localStorage.getItem(STORAGE_PREFIX + workspace) || 'null')
    if (parsed?.version === PINNED_STORAGE_VERSION && isStringArray(parsed.paths)) {
      return parsed.paths
    }
    if (parsed?.version === 2 && isStringArray(parsed.paths)) {
      return withLoreTab(parsed.paths)
    }
    if (isStringArray(parsed)) {
      return samePaths(parsed, LEGACY_DEFAULT_PINNED_PATHS) ? DEFAULT_PINNED_PATHS : withLoreTab(parsed)
    }
    return DEFAULT_PINNED_PATHS
  } catch {
    return DEFAULT_PINNED_PATHS
  }
}

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((item) => typeof item === 'string')
}

function withLoreTab(paths: string[]) {
  if (paths.includes(LORE_TAB_PIN_KEY)) return paths
  const ideasIndex = paths.indexOf('ideas.md')
  const insertAt = ideasIndex < 0 ? paths.length : ideasIndex + 1
  return [...paths.slice(0, insertAt), LORE_TAB_PIN_KEY, ...paths.slice(insertAt)]
}

function samePaths(left: string[], right: string[]) {
  return left.length === right.length && left.every((path, index) => path === right[index])
}
