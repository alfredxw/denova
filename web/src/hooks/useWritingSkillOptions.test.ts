import { describe, expect, it } from 'vitest'
import { writingSkillOptionsFromSnapshot } from './useWritingSkillOptions'
import type { SkillSummary } from '@/lib/api'

describe('writingSkillOptionsFromSnapshot', () => {
  it('lists built-in writing presets and active IDE-compatible user/workspace Skills', () => {
    const options = writingSkillOptionsFromSnapshot([
      skill({ name: 'novel-lite', scope: 'builtin', active: true, agent: 'ide' }),
      skill({ name: 'novel-standard', scope: 'builtin', active: true, agent: 'ide' }),
      skill({ name: 'novel-heavy', scope: 'builtin', active: true, agent: 'ide' }),
      skill({ name: 'outline', scope: 'builtin', active: true, agent: 'ide' }),
      skill({ name: 'slow-burn', scope: 'user', active: true, agent: 'ide' }),
      skill({ name: 'workspace-room', scope: 'workspace', active: true, agent: 'ide' }),
      skill({ name: 'story-only', scope: 'user', active: true, agent: 'interactive_story' }),
      skill({ name: 'inactive-skill', scope: 'workspace', active: false, agent: 'ide' }),
    ])

    expect(options.map((option) => `${option.scope}:${option.name}`)).toEqual([
      'builtin:novel-lite',
      'builtin:novel-standard',
      'builtin:novel-heavy',
      'user:slow-burn',
      'workspace:workspace-room',
    ])
  })

  it('allows agent skill overrides to disable a preset', () => {
    const options = writingSkillOptionsFromSnapshot([
      skill({ name: 'novel-standard', scope: 'builtin', active: true, agent: 'ide' }),
      skill({ name: 'slow-burn', scope: 'user', active: true, agent: 'ide' }),
    ], { ide: { 'novel-standard': false } })

    expect(options.map((option) => option.name)).toEqual(['slow-burn'])
  })
})

function skill(patch: Partial<SkillSummary>): SkillSummary {
  return {
    name: 'novel-standard',
    description: '',
    scope: 'builtin',
    path: '',
    editable: false,
    active: true,
    agent: 'ide',
    ...patch,
  }
}
