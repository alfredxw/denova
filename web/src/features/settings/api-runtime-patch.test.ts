import { describe, expect, it } from 'vitest'
import { createSettingsMergePatch } from './api'
import type { Settings } from './types'
import type { CLIModelSettings } from '@/features/agent-runtime/types'

describe('runtime settings patches', () => {
  for (const engine of ['codex', 'claude', 'grok', 'antigravity'] as const) {
  for (const role of ['ide', 'general', 'interactive_story'] as const) {
    const baseline: Settings = {
      agent_runtimes: { [role]: { selected: engine, [engine]: { model: 'first', effort: 'high' } } },
    }

    it.each<[string, CLIModelSettings]>([
      ['change effort', { model: 'first', effort: 'low' }],
      ['reset effort', { model: 'first' }],
      ['change model', { model: 'second', effort: 'high' }],
      ['change model and reset effort', { model: 'second' }],
    ])(`sends the complete ${role} ${engine} model selection to %s`, (_name, selection) => {
      const draft: Settings = { agent_runtimes: { [role]: { selected: engine, [engine]: selection } } }
      expect(createSettingsMergePatch(baseline, draft)).toEqual({ agent_runtimes: { [role]: { [engine]: selection } } })
    })

    it(`clears the ${role} ${engine} branch when restoring inheritance`, () => {
      expect(createSettingsMergePatch(baseline, { agent_runtimes: { [role]: { selected: engine } } }))
        .toEqual({ agent_runtimes: { [role]: { [engine]: null } } })
    })

    it(`preserves the dormant ${role} ${engine} branch when switching engines`, () => {
      expect(createSettingsMergePatch(baseline, {
        agent_runtimes: { [role]: { ...baseline.agent_runtimes![role], selected: 'native' } },
      })).toEqual({ agent_runtimes: { [role]: { selected: 'native' } } })
    })

    it(`omits unchanged ${role} ${engine} settings`, () => {
      expect(createSettingsMergePatch(baseline, structuredClone(baseline))).toEqual({})
    })
  }
  }
})
