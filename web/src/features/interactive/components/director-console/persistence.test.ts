import { beforeEach, describe, expect, it } from 'vitest'
import { readStoredDirectorRevealed, readStoredStatePanelTab, writeStoredDirectorRevealed, writeStoredStatePanelTab } from './persistence'

describe('director-console persistence', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('round-trips the director reveal flag per story', () => {
    expect(readStoredDirectorRevealed('story-a')).toBe(false)
    writeStoredDirectorRevealed('story-a', true)
    expect(readStoredDirectorRevealed('story-a')).toBe(true)
    expect(readStoredDirectorRevealed('story-b')).toBe(false)
    writeStoredDirectorRevealed('story-a', false)
    expect(readStoredDirectorRevealed('story-a')).toBe(false)
  })

  it('round-trips the state panel tab per story and rejects unknown values', () => {
    expect(readStoredStatePanelTab('story-a')).toBeNull()
    writeStoredStatePanelTab('story-a', 'changes')
    writeStoredStatePanelTab('story-b', 'world')
    expect(readStoredStatePanelTab('story-a')).toBe('changes')
    expect(readStoredStatePanelTab('story-b')).toBe('world')
    window.localStorage.setItem('nova.directorConsole.stateTab.story-a', 'bogus')
    expect(readStoredStatePanelTab('story-a')).toBeNull()
  })
})
