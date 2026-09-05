import { act, renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { AgentQuickPromptSettings } from '@/features/settings/types'
import { inputCommandValue } from './InputCommandMenu'
import { useInputCommands, type CommandScope } from './use-input-commands'

const quickPrompts: AgentQuickPromptSettings[] = [
  { id: 'first', name: '/clear', prompt: 'Discuss the scene', enabled: true, behavior: 'send' },
  { id: 'second', name: '/clear', prompt: 'Review the scene', enabled: true, behavior: 'fill' },
  { id: 'hidden', name: 'Hidden prompt', prompt: 'Hidden', enabled: false, behavior: 'fill' },
  { id: 'ideas', name: '梳理思路', prompt: 'Explore ideas', enabled: true, behavior: 'fill' },
]

describe('useInputCommands', () => {
  it('keeps duplicate prompt names separate from commands and Skills, preserving prompt identity', () => {
    const { result } = renderHook(() => useInputCommands({
      scope: 'all', skills: [{ name: 'clear', description: 'Reserved' }, { name: 'review', description: 'Review Skill' }],
      quickPrompts, goalEnabled: false,
    }))
    act(() => result.current.setCommandQuery(''))
    const options = result.current.filteredCommands
    expect(new Set(options.map(inputCommandValue)).size).toBe(options.length)
    expect(result.current.filteredQuickPromptCommands.map(({ command }) => command)).toMatchObject([
      { source: 'quick-prompt', id: 'first', prompt: 'Discuss the scene' },
      { source: 'quick-prompt', id: 'second', prompt: 'Review the scene' },
      { source: 'quick-prompt', id: 'ideas', prompt: 'Explore ideas' },
    ])
    expect(result.current.filteredSkillCommands.map(({ command }) => command.cmd)).toEqual(['/review'])
    expect(result.current.filteredBuiltinCommands.some(({ command }) => command.cmd === '/goal')).toBe(false)
    expect(options.some((command) => command.cmd === 'Hidden prompt')).toBe(false)
  })

  it('searches prompt names by substring and reindexes the visible results', () => {
    const { result } = renderHook(() => useInputCommands({
      scope: 'all', skills: [], quickPrompts, goalEnabled: false,
    }))
    act(() => result.current.setCommandQuery('思路'))
    expect(result.current.filteredQuickPromptCommands).toMatchObject([
      { index: 0, command: { id: 'ideas', prompt: 'Explore ideas' } },
    ])
  })

  it.each<CommandScope>(['skills', 'none'])('keeps prompts out of the %s command scope', (scope) => {
    const { result } = renderHook(() => useInputCommands({
      scope, skills: [{ name: 'review', description: 'Review' }], quickPrompts, goalEnabled: false,
    }))
    act(() => result.current.setCommandQuery(''))
    expect(result.current.filteredQuickPromptCommands).toEqual([])
    expect(result.current.filteredCommands.map(({ cmd }) => cmd)).toEqual(scope === 'skills' ? ['/review'] : [])
  })
})
