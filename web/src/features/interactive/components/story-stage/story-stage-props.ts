import type { BookOpeningPreset, StoryCreateInput } from '../../opening'
import type { ImagePreset, InteractiveTurnPersistedEvent, Snapshot, StoryDirector, StoryImageSettings, StorySummary, Teller } from '../../types'
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
  bookOpeningPresets?: BookOpeningPreset[]
  directorPanelVisible?: boolean
  stateDisplayPreference?: StoryStateDisplayPreference
  onStorySelect?: (storyId: string) => void
  onStoryCreate?: (input: StoryCreateInput) => void | Promise<void>
  onStorySetupUpdate?: (input: StoryCreateInput) => void | Promise<void>
  onNarrativeStyleChange?: (id: string) => void | Promise<unknown>
  onStoryDelete?: (storyIds: string[]) => void | Promise<void>
  onDirectorChange?: (directorId: string) => void
  onReplyTargetCharsChange?: (replyTargetChars: number) => void | Promise<void>
  onImageSettingsChange?: (settings: StoryImageSettings) => void | Promise<void>
  onRequestLoreInit?: () => void
  onOpenDirectorConfig?: () => void
  onToggleDirectorPanel?: () => void
  onOpenDirectorState?: () => void
  onRequestCreateBranch?: (source: BranchCreationSource) => void
  onStateDisplayPreferenceChange?: (value: StoryStateDisplayPreference) => void
  onTurnPersisted?: (event: InteractiveTurnPersistedEvent) => Snapshot | void
  onDone: (options?: { silent?: boolean }) => void | Promise<Snapshot | void>
}
