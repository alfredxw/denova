import { BookMarked, Pin, PinOff, X } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'
import type { WorkspaceSummary } from '@/lib/api'
import { WorkbenchTab, WorkbenchTabStrip } from './WorkbenchTabStrip'

const TABS_STORAGE_PREFIX = 'nova.layout.tabs:'
const ACTIVE_TAB_STORAGE_PREFIX = 'nova.layout.activeTab:'

/** 编辑区 Tab：文件与工作区级工具共享同一套生命周期和持久化规则。 */
export type Tab =
  | { kind: 'file'; path: string; pinned?: boolean }
  | { kind: 'lore'; pinned?: boolean }

/** Tab 唯一标识，用于 React key 与持久化匹配 */
export function tabKey(tab: Tab): string {
  switch (tab.kind) {
    case 'file':
      return `file:${tab.path}`
    case 'lore':
      return 'lore'
  }
}

/** 在 tabs 中挑选最久未激活、且不等于 protectedKey 的 tab key（LRU 淘汰目标）。 */
function pickLRUVictim(tabs: Tab[], protectedKey: string | null, activations: Map<string, number>): string | null {
  let victim: string | null = null
  let lowest = Infinity
  for (const t of tabs) {
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
    tabKey(tab) === key ? { ...tab, pinned: pinned || undefined } : tab
  )))
}

/** 按 max 限制裁剪 tab 列表，循环淘汰最久未激活的 tab；副作用：从 activations 删除被淘汰项。 */
export function enforceTabLimit(tabs: Tab[], protectedKey: string | null, max: number, activations: Map<string, number>): Tab[] {
  const deduped = orderTabs(dedupeTabs(tabs))
  if (max < 1) return deduped
  let current = deduped
  while (current.length > max) {
    const victim = pickLRUVictim(current, protectedKey, activations)
    if (!victim) break
    current = current.filter((t) => tabKey(t) !== victim)
    activations.delete(victim)
  }
  return current
}

/** Tab 显示标题 */
function tabLabel(tab: Tab): string {
  return tab.kind === 'file' ? tab.path.split('/').pop() || tab.path : ''
}

function formatChapterTabLabel(tab: Tab, summary: WorkspaceSummary | null, loreLabel: string): string {
  if (tab.kind === 'lore') return loreLabel
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
  window.localStorage.setItem(TABS_STORAGE_PREFIX + workspace, JSON.stringify(tabs))
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
}

export function TabController({
  tabs,
  activeTabKey,
  summary,
  actions,
  onActivateTab,
  onCloseTab,
  onTogglePin,
}: TabControllerProps) {
  const { t } = useTranslation()
  const activateTabKey = (key: string) => {
    const tab = tabs.find((candidate) => tabKey(candidate) === key)
    if (tab && key !== activeTabKey) onActivateTab(tab)
  }

  return (
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
        return (
          <ContextMenu key={key}>
            <ContextMenuTrigger asChild>
              <div className="group/tab relative h-full min-w-28 max-w-40 flex-[1_1_10rem]">
                <WorkbenchTab
                  value={key}
                  label={label}
                  icon={tab.kind === 'lore' ? <BookMarked className="size-3.5 text-emerald-500" /> : undefined}
                  className="h-full w-full min-w-0 max-w-none flex-none"
                  trailing={tab.pinned ? (
                    <Pin className="size-3 shrink-0 text-[var(--nova-text-faint)]" aria-hidden="true" />
                  ) : (
                    <span className="size-4 shrink-0" aria-hidden="true" />
                  )}
                />
                {!tab.pinned ? (
                  <button
                    type="button"
                    onPointerDown={(event) => event.stopPropagation()}
                    onKeyDown={(event) => event.stopPropagation()}
                    onClick={(event) => { event.stopPropagation(); onCloseTab(tab) }}
                    className="nova-nav-item absolute right-3 top-1/2 z-10 -translate-y-1/2 rounded p-0.5 opacity-0 group-hover/tab:opacity-100 max-md:opacity-100"
                    aria-label={t('tab.close', { label })}
                    title={t('common.close')}
                  >
                    <X className="size-3" />
                  </button>
                ) : null}
              </div>
            </ContextMenuTrigger>
            <ContextMenuContent className="min-w-40">
              <ContextMenuItem onSelect={() => onTogglePin(tab)}>
                {tab.pinned ? <PinOff /> : <Pin />}
                {t(tab.pinned ? 'tab.unpin' : 'tab.pin')}
              </ContextMenuItem>
              <ContextMenuSeparator />
              <ContextMenuItem onSelect={() => onCloseTab(tab)}>
                {t('tab.closeCurrent')}
              </ContextMenuItem>
            </ContextMenuContent>
          </ContextMenu>
        )
      })}
    </WorkbenchTabStrip>
  )
}
