import { create } from 'zustand'
import type { ChatMessage } from '@/lib/api'
import type { BranchSummary, InteractiveSubmode, Snapshot, StorySummary, Teller } from '../types'

const CURRENT_STORY_STORAGE_KEY = 'nova.interactive.current_story.v1'
const CURRENT_BRANCH_STORAGE_KEY = 'nova.interactive.current_branch.v1'

export interface StoryStageRunState {
  streaming: boolean
  activityContent: string
  liveMessages: ChatMessage[]
  rewindTurnId?: string
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

function readRememberedBranches(): Record<string, string> {
  if (typeof window === 'undefined') return {}
  const raw = window.localStorage.getItem(CURRENT_BRANCH_STORAGE_KEY)
  if (!raw) return {}
  try {
    const parsed = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {}
    const result: Record<string, string> = {}
    for (const [storyId, branchId] of Object.entries(parsed)) {
      if (typeof storyId === 'string' && typeof branchId === 'string' && storyId && branchId) {
        result[storyId] = branchId
      }
    }
    return result
  } catch {
    return {}
  }
}

function rememberCurrentBranch(storyId: string, branchId: string) {
  if (typeof window === 'undefined' || !storyId || !branchId) return
  const remembered = readRememberedBranches()
  remembered[storyId] = branchId
  window.localStorage.setItem(CURRENT_BRANCH_STORAGE_KEY, JSON.stringify(remembered))
}

function rememberedStoryId(stories: StorySummary[]) {
  if (typeof window === 'undefined') return ''
  const storyId = window.localStorage.getItem(CURRENT_STORY_STORAGE_KEY) || ''
  if (!storyId) return ''
  return stories.some((story) => story.id === storyId) ? storyId : ''
}

function rememberCurrentStory(storyId: string) {
  if (typeof window === 'undefined' || !storyId) return
  window.localStorage.setItem(CURRENT_STORY_STORAGE_KEY, storyId)
}

function rememberedBranchFor(storyId: string, branches?: BranchSummary[]) {
  if (!storyId) return ''
  const branchId = readRememberedBranches()[storyId] || ''
  if (!branchId) return ''
  if (branches && branches.length > 0 && !branches.some((branch) => branch.id === branchId)) return ''
  return branchId
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
  setStories: (stories, currentStoryId) => set((state) => {
    const storyId = rememberedStoryId(stories) || currentStoryId || state.currentStoryId || stories[0]?.id || ''
    const branchId = storyId ? rememberedBranchFor(storyId) || (storyId === state.currentStoryId ? state.currentBranchId : 'main') : 'main'
    rememberCurrentStory(storyId)
    return {
      stories,
      currentStoryId: storyId,
      currentBranchId: branchId || 'main',
    }
  }),
  setTellers: (tellers) => set({ tellers }),
  setBranches: (branches) => set((state) => {
    const branchId = rememberedBranchFor(state.currentStoryId, branches) || branches.find(branch => branch.current)?.id || (branches.some(branch => branch.id === state.currentBranchId) ? state.currentBranchId : 'main')
    rememberCurrentBranch(state.currentStoryId, branchId)
    return {
      branches,
      currentBranchId: branchId,
    }
  }),
  setSnapshot: (snapshot) => set((state) => {
    if (snapshot) rememberCurrentBranch(snapshot.story_id, snapshot.branch_id)
    return {
      snapshot,
      currentBranchId: snapshot?.branch_id || state.currentBranchId,
    }
  }),
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
  setCurrentStoryId: (storyId) => set(() => {
    rememberCurrentStory(storyId)
    return { currentStoryId: storyId, currentBranchId: rememberedBranchFor(storyId) || 'main', snapshot: null, branches: [] }
  }),
  setCurrentBranchId: (branchId) => set((state) => {
    rememberCurrentBranch(state.currentStoryId, branchId)
    return { currentBranchId: branchId }
  }),
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
