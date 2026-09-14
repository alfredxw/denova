import { createContext, useContext } from 'react'
import type { StoryPickerProps } from '@/features/interactive/components/StoryPicker'
import type { Manifest } from './api'
import type { Setup } from './RuntimeSetup'

export interface GameChoice { id: string; name: string; manifest?: Manifest; releaseId?: string }

/** New-story preferences and navigation only. Saves own their immutable game bindings. */
export interface GameStoryContextValue {
  projectId: string
  creating: boolean
  setCreating: (creating: boolean) => void
  selectedGameId: string
  chooseGame: (id: string) => void
  defaultGameId: string
  setDefault: () => Promise<void>
  choices: GameChoice[]
  picker: StoryPickerProps
  createInstalled: (title: string, setup: Setup, storyId?: string) => Promise<void>
}
export const GameStoryContext = createContext<GameStoryContextValue | null>(null)
export const useGameStories = () => useContext(GameStoryContext)
