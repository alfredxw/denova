import { describe, expect, it } from 'vitest'
import { resolveWritingSkillSelection, writingSkillOptionsFromSnapshot } from './useWritingSkillOptions'
import type { SkillSummary } from '@/lib/api'

describe('writingSkillOptionsFromSnapshot', () => {
  it('lists only active IDE-compatible Skills that explicitly provide a writing workflow', () => {
    const options = writingSkillOptionsFromSnapshot([
      writingSkill({ name: 'novel-lite', scope: 'builtin', active: true, agent: 'ide' }),
      writingSkill({ name: 'novel-standard', scope: 'builtin', active: true, agent: 'ide' }),
      writingSkill({ name: 'future-preset', scope: 'builtin', active: true, agent: 'ide' }),
      skill({ name: 'humanizer', scope: 'user', active: true, agent: 'ide', category: 'writing' }),
      writingSkill({ name: 'slow-burn', scope: 'user', active: true, agent: 'ide' }),
      writingSkill({ name: 'workspace-room', scope: 'workspace', active: true, agent: 'ide' }),
      writingSkill({ name: 'story-only', scope: 'user', active: true, agent: 'interactive_story' }),
      writingSkill({ name: 'inactive-skill', scope: 'workspace', active: false, agent: 'ide' }),
    ])

    expect(options.map((option) => `${option.scope}:${option.name}`)).toEqual([
      'builtin:novel-lite',
      'builtin:future-preset',
      'builtin:novel-standard',
      'user:slow-burn',
      'workspace:workspace-room',
    ])
  })

  it('allows agent skill overrides to disable a preset', () => {
    const options = writingSkillOptionsFromSnapshot([
      writingSkill({ name: 'novel-standard', scope: 'builtin', active: true, agent: 'ide' }),
      writingSkill({ name: 'slow-burn', scope: 'user', active: true, agent: 'ide' }),
    ], { ide: { 'novel-standard': false } })

    expect(options.map((option) => option.name)).toEqual(['slow-burn'])
  })

  it('falls back from a removed configured preset while preserving available custom Skills', () => {
    const options = writingSkillOptionsFromSnapshot([
      writingSkill({ name: 'novel-lite', scope: 'builtin', active: true, agent: 'ide' }),
      writingSkill({ name: 'slow-burn', scope: 'workspace', active: true, agent: 'ide' }),
    ])

    expect(resolveWritingSkillSelection('novel-heavy', options)).toBe('novel-lite')
    expect(resolveWritingSkillSelection('slow-burn', options)).toBe('slow-burn')
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

function writingSkill(patch: Partial<SkillSummary>): SkillSummary {
  return skill({ capabilities: ['writing-workflow'], ...patch })
}
