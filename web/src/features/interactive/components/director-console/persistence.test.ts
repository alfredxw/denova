import { beforeEach, describe, expect, it } from 'vitest'
import { readStoredConsoleTab, readStoredDirectorRevealed, writeStoredConsoleTab, writeStoredDirectorRevealed } from './persistence'

describe('director-console persistence', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('round-trips the active tab per story', () => {
    expect(readStoredConsoleTab('story-a')).toBeNull()
    writeStoredConsoleTab('story-a', 'director')
    writeStoredConsoleTab('story-b', 'state')
    expect(readStoredConsoleTab('story-a')).toBe('director')
    expect(readStoredConsoleTab('story-b')).toBe('state')
    expect(readStoredConsoleTab(undefined)).toBeNull()
  })

  it('maps legacy plan/run tab values to the merged director tab', () => {
    window.localStorage.setItem('nova.directorConsole.tab.story-a', 'plan')
    window.localStorage.setItem('nova.directorConsole.tab.story-b', 'run')
    expect(readStoredConsoleTab('story-a')).toBe('director')
    expect(readStoredConsoleTab('story-b')).toBe('director')
  })

  it('rejects unknown tab values', () => {
    window.localStorage.setItem('nova.directorConsole.tab.story-a', 'bogus')
    expect(readStoredConsoleTab('story-a')).toBeNull()
  })

  it('round-trips the director reveal flag per story', () => {
    expect(readStoredDirectorRevealed('story-a')).toBe(false)
    writeStoredDirectorRevealed('story-a', true)
    expect(readStoredDirectorRevealed('story-a')).toBe(true)
    expect(readStoredDirectorRevealed('story-b')).toBe(false)
    writeStoredDirectorRevealed('story-a', false)
    expect(readStoredDirectorRevealed('story-a')).toBe(false)
  })
})
