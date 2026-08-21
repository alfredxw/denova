import { describe, expect, it } from 'vitest'
import { selectWorkbenchRoute } from './WorkbenchRouteHost'

describe('selectWorkbenchRoute', () => {
  it('keeps shared routes explicit and independent from content mode', () => {
    expect(selectWorkbenchRoute({ mode: 'agentchat', rightPanel: 'ai', settingsOpen: false })).toBe('agentchat')
    expect(selectWorkbenchRoute({ mode: 'trajectory', rightPanel: 'ai', settingsOpen: false })).toBe('trajectory')
    expect(selectWorkbenchRoute({ mode: 'books', rightPanel: 'ai', settingsOpen: false })).toBe('books')
    expect(selectWorkbenchRoute({ mode: 'interactive', rightPanel: 'ai', settingsOpen: false })).toBe('interactive')
  })

  it('selects Writing, Lore, Presets, and Versions as independent routes', () => {
    expect(selectWorkbenchRoute({ mode: 'ide', rightPanel: 'ai', settingsOpen: false })).toBe('ide-writing')
    expect(selectWorkbenchRoute({ mode: 'lore', rightPanel: 'ai', settingsOpen: false })).toBe('lore')
    expect(selectWorkbenchRoute({ mode: 'presets', rightPanel: 'ai', settingsOpen: false })).toBe('presets')
    expect(selectWorkbenchRoute({ mode: 'versions', rightPanel: 'ai', settingsOpen: false })).toBe('versions')
  })

  it('gives the settings overlay presentation priority', () => {
    expect(selectWorkbenchRoute({ mode: 'interactive', rightPanel: 'ai', settingsOpen: true })).toBe('settings')
  })
})
