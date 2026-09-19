import { describe, expect, it } from 'vitest'
import { createSettingsMergePatch } from './api'
import type { Settings } from './types'
import type { CodexRuntimeSettings } from '@/features/agent-runtime/types'

describe('runtime settings patches', () => {
  for (const role of ['ide', 'general'] as const) {
    const baseline: Settings = {
      agent_runtimes: { [role]: { selected: 'codex', codex: { model: 'first', effort: 'high' } } },
    }

    it.each<[string, CodexRuntimeSettings]>([
      ['change effort', { model: 'first', effort: 'low' }],
      ['reset effort', { model: 'first' }],
      ['change model', { model: 'second', effort: 'high' }],
      ['change model and reset effort', { model: 'second' }],
    ])(`sends the complete ${role} model selection to %s`, (_name, codex) => {
      const draft: Settings = { agent_runtimes: { [role]: { selected: 'codex', codex } } }
      expect(createSettingsMergePatch(baseline, draft)).toEqual({ agent_runtimes: { [role]: { codex } } })
    })

    it(`clears the ${role} branch when restoring inheritance`, () => {
      expect(createSettingsMergePatch(baseline, { agent_runtimes: { [role]: { selected: 'codex' } } }))
        .toEqual({ agent_runtimes: { [role]: { codex: null } } })
    })

    it(`preserves the dormant ${role} branch when switching engines`, () => {
      expect(createSettingsMergePatch(baseline, {
        agent_runtimes: { [role]: { ...baseline.agent_runtimes![role], selected: 'native' } },
      })).toEqual({ agent_runtimes: { [role]: { selected: 'native' } } })
    })

    it(`omits unchanged ${role} settings`, () => {
      expect(createSettingsMergePatch(baseline, structuredClone(baseline))).toEqual({})
    })
  }
})
