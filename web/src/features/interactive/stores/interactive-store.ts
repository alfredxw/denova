import { create } from 'zustand'
import type { ChatMessage } from '@/lib/api'
import type { BranchSummary, InteractiveSubmode, InteractiveTurnPersistedEvent, Snapshot, StoryDirector, StorySummary, Teller, TurnEvent } from '../types'

const CURRENT_STORY_STORAGE_KEY = 'nova.interactive.current_story.v1'
const CURRENT_BRANCH_STORAGE_KEY = 'nova.interactive.current_branch.v1'
const SUBMODE_STORAGE_KEY = 'nova.interactive.submode.v1'

export interface StoryStageRunState {
  streaming: boolean
  activityContent: string
  liveMessages: ChatMessage[]
  rewindTurnId?: string
}

interface InteractiveStore {
  stories: StorySummary[]
  tellers: Teller[]
  storyDirectors: StoryDirector[]
  branches: BranchSummary[]
  snapshot: Snapshot | null
  storyStageRuns: Record<string, StoryStageRunState>
  currentStoryId: string
  currentBranchId: string
  submode: InteractiveSubmode
  setStories: (stories: StorySummary[], currentStoryId?: string) => void
  setTellers: (tellers: Teller[]) => void
  setStoryDirectors: (directors: StoryDirector[]) => void
  setBranches: (branches: BranchSummary[]) => void
  setSnapshot: (snapshot: Snapshot | null) => void
  applyTurnPersisted: (event: InteractiveTurnPersistedEvent) => Snapshot | null
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

function readRememberedSubmode(): InteractiveSubmode {
  if (typeof window === 'undefined') return 'story'
  const value = window.localStorage.getItem(SUBMODE_STORAGE_KEY)
  return isInteractiveSubmode(value) ? value : 'story'
}

function rememberSubmode(submode: InteractiveSubmode) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(SUBMODE_STORAGE_KEY, submode)
}

function isInteractiveSubmode(value: unknown): value is InteractiveSubmode {
  return value === 'story' || value === 'timeline' || value === 'memory' || value === 'lore' || value === 'creator' || value === 'teller'
}

export const useInteractiveStore = create<InteractiveStore>((set) => ({
  stories: [],
  tellers: [],
  storyDirectors: [],
  branches: [],
  snapshot: null,
  storyStageRuns: {},
  currentStoryId: '',
  currentBranchId: 'main',
  submode: readRememberedSubmode(),
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
  setStoryDirectors: (storyDirectors) => set({ storyDirectors }),
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
  applyTurnPersisted: (event) => {
    let appliedSnapshot: Snapshot | null = null
    set((state) => {
      if (!event?.story_id || !event.branch_id || !event.turn) return state
      if (state.currentStoryId && state.currentStoryId !== event.story_id) return state
      if (state.currentBranchId && state.currentBranchId !== event.branch_id) return state
      const snapshot = mergeInteractiveTurnPersistedSnapshot(state.snapshot, event)
      appliedSnapshot = snapshot
      rememberCurrentStory(event.story_id)
      rememberCurrentBranch(event.story_id, event.branch_id)
      return {
        snapshot,
        branches: event.branches?.length ? event.branches : state.branches,
        currentStoryId: event.story_id,
        currentBranchId: event.branch_id,
      }
    })
    return appliedSnapshot
  },
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
  setSubmode: (submode) => {
    rememberSubmode(submode)
    set({ submode })
  },
  resetWorkspaceState: () => set({
    stories: [],
    tellers: [],
    storyDirectors: [],
    branches: [],
    snapshot: null,
    storyStageRuns: {},
    currentStoryId: '',
    currentBranchId: 'main',
  }),
}))

export function mergeInteractiveTurnPersistedSnapshot(current: Snapshot | null, event: InteractiveTurnPersistedEvent): Snapshot {
  const base: Snapshot = current && current.story_id === event.story_id && current.branch_id === event.branch_id
    ? current
    : { story_id: event.story_id, branch_id: event.branch_id, turns: [], state: {} }
  const turn = event.turn
  const turns = mergePersistedTurn(base.turns || [], turn)
  return {
    ...base,
    story_id: event.story_id,
    branch_id: event.branch_id,
    turns,
    current_turn: turn,
    director_plan: event.director_plan || base.director_plan,
    state: event.state || base.state || {},
    graph: event.graph || base.graph,
    context_compaction: event.context_compaction === undefined ? base.context_compaction : event.context_compaction,
    context_compaction_removal: event.context_compaction_removal === undefined ? base.context_compaction_removal : event.context_compaction_removal,
  }
}

function mergePersistedTurn(currentTurns: TurnEvent[], turn: TurnEvent): TurnEvent[] {
  const existingIndex = currentTurns.findIndex((item) => item.id === turn.id)
  if (existingIndex >= 0) {
    return currentTurns.map((item, index) => (index === existingIndex ? turn : item))
  }
  const parentID = normalizeParentID(turn.parent_id)
  if (parentID) {
    const parentIndex = currentTurns.findIndex((item) => item.id === parentID)
    if (parentIndex >= 0) {
      return [...currentTurns.slice(0, parentIndex + 1), turn]
    }
  } else {
    return [turn]
  }
  return [...currentTurns, turn]
}

function normalizeParentID(parentID: TurnEvent['parent_id']) {
  if (typeof parentID !== 'string') return ''
  return parentID.trim()
}
