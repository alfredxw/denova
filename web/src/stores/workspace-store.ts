import { create } from 'zustand'

export type RightPanel = 'ai' | null
type BottomPanel = 'versions' | 'problems' | null
export type WorkspaceRoute = 'ide' | 'interactive' | 'lore' | 'presets' | 'versions' | 'books' | 'skills' | 'agents' | 'automations' | 'agentchat' | 'trajectory'
/** Historical internal name retained while the persisted key stays stable. */
export type WorkspaceMode = WorkspaceRoute

/**
 * Shared top-level destinations, alongside Writing and Game. The historical
 * `mode` name remains in persisted state, but navigation no longer has a global mode switch.
 */
const SHARED_WORKSPACE_MODES = ['lore', 'presets', 'versions', 'books', 'skills', 'agents', 'automations', 'agentchat', 'trajectory'] as const

export type SharedWorkspaceMode = (typeof SHARED_WORKSPACE_MODES)[number]

/** Reports whether a mode belongs to the shared top-level menu. */
export function isSharedWorkspaceMode(mode: WorkspaceMode): mode is SharedWorkspaceMode {
  return (SHARED_WORKSPACE_MODES as readonly WorkspaceMode[]).includes(mode)
}

/** Reports whether a destination needs an active Book. */
export function workspaceModeRequiresBook(mode: WorkspaceMode): boolean {
  return mode === 'ide' || mode === 'interactive' || mode === 'lore' || mode === 'presets' || mode === 'versions'
}

const MODE_STORAGE_KEY = 'nova:mode'
const CONTENT_MODE_STORAGE_KEY = 'nova:content-mode'
const RIGHT_PANEL_STORAGE_KEY = 'nova:right-panel'

function readInitialMode(): WorkspaceMode {
  if (typeof window === 'undefined') return 'ide'
  const stored = window.localStorage.getItem(MODE_STORAGE_KEY)
  return isWorkspaceMode(stored) ? stored : 'ide'
}

function readInitialRightPanel(): RightPanel {
  if (typeof window === 'undefined') return 'ai'
  const stored = window.localStorage.getItem(RIGHT_PANEL_STORAGE_KEY)
  if (stored === null) return 'ai'
  return isRightPanel(stored) ? stored : 'ai'
}

function isWorkspaceMode(value: unknown): value is WorkspaceMode {
  return value === 'ide' || value === 'interactive' || value === 'lore' || value === 'presets' || value === 'versions' || value === 'books' || value === 'skills' || value === 'agents' || value === 'automations' || value === 'agentchat' || value === 'trajectory'
}

function isRightPanel(value: unknown): value is RightPanel {
  return value === 'ai' || value === null
}

function persistMode(mode: WorkspaceMode) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(MODE_STORAGE_KEY, mode)
  if (mode === 'ide' || mode === 'interactive') window.localStorage.setItem(CONTENT_MODE_STORAGE_KEY, mode)
}

function persistRightPanel(panel: RightPanel) {
  if (typeof window === 'undefined') return
  if (panel === null) {
    window.localStorage.removeItem(RIGHT_PANEL_STORAGE_KEY)
    return
  }
  window.localStorage.setItem(RIGHT_PANEL_STORAGE_KEY, panel)
}

type WorkspaceStore = {
  mode: WorkspaceMode
  selectedProjectId?: string
  selectedChapterId?: string
  rightPanel: RightPanel
  bottomPanel: BottomPanel
  commandOpen: boolean
  setMode: (mode: WorkspaceMode) => void
  setSelectedProjectId: (id?: string) => void
  setSelectedChapterId: (id?: string) => void
  setRightPanel: (panel: RightPanel) => void
  setBottomPanel: (panel: BottomPanel) => void
  setCommandOpen: (open: boolean) => void
}

/** 工作区 UI 状态 Store，仅保存本地界面状态，不存放服务端数据。 */
export const useWorkspaceStore = create<WorkspaceStore>((set) => ({
  mode: readInitialMode(),
  selectedProjectId: undefined,
  selectedChapterId: undefined,
  rightPanel: readInitialRightPanel(),
  bottomPanel: null,
  commandOpen: false,
  setMode: (mode) => {
    persistMode(mode)
    set({ mode })
  },
  setSelectedProjectId: (id) => set({ selectedProjectId: id }),
  setSelectedChapterId: (id) => set({ selectedChapterId: id }),
  setRightPanel: (panel) => {
    persistRightPanel(panel)
    set({ rightPanel: panel })
  },
  setBottomPanel: (panel) => set({ bottomPanel: panel }),
  setCommandOpen: (open) => set({ commandOpen: open }),
}))
