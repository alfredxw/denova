import type { ConsoleTab } from './types'

// 导演控制台的 UI 偏好按故事持久化：切分支/刷新不丢失用户正在看的 tab 与防剧透揭示状态。
const TAB_KEY_PREFIX = 'nova.directorConsole.tab.'
const REVEAL_KEY_PREFIX = 'nova.directorConsole.revealed.'

function storageKey(prefix: string, storyId?: string) {
  return `${prefix}${storyId || 'default'}`
}

function read(key: string) {
  try {
    return window.localStorage.getItem(key)
  } catch {
    return null
  }
}

function write(key: string, value: string) {
  try {
    window.localStorage.setItem(key, value)
  } catch {
    // 隐私模式等场景下 localStorage 不可用，静默降级为会话内状态。
  }
}

export function readStoredConsoleTab(storyId?: string): ConsoleTab | null {
  const value = read(storageKey(TAB_KEY_PREFIX, storyId))
  if (value === 'state' || value === 'director') return value
  // 兼容旧版三 tab 结构：plan/run 已合并为 director。
  if (value === 'plan' || value === 'run') return 'director'
  return null
}

export function writeStoredConsoleTab(storyId: string | undefined, tab: ConsoleTab) {
  write(storageKey(TAB_KEY_PREFIX, storyId), tab)
}

export function readStoredDirectorRevealed(storyId?: string): boolean {
  return read(storageKey(REVEAL_KEY_PREFIX, storyId)) === '1'
}

export function writeStoredDirectorRevealed(storyId: string | undefined, revealed: boolean) {
  write(storageKey(REVEAL_KEY_PREFIX, storyId), revealed ? '1' : '0')
}
