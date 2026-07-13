import { useEffect, useMemo, useState } from 'react'
import { DndContext, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { GripVertical, MessageCircle, Pin, PinOff, Search, Settings2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { FileNode } from '@/hooks/useWorkspace'
import type { DocumentPreview } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { flattenFileTree } from './workbench-utils'

const STORAGE_PREFIX = 'nova.outline.pinned-settings:'
const PINNED_STORAGE_VERSION = 2
const LEGACY_DEFAULT_PINNED_PATHS = ['setting/outline.md', 'CREATOR.md', 'setting/progress.md']
const DEFAULT_PINNED_PATHS = [...LEGACY_DEFAULT_PINNED_PATHS, 'ideas.md', 'setting/character-states.md']

interface BookSettingItem {
  path: string
  title: string
  exists: boolean
}

interface BookSettingsShortcutsProps {
  workspace: string
  tree: FileNode[]
  outline?: DocumentPreview
  ideas?: DocumentPreview
  chapterPlans: DocumentPreview[]
  selectedFile: string | null
  onSelectFile: (path: string) => void | Promise<void>
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
  onSelectFile,
  onRequestCreate,
}: BookSettingsShortcutsProps) {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const [pinnedPaths, setPinnedPaths] = useState<string[]>(() => readPinnedPaths(workspace))
  const [missingItem, setMissingItem] = useState<BookSettingItem | null>(null)
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }))
  const candidates = useMemo(() => discoverBookSettings({ tree, outline, ideas, chapterPlans, t }), [chapterPlans, ideas, outline, t, tree])
  const candidatesByPath = useMemo(() => new Map(candidates.map((item) => [item.path, item])), [candidates])
  const visibleMissingItem = missingItem && !candidatesByPath.get(missingItem.path)?.exists ? missingItem : null

  useEffect(() => {
    setPinnedPaths(readPinnedPaths(workspace))
    setMissingItem(null)
  }, [workspace])

  useEffect(() => {
    if (!workspace) return
    window.localStorage.setItem(STORAGE_PREFIX + workspace, JSON.stringify({ version: PINNED_STORAGE_VERSION, paths: pinnedPaths }))
  }, [pinnedPaths, workspace])

  const pinnedItems = pinnedPaths.map((path) => candidatesByPath.get(path)).filter((item): item is BookSettingItem => Boolean(item))
  const orderedCandidates = [...pinnedItems, ...candidates.filter((item) => !pinnedPaths.includes(item.path))]
  const normalizedQuery = query.trim().toLocaleLowerCase()
  const filteredCandidates = normalizedQuery
    ? orderedCandidates.filter((item) => `${item.title} ${item.path}`.toLocaleLowerCase().includes(normalizedQuery))
    : orderedCandidates

  const togglePinned = (path: string) => {
    setPinnedPaths((current) => current.includes(path) ? current.filter((item) => item !== path) : [...current, path])
  }

  const selectItem = (item: BookSettingItem) => {
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
      <div className="flex items-center justify-between gap-2 px-1">
        <span className="text-[11px] font-medium text-[var(--nova-text-faint)]">{t('planning.bookSettings')}</span>
        <Popover>
          <PopoverTrigger asChild>
            <Button variant="ghost" size="xs" className="h-6 gap-1 px-1.5 text-[10px] text-[var(--nova-text-faint)]">
              <Settings2 className="h-3 w-3" />
              {t('planning.manageBookSettings')}
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
                        key={item.path}
                        item={item}
                        pinned={pinnedPaths.includes(item.path)}
                        selected={item.exists && selectedFile === item.path}
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
      {pinnedItems.length > 0 ? (
        <div className="flex flex-wrap gap-1">
          {pinnedItems.map((item) => (
            <button
              key={item.path}
              type="button"
              className={`nova-nav-item max-w-full px-2.5 py-1 text-[11px] font-medium ${item.exists && selectedFile === item.path ? 'is-active' : 'bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]'}`}
              title={item.title}
              onClick={() => selectItem(item)}
            >
              <span className="block truncate">{item.title}</span>
            </button>
          ))}
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
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: item.path, disabled: !pinned })
  return (
    <div ref={setNodeRef} style={{ transform: CSS.Transform.toString(transform), transition }} className={`flex items-center gap-1 rounded-md border px-1 py-1 ${selected ? 'border-[var(--nova-border)] bg-[var(--nova-active)]' : 'border-transparent bg-[var(--nova-surface)]'} ${isDragging ? 'z-10 opacity-70 shadow-lg' : ''}`}>
      <button type="button" disabled={!pinned} aria-label={t('planning.reorderBookSetting', { title: item.title })} className="cursor-grab p-1 text-[var(--nova-text-faint)] disabled:cursor-default disabled:opacity-20" {...attributes} {...listeners}>
        <GripVertical className="h-3.5 w-3.5" />
      </button>
      <button type="button" className="min-w-0 flex-1 px-1 text-left" onClick={() => onSelect(item)}>
        <span className="block truncate text-xs text-[var(--nova-text)]">{item.title}</span>
        <span className="block truncate text-[10px] text-[var(--nova-text-faint)]">{item.path}</span>
      </button>
      <button type="button" aria-label={pinned ? t('planning.unpinBookSetting', { title: item.title }) : t('planning.pinBookSetting', { title: item.title })} className="rounded p-1.5 text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" onClick={() => onTogglePinned(item.path)}>
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
  const known = new Map<string, BookSettingItem>([
    [outlinePath, { path: outlinePath, title: t('planning.outlineTab'), exists: Boolean(outline) || existingPaths.has(outlinePath) }],
    ['CREATOR.md', { path: 'CREATOR.md', title: t('planning.creatorRulesTab'), exists: existingPaths.has('CREATOR.md') }],
    ['setting/progress.md', { path: 'setting/progress.md', title: t('planning.writingProgressTab'), exists: existingPaths.has('setting/progress.md') }],
    [ideasPath, { path: ideasPath, title: t('planning.ideas'), exists: Boolean(ideas) || existingPaths.has(ideasPath) }],
    ['setting/character-states.md', { path: 'setting/character-states.md', title: t('planning.characterStates'), exists: existingPaths.has('setting/character-states.md') }],
  ])
  const chapterPlanPaths = new Set(chapterPlans.map((plan) => plan.path))
  for (const path of paths) {
    if (!isBookSettingPath(path, chapterPlanPaths)) continue
    const current = known.get(path)
    known.set(path, current ? { ...current, exists: true } : { path, title: titleFromPath(path), exists: true })
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

function readPinnedPaths(workspace: string) {
  if (!workspace) return DEFAULT_PINNED_PATHS
  try {
    const parsed = JSON.parse(window.localStorage.getItem(STORAGE_PREFIX + workspace) || 'null')
    if (parsed?.version === PINNED_STORAGE_VERSION && Array.isArray(parsed.paths) && parsed.paths.every((item: unknown) => typeof item === 'string')) {
      return parsed.paths
    }
    if (Array.isArray(parsed) && parsed.every((item) => typeof item === 'string')) {
      return samePaths(parsed, LEGACY_DEFAULT_PINNED_PATHS) ? DEFAULT_PINNED_PATHS : parsed
    }
    return DEFAULT_PINNED_PATHS
  } catch {
    return DEFAULT_PINNED_PATHS
  }
}

function samePaths(left: string[], right: string[]) {
  return left.length === right.length && left.every((path, index) => path === right[index])
}
