import { create } from 'zustand'
import type { ChatMessage } from '@/lib/api'
import type { BranchSummary, InteractiveSubmode, Snapshot, StorySummary, Teller } from '../types'

export interface StoryStageRunState {
  streaming: boolean
  activityContent: string
  liveMessages: ChatMessage[]
}

interface InteractiveStore {
  stories: StorySummary[]
  tellers: Teller[]
  branches: BranchSummary[]
  snapshot: Snapshot | null
  storyStageRuns: Record<string, StoryStageRunState>
  currentStoryId: string
  currentBranchId: string
  submode: InteractiveSubmode
  setStories: (stories: StorySummary[], currentStoryId?: string) => void
  setTellers: (tellers: Teller[]) => void
  setBranches: (branches: BranchSummary[]) => void
  setSnapshot: (snapshot: Snapshot | null) => void
  setStoryStageRun: (stageKey: string, updater: Partial<StoryStageRunState> | ((current: StoryStageRunState) => StoryStageRunState)) => void
  clearStoryStageRun: (stageKey: string) => void
  setCurrentStoryId: (storyId: string) => void
  setCurrentBranchId: (branchId: string) => void
  setSubmode: (mode: InteractiveSubmode) => void
  resetWorkspaceState: () => void
}

export function emptyStoryStageRun(): StoryStageRunState {
  return { streaming: false, activityContent: '', liveMessages: [] }
}

export const useInteractiveStore = create<InteractiveStore>((set) => ({
  stories: [],
  tellers: [],
  branches: [],
  snapshot: null,
  storyStageRuns: {},
  currentStoryId: '',
  currentBranchId: 'main',
  submode: 'story',
  setStories: (stories, currentStoryId) => set((state) => ({
    stories,
    currentStoryId: currentStoryId || state.currentStoryId || stories[0]?.id || '',
  })),
  setTellers: (tellers) => set({ tellers }),
  setBranches: (branches) => set((state) => ({
    branches,
    currentBranchId: branches.find(branch => branch.current)?.id || state.currentBranchId || 'main',
  })),
  setSnapshot: (snapshot) => set((state) => ({
    snapshot,
    currentBranchId: snapshot?.branch_id || state.currentBranchId,
  })),
  setStoryStageRun: (stageKey, updater) => set((state) => {
    const current = state.storyStageRuns[stageKey] || emptyStoryStageRun()
    const next = typeof updater === 'function' ? updater(current) : { ...current, ...updater }
    return { storyStageRuns: { ...state.storyStageRuns, [stageKey]: next } }
  }),
  clearStoryStageRun: (stageKey) => set((state) => {
    if (!state.storyStageRuns[stageKey]) return state
    const nextRuns = { ...state.storyStageRuns }
    delete nextRuns[stageKey]
    return { storyStageRuns: nextRuns }
  }),
  setCurrentStoryId: (storyId) => set({ currentStoryId: storyId, currentBranchId: 'main', snapshot: null, branches: [] }),
  setCurrentBranchId: (branchId) => set({ currentBranchId: branchId }),
  setSubmode: (submode) => set({ submode }),
  resetWorkspaceState: () => set({
    stories: [],
    tellers: [],
    branches: [],
    snapshot: null,
    storyStageRuns: {},
    currentStoryId: '',
    currentBranchId: 'main',
  }),
}))
