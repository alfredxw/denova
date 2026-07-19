import { describe, expect, it } from 'vitest'
import {
  rebaseJSONValue,
  rebaseJSONValueWithConflicts,
  rebaseText,
  rebaseTextWithConflicts,
} from './three-way-rebase'

describe('three-way rebase', () => {
  it('preserves non-overlapping local and external text edits', () => {
    const baseline = '# Skill\n\ndescription: old\n\nBody old.\n'
    const local = '# Skill\n\ndescription: local\n\nBody old.\n'
    const external = '# Skill\n\ndescription: old\n\nBody updated externally.\n'

    expect(rebaseText(baseline, local, external)).toBe(
      '# Skill\n\ndescription: local\n\nBody updated externally.\n',
    )
  })

  it('keeps the active local text and reports both sides of an overlapping edit', () => {
    const merged = rebaseTextWithConflicts(
      '# Draft\n\nShared sentence.\n',
      '# Draft\n\nLocal sentence.\n',
      '# Draft\n\nAgent sentence.\n',
    )

    expect(merged.value).toBe('# Draft\n\nLocal sentence.\n')
    expect(merged.conflicts).toHaveLength(1)
    expect(merged.conflicts[0]).toEqual({
      path: [],
      baseline: 'Shared sentence.\n',
      local: 'Local sentence.\n',
      external: 'Agent sentence.\n',
    })
  })

  it('replays only locally changed JSON fields over the external snapshot', () => {
    const baseline = {
      theme: 'dark',
      agents: { ide: { profile: 'old' }, image: { profile: 'image-old' } },
    }
    const local = {
      theme: 'dark',
      agents: { ide: { profile: 'local' }, image: { profile: 'image-old' } },
    }
    const external = {
      theme: 'light',
      agents: { ide: { profile: 'old' }, image: { profile: 'image-external' } },
    }

    expect(rebaseJSONValue(baseline, local, external)).toEqual({
      theme: 'light',
      agents: { ide: { profile: 'local' }, image: { profile: 'image-external' } },
    })
  })

  it('reports overlapping JSON fields while retaining the external value in the recovery record', () => {
    const merged = rebaseJSONValueWithConflicts(
      { agents: { ide: { profile: 'baseline' } } },
      { agents: { ide: { profile: 'local' } } },
      { agents: { ide: { profile: 'agent' } } },
    )

    expect(merged.value).toEqual({ agents: { ide: { profile: 'local' } } })
    expect(merged.conflicts).toEqual([{
      path: ['agents', 'ide', 'profile'],
      baseline: 'baseline',
      local: 'local',
      external: 'agent',
    }])
  })

  it('reports a conflict when the local draft deletes a field changed externally', () => {
    const merged = rebaseJSONValueWithConflicts(
      { model: 'baseline', untouched: true },
      { untouched: true },
      { model: 'external', untouched: true },
    )

    expect(merged.value).toEqual({ untouched: true })
    expect(merged.conflicts).toEqual([{
      path: ['model'],
      baseline: 'baseline',
      local: undefined,
      external: 'external',
    }])
  })
})
