import { BookMarked, Bot, Pin, PinOff, X } from 'lucide-react'
import type { ReactNode } from 'react'
import { SortableContext, horizontalListSortingStrategy } from '@dnd-kit/sortable'
import { useTranslation } from 'react-i18next'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'
import type { WorkspaceSummary } from '@/lib/api'
import {
  SortableWorkbenchTabItem,
  WorkbenchTabDragContext,
} from './WorkbenchTabDrag'
import {
  WorkbenchTab,
  WorkbenchTabStrip,
} from './WorkbenchTabStrip'

const TABS_STORAGE_PREFIX = 'nova.layout.tabs:'
const ACTIVE_TAB_STORAGE_PREFIX = 'nova.layout.activeTab:'
export const WRITING_SUBAGENT_TAB_KEY = 'subagent'

/** 编辑区 Tab：文件与工作区级工具共享同一套生命周期和持久化规则。 */
export type Tab =
  | { kind: 'file'; path: string; pinned?: boolean }
  | { kind: 'lore'; pinned?: boolean }
  | {
      kind: 'subagent'
      parentSessionId: string
      sessionKey: string
      title: string
      returnTabKey: string | null
      pinned?: never
    }

/** Tab 唯一标识，用于 React key 与持久化匹配 */
export function tabKey(tab: Tab): string {
  switch (tab.kind) {
    case 'file':
      return `file:${tab.path}`
    case 'lore':
      return 'lore'
    case 'subagent':
      return WRITING_SUBAGENT_TAB_KEY
  }
}

/** 在 tabs 中挑选最久未激活、且不等于 protectedKey 的 tab key（LRU 淘汰目标）。 */
function pickLRUVictim(tabs: Tab[], protectedKey: string | null, activations: Map<string, number>): string | null {
  let victim: string | null = null
  let lowest = Infinity
  for (const t of tabs) {
    if (t.kind === 'subagent') continue
    const k = tabKey(t)
    if (k === protectedKey || t.pinned) continue
    const score = activations.get(k) ?? 0
    if (score < lowest) {
      lowest = score
      victim = k
    }
  }
  return victim
}

/** 按 tabKey 去重，保留首次出现的条目，防止 React 渲染时出现重复 key。 */
export function dedupeTabs(tabs: Tab[]): Tab[] {
  const seen = new Set<string>()
  const result: Tab[] = []
  for (const t of tabs) {
    const k = tabKey(t)
    if (seen.has(k)) continue
    seen.add(k)
    result.push(t)
  }
  return result
}

/** Pinned documents always lead the strip; relative order inside both groups stays stable. */
export function orderTabs(tabs: Tab[]): Tab[] {
  return tabs
    .map((tab, index) => ({ tab, index }))
    .sort((left, right) => Number(Boolean(right.tab.pinned)) - Number(Boolean(left.tab.pinned)) || left.index - right.index)
    .map(({ tab }) => tab)
}

/** Toggle pinning through the one canonical ordering path used by rendering and persistence. */
export function setTabPinned(tabs: Tab[], key: string, pinned: boolean): Tab[] {
  return orderTabs(tabs.map((tab) => (
    tab.kind !== 'subagent' && tabKey(tab) === key ? { ...tab, pinned: pinned || undefined } : tab
  )))
}

/** Move one tab onto another tab's rendered position, preserving the pinned/unpinned boundary. */
export function reorderTabs(tabs: Tab[], sourceKey: string, targetKey: string): Tab[] {
  const sourceIndex = tabs.findIndex((tab) => tabKey(tab) === sourceKey)
  const targetIndex = tabs.findIndex((tab) => tabKey(tab) === targetKey)
  if (sourceIndex < 0 || targetIndex < 0 || sourceIndex === targetIndex) return tabs
  const reordered = [...tabs]
  const [source] = reordered.splice(sourceIndex, 1)
  reordered.splice(targetIndex, 0, source)
  return orderTabs(reordered)
}

/** 按 max 限制裁剪 tab 列表，循环淘汰最久未激活的 tab；副作用：从 activations 删除被淘汰项。 */
export function enforceTabLimit(tabs: Tab[], protectedKey: string | null, max: number, activations: Map<string, number>): Tab[] {
  const deduped = orderTabs(dedupeTabs(tabs))
  if (max < 1) return deduped
  let current = deduped
  while (current.filter((tab) => tab.kind !== 'subagent').length > max) {
    const victim = pickLRUVictim(current, protectedKey, activations)
    if (!victim) break
    current = current.filter((t) => tabKey(t) !== victim)
    activations.delete(victim)
  }
  return current
}

/** Tab 显示标题 */
function tabLabel(tab: Tab): string {
  if (tab.kind === 'file') return tab.path.split('/').pop() || tab.path
  return tab.kind === 'subagent' ? tab.title : ''
}

function formatChapterTabLabel(tab: Tab, summary: WorkspaceSummary | null, loreLabel: string): string {
  if (tab.kind === 'lore') return loreLabel
  if (tab.kind === 'subagent') return tab.title
  return (summary?.chapters || []).find((chapter) => chapter.path === tab.path)?.display_title || tabLabel(tab)
}

/** 按 workspace 分桶读取已打开 tab 列表 */
export function readTabsFor(workspace: string): Tab[] {
  if (typeof window === 'undefined' || !workspace) return []
  try {
    const raw = window.localStorage.getItem(TABS_STORAGE_PREFIX + workspace)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    const tabs = parsed.flatMap((item): Tab[] => {
      if (item && typeof item === 'object') {
        const pinned = item.pinned === true ? true : undefined
        if (item.kind === 'file' && typeof item.path === 'string') return [{ kind: 'file', path: item.path, pinned }]
        if (item.kind === 'lore') return [{ kind: 'lore', pinned }]
      }
      // 兼容旧版本（仅文件路径字符串）
      if (typeof item === 'string') return [{ kind: 'file', path: item }]
      return []
    })
    return orderTabs(dedupeTabs(tabs))
  } catch {
    return []
  }
}

/** 按 workspace 分桶读取激活的 tab key */
export function readActiveTabKeyFor(workspace: string): string | null {
  if (typeof window === 'undefined' || !workspace) return null
  return window.localStorage.getItem(ACTIVE_TAB_STORAGE_PREFIX + workspace)
}

export function persistTabsFor(workspace: string, tabs: Tab[]) {
  if (typeof window === 'undefined' || !workspace) return
  window.localStorage.setItem(TABS_STORAGE_PREFIX + workspace, JSON.stringify(tabs.filter((tab) => tab.kind !== 'subagent')))
}

export function persistActiveTabKeyFor(workspace: string, activeTabKey: string | null) {
  if (typeof window === 'undefined' || !workspace) return
  if (activeTabKey) {
    window.localStorage.setItem(ACTIVE_TAB_STORAGE_PREFIX + workspace, activeTabKey)
  } else {
    window.localStorage.removeItem(ACTIVE_TAB_STORAGE_PREFIX + workspace)
  }
}

interface TabControllerProps {
  tabs: Tab[]
  activeTabKey: string | null
  summary: WorkspaceSummary | null
  actions?: ReactNode
  onActivateTab: (tab: Tab) => void
  onCloseTab: (tab: Tab) => void
  onTogglePin: (tab: Tab) => void
  onMoveTab: (sourceKey: string, targetKey: string) => void
}

export function TabController({
  tabs,
  activeTabKey,
  summary,
  actions,
  onActivateTab,
  onCloseTab,
  onTogglePin,
  onMoveTab,
}: TabControllerProps) {
  const { t } = useTranslation()
  const activateTabKey = (key: string) => {
    const tab = tabs.find((candidate) => tabKey(candidate) === key)
    if (tab && key !== activeTabKey) onActivateTab(tab)
  }

  return (
    <WorkbenchTabDragContext
      onDragEnd={({ active, over }) => {
        if (over && active.id !== over.id) onMoveTab(String(active.id), String(over.id))
      }}
    >
      <SortableContext items={tabs.map(tabKey)} strategy={horizontalListSortingStrategy}>
        <WorkbenchTabStrip
          value={activeTabKey ?? ''}
          onValueChange={activateTabKey}
          endActions={actions}
        >
          {tabs.length === 0 ? (
            <div className="flex h-full items-center px-3 text-[var(--nova-text-faint)]">{t('tab.empty')}</div>
          ) : tabs.map((tab) => {
            const key = tabKey(tab)
            const label = formatChapterTabLabel(tab, summary, t('tab.lore'))
            const icon = tab.kind === 'lore'
              ? <BookMarked className="size-3.5 text-emerald-500" />
              : tab.kind === 'subagent'
                ? <Bot className="size-3.5 text-[var(--nova-text-muted)]" />
                : undefined
            return (
              <SortableWorkbenchTabItem key={key} id={key} label={label} previewIcon={icon}>
                {(dragHandleProps) => (
                  <ContextMenu>
                    <ContextMenuTrigger asChild>
                      <div
                        data-selected={key === activeTabKey ? 'true' : undefined}
                        className="group/tab relative h-full w-full"
                      >
                        <WorkbenchTab
                          {...dragHandleProps}
                          value={key}
                          label={label}
                          icon={icon}
                          className="h-full w-full min-w-0 max-w-none flex-none"
                          trailing={tab.pinned ? (
                            <Pin className="size-3 shrink-0 text-[var(--nova-text-faint)]" aria-hidden="true" />
                          ) : (
                            <span
                              data-slot="workbench-tab-close-space"
                              className="-ml-1.5 h-4 w-0 shrink-0 transition-[width,margin] group-hover/tab:ml-0 group-hover/tab:w-4 group-aria-[selected=true]/tab:ml-0 group-aria-[selected=true]/tab:w-4 max-md:ml-0 max-md:w-4"
                              aria-hidden="true"
                            />
                          )}
                        />
                        {!tab.pinned ? (
                          <button
                            type="button"
                            onPointerDown={(event) => event.stopPropagation()}
                            onKeyDown={(event) => event.stopPropagation()}
                            onClick={(event) => { event.stopPropagation(); onCloseTab(tab) }}
                            className="nova-nav-item pointer-events-none absolute right-2.5 top-1/2 z-10 -translate-y-1/2 rounded p-0.5 opacity-0 group-hover/tab:pointer-events-auto group-hover/tab:opacity-100 group-data-[selected=true]/tab:pointer-events-auto group-data-[selected=true]/tab:opacity-100 max-md:pointer-events-auto max-md:opacity-100"
                            aria-label={t('tab.close', { label })}
                          >
                            <X className="size-3" />
                          </button>
                        ) : null}
                      </div>
                    </ContextMenuTrigger>
                    <ContextMenuContent className="min-w-40">
                      {tab.kind !== 'subagent' ? (
                        <>
                          <ContextMenuItem onSelect={() => onTogglePin(tab)}>
                            {tab.pinned ? <PinOff /> : <Pin />}
                            {t(tab.pinned ? 'tab.unpin' : 'tab.pin')}
                          </ContextMenuItem>
                          <ContextMenuSeparator />
                        </>
                      ) : null}
                      <ContextMenuItem onSelect={() => onCloseTab(tab)}>
                        {t('tab.closeCurrent')}
                      </ContextMenuItem>
                    </ContextMenuContent>
                  </ContextMenu>
                )}
              </SortableWorkbenchTabItem>
            )
          })}
        </WorkbenchTabStrip>
      </SortableContext>
    </WorkbenchTabDragContext>
  )
}
