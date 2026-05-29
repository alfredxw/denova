import { create } from 'zustand'

export type RightPanel = 'ai' | 'lore' | 'creator' | 'teller' | 'outline' | 'characters' | 'versions' | null
export type BottomPanel = 'versions' | 'problems' | null
export type WorkspaceMode = 'ide' | 'interactive' | 'books'

const MODE_STORAGE_KEY = 'nova:mode'

function readInitialMode(): WorkspaceMode {
  if (typeof window === 'undefined') return 'ide'
  const stored = window.localStorage.getItem(MODE_STORAGE_KEY)
  return stored === 'interactive' || stored === 'books' ? stored : 'ide'
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
  rightPanel: 'ai',
  bottomPanel: null,
  commandOpen: false,
  setMode: (mode) => {
    if (typeof window !== 'undefined') window.localStorage.setItem(MODE_STORAGE_KEY, mode)
    set({ mode })
  },
  setSelectedProjectId: (id) => set({ selectedProjectId: id }),
  setSelectedChapterId: (id) => set({ selectedChapterId: id }),
  setRightPanel: (panel) => set({ rightPanel: panel }),
  setBottomPanel: (panel) => set({ bottomPanel: panel }),
  setCommandOpen: (open) => set({ commandOpen: open }),
}))
