import type { BookOpeningPreset, StoryCreateInput } from '../../opening'
import type { ImagePreset, InteractiveTurnPersistedEvent, Snapshot, StoryDirector, StorySummary, Teller } from '../../types'
import type { LoreItem } from '@/lib/api'
import type { BranchCreationSource } from '../branching/model'
import type { StoryStateDisplayPreference } from '../story-state/display-preference'

/** Stable integration surface between the interactive workspace and the story stage. */
export interface StoryStageProps {
  projectId: string
  workspace?: string
  styleSceneSuggestions?: string[]
  stories?: StorySummary[]
  story?: StorySummary
  tellers?: Teller[]
  storyDirectors?: StoryDirector[]
  imagePresets?: ImagePreset[]
  recentNarrativeStyleID?: string
  narrativeStyleLoading?: boolean
  storyId: string
  branchId: string
  snapshot: Snapshot | null
  snapshotLoading?: boolean
  loreEmpty?: boolean
  loreItems?: LoreItem[]
  bookOpeningPresets?: BookOpeningPreset[]
  directorPanelVisible?: boolean
  stateDisplayPreference?: StoryStateDisplayPreference
  onStorySelect?: (storyId: string) => void
  onStoryCreate?: (input: StoryCreateInput) => StorySummary | void | Promise<StorySummary | void>
  onStorySetupUpdate?: (input: StoryCreateInput) => void | Promise<void>
  onNarrativeStyleChange?: (id: string) => void | Promise<unknown>
  onStoryDelete?: (storyIds: string[]) => void | Promise<void>
  onStoryRename?: (storyId: string, title: string) => void | Promise<void>
  onRequestLoreInit?: () => void
  onOpenDirectorConfig?: () => void
  onToggleDirectorPanel?: () => void
  onOpenDirectorState?: () => void
  onRequestCreateBranch?: (source: BranchCreationSource) => void
  onStateDisplayPreferenceChange?: (value: StoryStateDisplayPreference) => void
  onTurnPersisted?: (event: InteractiveTurnPersistedEvent) => Snapshot | void
  onDone: (options?: { silent?: boolean }) => void | Promise<Snapshot | void>
}
