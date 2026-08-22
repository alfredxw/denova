import { beforeEach, describe, expect, it } from 'vitest'
import { readStoredHiddenActivityIDs, storeHiddenActivityIDs } from './workbench-activity-order'

describe('workbench activity visibility preferences', () => {
  beforeEach(() => window.localStorage.clear())

  it('keeps only unique known activity IDs from storage', () => {
    window.localStorage.setItem('nova.activity.hidden.workspace.v1', JSON.stringify(['story', 'unknown', 'story', 42, 'skills']))

    expect(readStoredHiddenActivityIDs()).toEqual(['story', 'skills'])
  })

  it('falls back to every menu being visible when storage is malformed', () => {
    window.localStorage.setItem('nova.activity.hidden.workspace.v1', '{invalid')

    expect(readStoredHiddenActivityIDs()).toEqual([])
  })

  it('repairs storage that hides every primary destination', () => {
    window.localStorage.setItem('nova.activity.hidden.workspace.v1', JSON.stringify([
      'writing', 'story', 'agentchat', 'trajectory', 'lore', 'teller', 'versions', 'books', 'skills', 'agents', 'automations',
    ]))

    expect(readStoredHiddenActivityIDs()).not.toContain('writing')
  })

  it('persists hidden activity IDs without changing the existing order preference', () => {
    window.localStorage.setItem('nova.activity.order.workspace.v1', JSON.stringify(['story', 'writing']))

    storeHiddenActivityIDs(['lore', 'versions'])

    expect(JSON.parse(window.localStorage.getItem('nova.activity.hidden.workspace.v1') || '[]')).toEqual(['lore', 'versions'])
    expect(JSON.parse(window.localStorage.getItem('nova.activity.order.workspace.v1') || '[]')).toEqual(['story', 'writing'])
  })
})
