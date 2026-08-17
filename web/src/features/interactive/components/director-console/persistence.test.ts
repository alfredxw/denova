import { beforeEach, describe, expect, it } from 'vitest'
import { readStoredDirectorConsoleTab, readStoredDirectorRevealed, writeStoredDirectorConsoleTab, writeStoredDirectorRevealed } from './persistence'

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

  it('round-trips every console tab per story and rejects unknown values', () => {
    expect(readStoredDirectorConsoleTab('story-a')).toBeNull()
    writeStoredDirectorConsoleTab('story-a', 'changes')
    writeStoredDirectorConsoleTab('story-b', 'branches')
    expect(readStoredDirectorConsoleTab('story-a')).toBe('changes')
    expect(readStoredDirectorConsoleTab('story-b')).toBe('branches')
    window.localStorage.setItem('nova.directorConsole.tab.story-a', 'bogus')
    expect(readStoredDirectorConsoleTab('story-a')).toBeNull()
  })

  it('reads the former state-tab preference when the new key is absent', () => {
    window.localStorage.setItem('nova.directorConsole.stateTab.story-a', 'world')
    expect(readStoredDirectorConsoleTab('story-a')).toBe('world')
  })
})
