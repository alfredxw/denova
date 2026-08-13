import { describe, expect, it } from 'vitest'
import { selectWorkbenchRoute } from './WorkbenchRouteHost'

describe('selectWorkbenchRoute', () => {
  it('keeps shared routes explicit and independent from content mode', () => {
    expect(selectWorkbenchRoute({ mode: 'agentchat', rightPanel: 'versions', settingsOpen: false })).toBe('agentchat')
    expect(selectWorkbenchRoute({ mode: 'trajectory', rightPanel: 'versions', settingsOpen: false })).toBe('trajectory')
    expect(selectWorkbenchRoute({ mode: 'books', rightPanel: 'ai', settingsOpen: false })).toBe('books')
    expect(selectWorkbenchRoute({ mode: 'interactive', rightPanel: 'versions', settingsOpen: false })).toBe('interactive')
  })

  it('projects writing subroutes without changing mode', () => {
    expect(selectWorkbenchRoute({ mode: 'ide', rightPanel: 'versions', settingsOpen: false })).toBe('versions')
    expect(selectWorkbenchRoute({ mode: 'ide', rightPanel: 'lore', settingsOpen: false })).toBe('ide-lore')
    expect(selectWorkbenchRoute({ mode: 'ide', rightPanel: 'teller', settingsOpen: false })).toBe('ide-teller')
    expect(selectWorkbenchRoute({ mode: 'ide', rightPanel: 'ai', settingsOpen: false })).toBe('ide-writing')
  })

  it('gives the settings overlay presentation priority', () => {
    expect(selectWorkbenchRoute({ mode: 'interactive', rightPanel: 'ai', settingsOpen: true })).toBe('settings')
  })
})
