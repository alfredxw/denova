import { X } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { AnimatePresence, LayoutGroup, motion } from 'motion/react'
import type { WorkspaceSummary } from '@/lib/api'
import { novaEase, novaSpring } from '@/features/motion/motion-tokens'

const TABS_STORAGE_PREFIX = 'nova.layout.tabs:'
const ACTIVE_TAB_STORAGE_PREFIX = 'nova.layout.activeTab:'
const tabLayoutTransition = { duration: 0.1, ease: novaEase } as const
const tabPresenceTransition = { duration: 0.06, ease: novaEase } as const
const tabMotionTransition = {
  opacity: tabPresenceTransition,
  scale: tabPresenceTransition,
  layout: tabLayoutTransition,
} as const

/** 编辑区 Tab：承载已打开文件。 */
export type Tab = { kind: 'file'; path: string }

/** Tab 唯一标识，用于 React key 与持久化匹配 */
export function tabKey(tab: Tab): string {
  return `file:${tab.path}`
}

/** 在 tabs 中挑选最久未激活、且不等于 protectedKey 的 tab key（LRU 淘汰目标）。 */
function pickLRUVictim(tabs: Tab[], protectedKey: string | null, activations: Map<string, number>): string | null {
  let victim: string | null = null
  let lowest = Infinity
  for (const t of tabs) {
    const k = tabKey(t)
    if (k === protectedKey) continue
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

/** 按 max 限制裁剪 tab 列表，循环淘汰最久未激活的 tab；副作用：从 activations 删除被淘汰项。 */
export function enforceTabLimit(tabs: Tab[], protectedKey: string | null, max: number, activations: Map<string, number>): Tab[] {
  const deduped = dedupeTabs(tabs)
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
  return tab.path.split('/').pop() || tab.path
}

function formatChapterTabLabel(tab: Tab, summary: WorkspaceSummary | null): string {
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
        if (item.kind === 'file' && typeof item.path === 'string') return [{ kind: 'file', path: item.path }]
      }
      // 兼容旧版本（仅文件路径字符串）
      if (typeof item === 'string') return [{ kind: 'file', path: item }]
      return []
    })
    return dedupeTabs(tabs)
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
}

export function TabController({
  tabs,
  activeTabKey,
  summary,
  actions,
  onActivateTab,
  onCloseTab,
}: TabControllerProps) {
  const { t } = useTranslation()
  return (
    <div className="nova-sidebar flex h-9 shrink-0 items-stretch border-b text-xs">
      <LayoutGroup id="editor-tabs">
      <div className="flex min-w-0 flex-1 items-stretch overflow-x-auto">
        {tabs.length === 0 ? (
          <div className="flex h-full items-center px-3 text-[var(--nova-text-faint)]">{t('tab.empty')}</div>
        ) : (
          <AnimatePresence initial={false} mode="popLayout">
            {tabs.map((tab) => {
              const key = tabKey(tab)
              const isActive = key === activeTabKey
              const label = formatChapterTabLabel(tab, summary)
              return (
                <motion.div
                  key={key}
                  layout
                  initial={{ opacity: 0, scale: 0.98 }}
                  animate={{ opacity: 1, scale: 1 }}
                  exit={{ opacity: 0, scale: 0.99, transition: tabPresenceTransition }}
                  transition={tabMotionTransition}
                  style={{ originX: 0 }}
                  className={`group relative flex h-full shrink-0 items-center gap-2 overflow-hidden border-r border-[var(--nova-border)] px-3 transition-colors ${
                    isActive
                      ? 'text-[var(--nova-text)]'
                      : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)]'
                  }`}
                >
                  {isActive && (
                    <>
                      <motion.span layoutId="editor-tab-active-bg" className="absolute inset-0 bg-[var(--nova-active)]" transition={novaSpring} />
                      <motion.span layoutId="editor-tab-active-line" className="absolute inset-x-0 top-0 h-0.5 bg-[var(--nova-text-faint)]" transition={novaSpring} />
                    </>
                  )}
                  <button
                    type="button"
                    onClick={() => { if (!isActive) onActivateTab(tab) }}
                    className="relative z-10 max-w-[220px] truncate text-left"
                    title={tab.path}
                  >
                    {label}
                  </button>
                  <button
                    type="button"
                    onClick={(event) => { event.stopPropagation(); onCloseTab(tab) }}
                    className="nova-nav-item relative z-10 rounded p-0.5 opacity-0 group-hover:opacity-100"
                    aria-label={t('tab.close', { label })}
                    title={t('common.close')}
                  >
                    <X className="h-3 w-3" />
                  </button>
                </motion.div>
              )
            })}
          </AnimatePresence>
        )}
      </div>
      </LayoutGroup>
      {actions && (
        <div className="flex shrink-0 items-center gap-1 border-l border-[var(--nova-border)] px-2">
          {actions}
        </div>
      )}
    </div>
  )
}
