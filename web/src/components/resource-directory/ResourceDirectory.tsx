import { DndContext, DragOverlay, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent, type DragStartEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { ChevronDown, ChevronsDownUp, ChevronsUpDown, FileText, Plus, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useControllableState } from '@radix-ui/react-use-controllable-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Empty, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { EmbeddedSidebar } from '@/components/navigation/embedded-sidebar'
import {
  SidebarContent,
  SidebarGroup,
  SidebarGroupAction,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarSeparator,
} from '@/components/ui/sidebar'
import { cn } from '@/lib/utils'
import type { ResourceDirectoryBadge, ResourceDirectoryItem, ResourceDirectoryPinnedEntry, ResourceDirectorySection } from './types'

/** 默认匹配 title + summary + searchText，空格分词后取交集。 */
function defaultFilterItem(item: ResourceDirectoryItem, query: string): boolean {
  const haystack = `${item.title}\n${item.summary ?? ''}\n${item.searchText ?? ''}`.toLowerCase()
  return query.toLowerCase().split(/\s+/).every((word) => haystack.includes(word))
}

interface ResourceDirectoryProps {
  sections: ResourceDirectorySection[]
  activeId: string | null
  onSelect: (id: string) => void
  saving?: boolean
  pinnedEntries?: ResourceDirectoryPinnedEntry[]
  searchPlaceholder?: string
  /** 受控 query（如资料库需要把 query 共享给编辑器做高亮）；不传则组件内部自持 */
  query?: string
  onQueryChange?: (value: string) => void
  /** 自定义条目过滤；缺省匹配 title + summary + searchText */
  filterItem?: (item: ResourceDirectoryItem, query: string) => boolean
  /** 嵌在搜索框尾部的附加控件（如加载方式过滤器） */
  searchAccessory?: ReactNode
  /** 搜索行右侧的附加按钮（如批量生成、分类） */
  headerActions?: ReactNode
  /** 展示「展开/收起全部」按钮 */
  showExpandCollapseAll?: boolean
  /** 值变化时强制展开对应分组（如方案预设切换资源类型） */
  expandedSectionId?: string
  /** 空分组沉底展示（资料库语义）；缺省保持传入顺序 */
  emptySectionsLast?: boolean
  /** Some catalogs need grouping without a search affordance. */
  showSearch?: boolean
  /** Domain toolbar rendered before search and pinned entries. */
  headerContent?: ReactNode
  /** Empty catalog content; search-empty messaging remains built in. */
  emptyContent?: ReactNode
  /** 同一分组内的排序结果；条目 id 按新的显示顺序返回。 */
  onReorderItems?: (sectionId: string, orderedItemIds: string[]) => void
}

/**
 * 统一的资源目录左侧栏：搜索 + 置顶伪条目 + 分组折叠 + 计数 + 组级新建。
 * 资料库、自动化与游戏设定共用；业务数据和拖拽行为由调用方持有。
 */
export function ResourceDirectory({
  sections,
  activeId,
  onSelect,
  saving = false,
  pinnedEntries,
  searchPlaceholder,
  query: queryProp,
  onQueryChange,
  filterItem,
  searchAccessory,
  headerActions,
  showExpandCollapseAll = false,
  expandedSectionId,
  emptySectionsLast = false,
  showSearch = true,
  headerContent,
  emptyContent,
  onReorderItems,
}: ResourceDirectoryProps) {
  const { t } = useTranslation()
  const [query = '', setQuery] = useControllableState({
    prop: queryProp,
    defaultProp: '',
    onChange: onQueryChange,
  })
  const [collapsedSections, setCollapsedSections] = useState<Record<string, boolean>>({})
  const [draggingItem, setDraggingItem] = useState<ResourceDirectoryItem | null>(null)
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  const trimmedQuery = query.trim()
  const searching = trimmedQuery.length > 0
  const filter = filterItem ?? defaultFilterItem

  useEffect(() => {
    if (expandedSectionId) {
      setCollapsedSections((current) => ({ ...current, [expandedSectionId]: false }))
    }
  }, [expandedSectionId])

  const visibleSections = useMemo(() => {
    const mapped = sections.map((section) => ({
      section,
      items: searching ? section.items.filter((item) => filter(item, trimmedQuery)) : section.items,
    }))
    if (!emptySectionsLast) return mapped
    // 空组沉底，非空组保持传入相对顺序
    const withItems = mapped.filter((entry) => entry.items.length > 0)
    const withoutItems = mapped.filter((entry) => entry.items.length === 0)
    return [...withItems, ...withoutItems]
  }, [sections, searching, trimmedQuery, filter, emptySectionsLast])

  const isCollapsed = (section: ResourceDirectorySection, items: ResourceDirectoryItem[]) => {
    if (searching) return collapsedSections[section.id] ?? false
    return collapsedSections[section.id] ?? section.defaultCollapsed ?? items.length === 0
  }

  const allCollapsed = visibleSections.length > 0 && visibleSections.every(({ section, items }) => isCollapsed(section, items))
  const toggleAllSections = () => {
    const next: Record<string, boolean> = {}
    for (const { section } of visibleSections) next[section.id] = !allCollapsed
    setCollapsedSections((current) => ({ ...current, ...next }))
  }

  const totalVisible = visibleSections.reduce((sum, entry) => sum + entry.items.length, 0)

  const handleDragStart = (event: DragStartEvent) => {
    const activeId = String(event.active.id)
    const item = visibleSections.flatMap((entry) => entry.items).find((entry) => entry.id === activeId)
    setDraggingItem(item ?? null)
  }

  const handleDragEnd = (event: DragEndEvent) => {
    setDraggingItem(null)
    if (!event.over || event.active.id === event.over.id || !onReorderItems) return
    const activeId = String(event.active.id)
    const overId = String(event.over.id)
    const entry = visibleSections.find(({ section, items }) => (
      section.reorderable
      && items.some((item) => item.id === activeId)
      && items.some((item) => item.id === overId)
    ))
    if (!entry) return
    const ids = entry.items.map((item) => item.id)
    onReorderItems(entry.section.id, arrayMove(ids, ids.indexOf(activeId), ids.indexOf(overId)))
  }

  const directoryContent = (
    <>
      {searching && totalVisible === 0 ? (
        <Empty className="border-0 p-4 text-muted-foreground">
          <EmptyHeader>
            <EmptyMedia variant="icon"><Search aria-hidden="true" /></EmptyMedia>
            <EmptyTitle>{t('common.searchNoResults')}</EmptyTitle>
          </EmptyHeader>
        </Empty>
      ) : totalVisible === 0 && emptyContent ? (
        emptyContent
      ) : (
        visibleSections.map(({ section, items }) => {
          const SectionIcon = section.icon
          const collapsed = isCollapsed(section, items)
          const reorderable = Boolean(onReorderItems && section.reorderable && !searching && items.length > 1)
          const itemRows = items.map((item) => reorderable ? (
            <SortableDirectoryItemRow
              key={item.id}
              item={item}
              active={activeId === item.id}
              onSelect={() => onSelect(item.id)}
            />
          ) : (
            <DirectoryItemRow
              key={item.id}
              item={item}
              active={activeId === item.id}
              onSelect={() => onSelect(item.id)}
            />
          ))
          return (
            <Collapsible
              key={section.id}
              open={!collapsed}
              onOpenChange={(open) => setCollapsedSections((current) => ({ ...current, [section.id]: !open }))}
            >
              <SidebarGroup className="py-1">
                <SidebarGroupLabel asChild className={cn('gap-1.5', section.onCreate && 'pr-8')}>
                  <CollapsibleTrigger
                    aria-label={`${collapsed ? t('common.expand') : t('common.collapse')}${section.label}`}
                    aria-expanded={!collapsed}
                    title={section.description ?? section.label}
                  >
                    <ChevronDown className={cn('transition-transform', collapsed && '-rotate-90')} aria-hidden="true" />
                    {SectionIcon && <SectionIcon aria-hidden="true" />}
                    <span data-resource-directory-section-label className="min-w-0 flex-1 truncate text-sidebar-foreground">{section.label}</span>
                    <span className="shrink-0 font-normal text-sidebar-foreground/50">{items.length}</span>
                    {section.headerMeta}
                  </CollapsibleTrigger>
                </SidebarGroupLabel>
                {section.onCreate && (
                  <SidebarGroupAction
                    type="button"
                    className="top-2.5"
                    disabled={saving}
                    onClick={section.onCreate}
                    aria-label={section.createLabel ?? `${t('common.create')} ${section.label}`}
                  >
                    <Plus aria-hidden="true" />
                  </SidebarGroupAction>
                )}
                <CollapsibleContent>
                  <SidebarGroupContent className="pl-2">
                    <SidebarMenu>
                      {reorderable ? (
                        <SortableContext items={items.map((item) => item.id)} strategy={verticalListSortingStrategy}>
                          {itemRows}
                        </SortableContext>
                      ) : itemRows}
                    </SidebarMenu>
                  </SidebarGroupContent>
                </CollapsibleContent>
              </SidebarGroup>
            </Collapsible>
          )
        })
      )}
    </>
  )

  const hasHeader = Boolean(headerContent || showSearch || pinnedEntries?.length)

  return (
    <EmbeddedSidebar className="nova-sidebar overflow-hidden">
        {hasHeader && (
          <>
            <SidebarHeader>
              {headerContent}
              {pinnedEntries && pinnedEntries.length > 0 && (
                <SidebarMenu>
                  {pinnedEntries.map((entry) => {
                    const PinnedIcon = entry.icon
                    const active = activeId === entry.id
                    return (
                      <SidebarMenuItem key={entry.id}>
                        <SidebarMenuButton
                          type="button"
                          size={entry.summary ? 'lg' : 'default'}
                          isActive={active}
                          aria-current={active ? 'true' : undefined}
                          onClick={() => onSelect(entry.id)}
                        >
                          <PinnedIcon aria-hidden="true" />
                          <span className="grid min-w-0 flex-1 gap-0.5">
                            <span className="truncate text-sidebar-foreground">{entry.label}</span>
                            {entry.summary ? <span className="truncate text-xs text-sidebar-foreground/60">{entry.summary}</span> : null}
                          </span>
                        </SidebarMenuButton>
                      </SidebarMenuItem>
                    )
                  })}
                </SidebarMenu>
              )}
              {showSearch && (
                <div className="flex items-center gap-2">
                  <InputGroup className="min-w-0 flex-1">
                    <InputGroupAddon><Search /></InputGroupAddon>
                    <InputGroupInput
                      value={query}
                      onChange={(event) => setQuery(event.target.value)}
                      placeholder={searchPlaceholder ?? t('common.search')}
                      aria-label={searchPlaceholder ?? t('common.search')}
                    />
                    {searchAccessory && <InputGroupAddon align="inline-end">{searchAccessory}</InputGroupAddon>}
                  </InputGroup>
                  {showExpandCollapseAll && (
                    <Button
                      type="button"
                      variant="outline"
                      size="icon-sm"
                      onClick={toggleAllSections}
                      aria-label={allCollapsed ? t('common.expandAll') : t('common.collapseAll')}
                    >
                      {allCollapsed ? <ChevronsUpDown /> : <ChevronsDownUp />}
                    </Button>
                  )}
                  {headerActions}
                </div>
              )}
            </SidebarHeader>
            <SidebarSeparator />
          </>
        )}
        <SidebarContent>
          {onReorderItems ? (
            <DndContext
              sensors={sensors}
              collisionDetection={closestCenter}
              onDragStart={handleDragStart}
              onDragCancel={() => setDraggingItem(null)}
              onDragEnd={handleDragEnd}
            >
              {directoryContent}
              <DragOverlay>{draggingItem ? <DirectoryItemDragOverlay item={draggingItem} /> : null}</DragOverlay>
            </DndContext>
          ) : directoryContent}
        </SidebarContent>
    </EmbeddedSidebar>
  )
}

function SortableDirectoryItemRow({ item, active, onSelect }: { item: ResourceDirectoryItem; active: boolean; onSelect: () => void }) {
  const { attributes, listeners, setActivatorNodeRef, setNodeRef, transform, transition, isDragging } = useSortable({ id: item.id })
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        ref={(node) => {
          setNodeRef(node)
          setActivatorNodeRef(node)
        }}
        type="button"
        size={item.summary ? 'lg' : 'default'}
        style={{ transform: CSS.Transform.toString(transform), transition }}
        isActive={active}
        className={cn('cursor-default', item.disabled && 'opacity-50', isDragging && 'opacity-35')}
        onClick={onSelect}
        aria-current={active ? 'true' : undefined}
        title={item.summary ? `${item.title}\n${item.summary}` : item.title}
        {...attributes}
        {...listeners}
      >
        <DirectoryItemContent item={item} />
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}

function DirectoryItemRow({ item, active, onSelect }: { item: ResourceDirectoryItem; active: boolean; onSelect: () => void }) {
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        type="button"
        size={item.summary ? 'lg' : 'default'}
        isActive={active}
        className={cn(item.disabled && 'opacity-50')}
        onClick={onSelect}
        aria-current={active ? 'true' : undefined}
        title={item.summary ? `${item.title}\n${item.summary}` : item.title}
      >
        <DirectoryItemContent item={item} />
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}

function DirectoryItemDragOverlay({ item }: { item: ResourceDirectoryItem }) {
  return (
    <div className="flex w-60 items-center gap-2 rounded-md bg-sidebar p-2 text-sm text-sidebar-foreground shadow-lg ring-1 ring-sidebar-border">
      <DirectoryItemContent item={item} />
    </div>
  )
}

function DirectoryItemContent({ item }: { item: ResourceDirectoryItem }) {
  const ItemIcon = item.icon ?? FileText
  return (
    <>
      {item.thumbnailUrl ? (
        <span className="flex size-5 shrink-0 overflow-hidden rounded-md border border-sidebar-border bg-sidebar">
          <img src={item.thumbnailUrl} alt="" className="size-full object-cover" />
        </span>
      ) : (
        <span className="relative flex size-5 shrink-0 items-center justify-center rounded-md border border-sidebar-border bg-sidebar">
          <ItemIcon className="size-3.5 text-sidebar-foreground/50" />
          {item.status ? <StatusIndicator status={item.status} /> : null}
        </span>
      )}
      <span className="min-w-0 flex-1">
        <span className="block truncate">{item.title}</span>
        {item.summary && <span className="block truncate text-xs text-sidebar-foreground/60">{item.summary}</span>}
      </span>
      {item.badges?.map((badge, index) => <ItemBadge key={`${badge.label}-${index}`} badge={badge} />)}
    </>
  )
}

function StatusIndicator({ status }: { status: NonNullable<ResourceDirectoryItem['status']> }) {
  return (
    <span
      role="img"
      aria-label={status.label}
      className={cn(
        'absolute -right-1 -top-1 size-2 rounded-full ring-2 ring-background',
        status.tone === 'success' && 'bg-[var(--nova-success)]',
        status.tone === 'warning' && 'bg-[var(--nova-warning)]',
        status.tone === 'danger' && 'bg-destructive',
        status.tone === 'muted' && 'bg-muted-foreground',
        (!status.tone || status.tone === 'default') && 'bg-primary',
      )}
    />
  )
}

function ItemBadge({ badge }: { badge: ResourceDirectoryBadge }) {
  if (badge.tone === 'muted') {
    return (
      <span className="shrink-0 text-[10px] text-[var(--nova-text-faint)]">
        {badge.label}
      </span>
    )
  }
  return (
    <Badge
      variant={badge.tone === 'outline' || badge.tone === 'warning' ? 'outline' : 'secondary'}
      className={cn('shrink-0', badge.tone === 'warning' && 'border-transparent bg-[var(--nova-warning-bg)] text-[var(--nova-warning)]')}
      aria-label={badge.title}
    >
      {badge.label}
    </Badge>
  )
}
