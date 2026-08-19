import { describe, expect, it } from 'vitest'
import { buildStoryStageCommandMenu } from './story-stage-commands'

const labels = {
  compactDescription: 'Compact context',
  compactHint: 'Reduce context',
  goalDescription: 'Set a durable goal',
  goalHint: 'Continue until complete',
  skillHint: 'Use skill',
}

describe('buildStoryStageCommandMenu', () => {
  it('exposes goal as a built-in command and prevents skill shadowing', () => {
    const menu = buildStoryStageCommandMenu('', [
      { name: 'goal', description: 'shadow' },
      { name: 'scene', description: 'Scene skill' },
    ], labels)

    expect(menu.commands.map((command) => command.name)).toEqual(['goal', 'compact', 'scene'])
    expect(menu.builtInItems.map(({ command }) => command.name)).toEqual(['goal', 'compact'])
  })
})
