import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { ChevronDown, ChevronsDownUp, ChevronsUpDown, FileText, Plus, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useControllableState } from '@radix-ui/react-use-controllable-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { ScrollArea } from '@/components/ui/scroll-area'
import { cn } from '@/lib/utils'
import type { ResourceDirectoryBadge, ResourceDirectoryItem, ResourceDirectoryPinnedEntry, ResourceDirectorySection } from './types'

const iconActionClassName = 'nova-nav-item border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'

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
}

/**
 * 统一的资源目录左侧栏：搜索 + 置顶伪条目 + 分组折叠 + 计数 + 组级新建。
 * 资料库 / 方案预设 / Skills 三个页面共用，替代原先三份各自实现。
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
}: ResourceDirectoryProps) {
  const { t } = useTranslation()
  const [query = '', setQuery] = useControllableState({
    prop: queryProp,
    defaultProp: '',
    onChange: onQueryChange,
  })
  const [collapsedSections, setCollapsedSections] = useState<Record<string, boolean>>({})

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
  const toggleSection = (section: ResourceDirectorySection, items: ResourceDirectoryItem[]) => {
    setCollapsedSections((current) => ({
      ...current,
      [section.id]: !isCollapsed(section, items),
    }))
  }

  const allCollapsed = visibleSections.length > 0 && visibleSections.every(({ section, items }) => isCollapsed(section, items))
  const toggleAllSections = () => {
    const next: Record<string, boolean> = {}
    for (const { section } of visibleSections) next[section.id] = !allCollapsed
    setCollapsedSections((current) => ({ ...current, ...next }))
  }

  const totalVisible = visibleSections.reduce((sum, entry) => sum + entry.items.length, 0)

  return (
    <>
      <div className="border-b border-[var(--nova-border)] p-2">
        <div className="flex items-center gap-2">
          <InputGroup className="nova-field min-w-0 flex-1 border-0">
            <InputGroupAddon>
              <Search />
            </InputGroupAddon>
            <InputGroupInput
              className="px-1 text-xs text-[var(--nova-text-muted)] placeholder:text-[var(--nova-text-faint)]"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder={searchPlaceholder ?? t('common.search')}
              aria-label={searchPlaceholder ?? t('common.search')}
            />
            {searchAccessory && (
              <InputGroupAddon align="inline-end" className="pr-1">
                {searchAccessory}
              </InputGroupAddon>
            )}
          </InputGroup>
          {showExpandCollapseAll && (
            <Button
              className={iconActionClassName}
              variant="outline"
              size="icon"
              onClick={toggleAllSections}
              aria-label={allCollapsed ? t('common.expandAll') : t('common.collapseAll')}
              title={allCollapsed ? t('common.expandAll') : t('common.collapseAll')}
            >
              {allCollapsed ? <ChevronsUpDown className="h-3.5 w-3.5" /> : <ChevronsDownUp className="h-3.5 w-3.5" />}
            </Button>
          )}
          {headerActions}
        </div>
        {pinnedEntries && pinnedEntries.length > 0 && (
          <div className="mt-2 space-y-2">
            {pinnedEntries.map((entry) => {
              const PinnedIcon = entry.icon
              return (
                <button
                  key={entry.id}
                  type="button"
                  onClick={() => onSelect(entry.id)}
                  className={cn(
                    'flex h-9 w-full items-center gap-2 rounded-md px-2 text-left text-xs transition',
                    activeId === entry.id
                      ? 'is-active bg-[var(--nova-active)] text-[var(--nova-text)]'
                      : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]',
                  )}
                >
                  <PinnedIcon className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                  <span className="min-w-0 flex-1 truncate">{entry.label}</span>
                </button>
              )
            })}
          </div>
        )}
      </div>
      <ScrollArea className="min-h-0 flex-1">
        <div className="w-0 min-w-full p-2">
          {searching && totalVisible === 0 ? (
            <div className="px-2 py-6 text-center text-xs text-[var(--nova-text-faint)]">{t('common.searchNoResults')}</div>
          ) : (
            visibleSections.map(({ section, items }) => {
              const SectionIcon = section.icon
              const collapsed = isCollapsed(section, items)
              return (
                <section key={section.id} className={items.length ? 'mb-2' : 'mb-1'}>
                  <div className={cn('flex h-8 items-center gap-2 rounded px-2 text-xs', items.length ? 'text-[var(--nova-text-muted)]' : 'text-[var(--nova-text-faint)]')}>
                    <button
                      type="button"
                      className="nova-nav-item rounded p-0.5 text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
                      onClick={() => toggleSection(section, items)}
                      aria-label={`${collapsed ? t('common.expand') : t('common.collapse')}${section.label}`}
                    >
                      <ChevronDown className={cn('h-3.5 w-3.5 transition-transform', collapsed && '-rotate-90')} />
                    </button>
                    {SectionIcon && <SectionIcon className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />}
                    <span className="min-w-0 flex-1 truncate font-medium">{section.label}</span>
                    <span className="text-[11px] text-[var(--nova-text-faint)]">{items.length}</span>
                    {section.headerMeta}
                    {section.onCreate && (
                      <button
                        type="button"
                        className="nova-nav-item rounded p-1 text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
                        disabled={saving}
                        onClick={section.onCreate}
                        aria-label={section.createLabel ?? `${t('common.create')} ${section.label}`}
                      >
                        <Plus className="h-3.5 w-3.5" />
                      </button>
                    )}
                  </div>
                  {!collapsed && items.length > 0 && (
                    <div className="ml-5 space-y-0.5 border-l border-[var(--nova-border)] pl-2">
                      {items.map((item) => (
                        <DirectoryItemRow
                          key={item.id}
                          item={item}
                          active={activeId === item.id}
                          onSelect={() => onSelect(item.id)}
                        />
                      ))}
                    </div>
                  )}
                </section>
              )
            })
          )}
        </div>
      </ScrollArea>
    </>
  )
}

function DirectoryItemRow({ item, active, onSelect }: { item: ResourceDirectoryItem; active: boolean; onSelect: () => void }) {
  const ItemIcon = item.icon ?? FileText
  return (
    <button
      type="button"
      onClick={onSelect}
      className={cn(
        'flex w-full items-center gap-2 rounded-md px-2 text-left text-xs transition',
        item.summary ? 'py-1.5' : 'h-8',
        active
          ? 'is-active bg-[var(--nova-active)] text-[var(--nova-text)]'
          : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]',
        item.disabled && 'opacity-50',
      )}
    >
      {item.thumbnailUrl ? (
        <span className="flex h-5 w-5 shrink-0 overflow-hidden rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface)]">
          <img src={item.thumbnailUrl} alt="" className="h-full w-full object-cover" />
        </span>
      ) : (
        <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface)]">
          <ItemIcon className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
        </span>
      )}
      <span className="min-w-0 flex-1">
        <span className="block truncate">{item.title}</span>
        {item.summary && <span className="block truncate text-[11px] text-[var(--nova-text-faint)]">{item.summary}</span>}
      </span>
      {item.badges?.map((badge, index) => <ItemBadge key={`${badge.label}-${index}`} badge={badge} />)}
    </button>
  )
}

function ItemBadge({ badge }: { badge: ResourceDirectoryBadge }) {
  if (badge.tone === 'muted') {
    return (
      <span className="shrink-0 text-[10px] text-[var(--nova-text-faint)]" title={badge.title}>
        {badge.label}
      </span>
    )
  }
  return (
    <Badge
      variant={badge.tone === 'outline' || badge.tone === 'warning' ? 'outline' : 'secondary'}
      className={cn('shrink-0', badge.tone === 'warning' && 'border-transparent bg-[var(--nova-warning-bg)] text-[var(--nova-warning)]')}
      title={badge.title}
      aria-label={badge.title}
    >
      {badge.label}
    </Badge>
  )
}
