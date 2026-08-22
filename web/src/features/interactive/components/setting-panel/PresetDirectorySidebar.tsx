import { DndContext, DragOverlay, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent, type DragStartEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { useEffect, useMemo, useState, type CSSProperties } from 'react'
import { Bot, ChevronDown, ChevronsDownUp, ChevronsUpDown, FileText, Plus, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ResourceDirectoryItem, ResourceDirectorySection } from '@/components/resource-directory/types'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Empty, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupAction,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInput,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarSeparator,
} from '@/components/ui/sidebar'
import { cn } from '@/lib/utils'
import { presetModuleOwnership, type PresetResourceKind } from '../../preset-ownership'

interface PresetDirectorySidebarProps {
  sections: ResourceDirectorySection[]
  activeId: string | null
  activeSectionId: PresetResourceKind
  agentEntryId: string
  saving?: boolean
  onSelect: (id: string) => void
  onReorderItems: (sectionId: string, orderedItemIds: string[]) => void
}

interface VisiblePresetSection {
  section: ResourceDirectorySection
  items: ResourceDirectoryItem[]
}

function filterPresetItem(item: ResourceDirectoryItem, words: string[]) {
  const searchable = `${item.title}\n${item.summary ?? ''}\n${item.searchText ?? ''}`.toLocaleLowerCase()
  return words.every((word) => searchable.includes(word))
}

/** Preset-specific navigation composed from shadcn Sidebar primitives. */
export function PresetDirectorySidebar({
  sections,
  activeId,
  activeSectionId,
  agentEntryId,
  saving = false,
  onSelect,
  onReorderItems,
}: PresetDirectorySidebarProps) {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const [openSections, setOpenSections] = useState<Record<string, boolean>>({})
  const [draggingItem, setDraggingItem] = useState<ResourceDirectoryItem | null>(null)
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  useEffect(() => {
    setOpenSections((current) => ({ ...current, [activeSectionId]: true }))
  }, [activeSectionId])

  const searchWords = useMemo(
    () => query.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean),
    [query],
  )
  const searching = searchWords.length > 0
  const visibleSections = useMemo<VisiblePresetSection[]>(() => sections
    .map((section) => ({
      section,
      items: searching ? section.items.filter((item) => filterPresetItem(item, searchWords)) : section.items,
    }))
    .filter(({ items }) => !searching || items.length > 0), [searchWords, searching, sections])

  useEffect(() => {
    if (!searching) return
    setOpenSections((current) => {
      if (visibleSections.every(({ section }) => current[section.id] === true)) return current
      return {
        ...current,
        ...Object.fromEntries(visibleSections.map(({ section }) => [section.id, true])),
      }
    })
  }, [searching, visibleSections])

  const totalCount = sections.reduce((count, section) => count + section.items.length, 0)
  const sectionIsOpen = (section: ResourceDirectorySection) => (
    openSections[section.id] ?? (searching || section.id === activeSectionId || section.defaultCollapsed === false)
  )
  const allClosed = visibleSections.length > 0 && visibleSections.every(({ section }) => !sectionIsOpen(section))
  const toggleAllSections = () => {
    const nextOpen = allClosed
    setOpenSections((current) => ({
      ...current,
      ...Object.fromEntries(visibleSections.map(({ section }) => [section.id, nextOpen])),
    }))
  }

  const handleDragStart = (event: DragStartEvent) => {
    const activeItemId = String(event.active.id)
    setDraggingItem(visibleSections.flatMap(({ items }) => items).find((item) => item.id === activeItemId) ?? null)
  }

  const handleDragEnd = (event: DragEndEvent) => {
    setDraggingItem(null)
    if (!event.over || event.active.id === event.over.id) return
    const activeItemId = String(event.active.id)
    const overItemId = String(event.over.id)
    const entry = visibleSections.find(({ section, items }) => (
      section.reorderable
      && items.some((item) => item.id === activeItemId)
      && items.some((item) => item.id === overItemId)
    ))
    if (!entry) return
    const itemIds = entry.items.map((item) => item.id)
    onReorderItems(entry.section.id, arrayMove(itemIds, itemIds.indexOf(activeItemId), itemIds.indexOf(overItemId)))
  }

  return (
    <SidebarProvider
      className="preset-directory nova-sidebar h-full min-h-0 w-full overflow-hidden"
      style={{ '--sidebar-width': '100%' } as CSSProperties}
    >
      <Sidebar collapsible="none" className="w-full">
        <SidebarHeader className="gap-3 p-3">
          <div className="flex items-center gap-3 px-1">
            <div className="flex min-w-0 flex-1 flex-col gap-0.5">
              <span className="truncate text-sm font-medium text-sidebar-foreground">{t('settingPanel.tellerDirectory')}</span>
              <span className="truncate text-xs text-muted-foreground">{t('settingPanel.directory.subtitle')}</span>
            </div>
            <Badge variant="secondary" aria-label={t('settingPanel.directory.count', { count: totalCount })}>
              {totalCount}
            </Badge>
          </div>

          <div className="flex items-center gap-2">
            <div className="relative min-w-0 flex-1">
              <SidebarInput
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={t('settingPanel.directory.search')}
                aria-label={t('settingPanel.directory.search')}
                className="pl-8"
              />
              <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
            </div>
            <Button
              type="button"
              size="icon-sm"
              variant="outline"
              disabled={visibleSections.length === 0}
              onClick={toggleAllSections}
              aria-label={allClosed ? t('common.expandAll') : t('common.collapseAll')}
            >
              {allClosed ? <ChevronsUpDown data-icon="inline-start" /> : <ChevronsDownUp data-icon="inline-start" />}
            </Button>
          </div>
        </SidebarHeader>

        <SidebarSeparator />
        <SidebarContent>
          {visibleSections.length === 0 ? (
            <Empty className="border-0 p-4 text-muted-foreground">
              <EmptyHeader>
                <EmptyMedia variant="icon"><Search aria-hidden="true" /></EmptyMedia>
                <EmptyTitle>{t('common.searchNoResults')}</EmptyTitle>
              </EmptyHeader>
            </Empty>
          ) : (
            <DndContext
              sensors={sensors}
              collisionDetection={closestCenter}
              onDragStart={handleDragStart}
              onDragCancel={() => setDraggingItem(null)}
              onDragEnd={handleDragEnd}
            >
              {visibleSections.map(({ section, items }) => {
                const open = sectionIsOpen(section)
                const SectionIcon = section.icon
                const reorderable = Boolean(section.reorderable && !searching && items.length > 1)
                const availability = t(`settingPanel.presetAvailability.${presetModuleOwnership(section.id as PresetResourceKind)}`)
                return (
                  <Collapsible
                    key={section.id}
                    open={open}
                    onOpenChange={(nextOpen) => setOpenSections((current) => ({ ...current, [section.id]: nextOpen }))}
                  >
                    <SidebarGroup className="py-1">
                      <SidebarGroupLabel asChild className="gap-1.5 whitespace-nowrap pr-8">
                        <CollapsibleTrigger
                          aria-label={`${open ? t('common.collapse') : t('common.expand')}${section.label}`}
                          aria-expanded={open}
                          title={`${section.label} · ${items.length} · ${availability}`}
                        >
                          <ChevronDown className={cn('transition-transform', !open && '-rotate-90')} aria-hidden="true" />
                          {SectionIcon && <SectionIcon aria-hidden="true" />}
                          <span className="min-w-0 flex-1 truncate text-sidebar-foreground">{section.label}</span>
                          <span className="flex shrink-0 items-center gap-1.5 font-normal text-sidebar-foreground/50">
                            <span>{items.length}</span>
                            <span>{availability}</span>
                          </span>
                        </CollapsibleTrigger>
                      </SidebarGroupLabel>
                      {section.onCreate && (
                        <SidebarGroupAction
                          type="button"
                          disabled={saving}
                          onClick={section.onCreate}
                          aria-label={section.createLabel ?? `${t('common.create')} ${section.label}`}
                          className="top-2.5"
                        >
                          <Plus aria-hidden="true" />
                        </SidebarGroupAction>
                      )}
                      <CollapsibleContent>
                        <SidebarGroupContent className="pl-2">
                          <SidebarMenu>
                            {reorderable ? (
                              <SortableContext items={items.map((item) => item.id)} strategy={verticalListSortingStrategy}>
                                {items.map((item) => (
                                  <SortablePresetItem
                                    key={item.id}
                                    item={item}
                                    active={activeId === item.id}
                                    onSelect={() => onSelect(item.id)}
                                  />
                                ))}
                              </SortableContext>
                            ) : items.map((item) => (
                              <PresetItem
                                key={item.id}
                                item={item}
                                active={activeId === item.id}
                                onSelect={() => onSelect(item.id)}
                              />
                            ))}
                          </SidebarMenu>
                        </SidebarGroupContent>
                      </CollapsibleContent>
                    </SidebarGroup>
                  </Collapsible>
                )
              })}
              <DragOverlay>{draggingItem ? <PresetItemDragOverlay item={draggingItem} /> : null}</DragOverlay>
            </DndContext>
          )}
        </SidebarContent>

        <SidebarSeparator />
        <SidebarFooter>
          <Button
            type="button"
            size="sm"
            variant={activeId === agentEntryId ? 'secondary' : 'outline'}
            aria-pressed={activeId === agentEntryId}
            onClick={() => onSelect(agentEntryId)}
          >
            <Bot data-icon="inline-start" />
            {t('settingPanel.tellerAgent.title')}
          </Button>
        </SidebarFooter>
      </Sidebar>
    </SidebarProvider>
  )
}

function SortablePresetItem({ item, active, onSelect }: { item: ResourceDirectoryItem; active: boolean; onSelect: () => void }) {
  const { attributes, listeners, setActivatorNodeRef, setNodeRef, transform, transition, isDragging } = useSortable({ id: item.id })
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        ref={(node) => {
          setNodeRef(node)
          setActivatorNodeRef(node)
        }}
        type="button"
        size="lg"
        style={{ transform: CSS.Transform.toString(transform), transition }}
        isActive={active}
        className={cn('cursor-grab', isDragging && 'opacity-35')}
        aria-current={active ? 'true' : undefined}
        title={item.summary ? `${item.title}\n${item.summary}` : item.title}
        onClick={onSelect}
        {...attributes}
        {...listeners}
      >
        <PresetItemContent item={item} />
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}

function PresetItem({ item, active, onSelect }: { item: ResourceDirectoryItem; active: boolean; onSelect: () => void }) {
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        type="button"
        size="lg"
        isActive={active}
        aria-current={active ? 'true' : undefined}
        title={item.summary ? `${item.title}\n${item.summary}` : item.title}
        onClick={onSelect}
      >
        <PresetItemContent item={item} />
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}

function PresetItemContent({ item }: { item: ResourceDirectoryItem }) {
  return (
    <>
      <FileText aria-hidden="true" />
      <span className="grid min-w-0 flex-1 gap-0.5">
        <span className="truncate text-xs text-sidebar-foreground">{item.title}</span>
        {item.summary && <span className="truncate text-xs text-sidebar-foreground/60">{item.summary}</span>}
      </span>
    </>
  )
}

function PresetItemDragOverlay({ item }: { item: ResourceDirectoryItem }) {
  return (
    <div className="flex w-64 items-center gap-2 rounded-md bg-sidebar p-2 text-sm text-sidebar-foreground shadow-lg ring-1 ring-sidebar-border">
      <PresetItemContent item={item} />
    </div>
  )
}
