import type { StatePanelTab } from './types'

// 导演控制台的 UI 偏好按故事持久化：防剧透揭示状态、右栏状态面板的上次激活分区。
const REVEAL_KEY_PREFIX = 'nova.directorConsole.revealed.'
const STATE_TAB_KEY_PREFIX = 'nova.directorConsole.stateTab.'

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

export function readStoredDirectorRevealed(storyId?: string): boolean {
  return read(storageKey(REVEAL_KEY_PREFIX, storyId)) === '1'
}

export function writeStoredDirectorRevealed(storyId: string | undefined, revealed: boolean) {
  write(storageKey(REVEAL_KEY_PREFIX, storyId), revealed ? '1' : '0')
}

export function readStoredStatePanelTab(storyId?: string): StatePanelTab | null {
  const value = read(storageKey(STATE_TAB_KEY_PREFIX, storyId))
  return value === 'changes' || value === 'actors' || value === 'world' ? value : null
}

export function writeStoredStatePanelTab(storyId: string | undefined, tab: StatePanelTab) {
  write(storageKey(STATE_TAB_KEY_PREFIX, storyId), tab)
}
