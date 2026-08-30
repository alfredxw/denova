import type { DirectorConsoleTab } from './types'

// 导演控制台的 UI 偏好按故事持久化：防剧透揭示状态、右栏状态面板的上次激活分区。
const REVEAL_KEY_PREFIX = 'nova.directorConsole.revealed.'
const CONSOLE_TAB_KEY_PREFIX = 'nova.directorConsole.tab.'
const LEGACY_STATE_TAB_KEY_PREFIX = 'nova.directorConsole.stateTab.'

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

export function readStoredDirectorConsoleTab(storyId?: string): DirectorConsoleTab | null {
  const value = read(storageKey(CONSOLE_TAB_KEY_PREFIX, storyId)) || read(storageKey(LEGACY_STATE_TAB_KEY_PREFIX, storyId))
  return migrateDirectorConsoleTab(value)
}

export function writeStoredDirectorConsoleTab(storyId: string | undefined, tab: DirectorConsoleTab) {
  write(storageKey(CONSOLE_TAB_KEY_PREFIX, storyId), tab)
}

function migrateDirectorConsoleTab(value: string | null): DirectorConsoleTab | null {
  if (value === 'overview' || value === 'controls' || value === 'routes') return value
  if (value === 'tuning') return 'controls'
  if (value === 'branches') return 'routes'
  if (value === 'changes' || value === 'actors' || value === 'world' || value === 'plan') return 'overview'
  return null
}
