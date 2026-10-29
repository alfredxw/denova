import { describe, expect, it } from 'vitest'
import { directNarrativeSlots, orderedImportPrompts, parseNarrativePreset } from './narrative-preset-import'

describe('narrative preset import', () => {
  it('preserves different rules sharing an identifier and follows the first definition for activation', () => {
    const preset = parseNarrativePreset(JSON.stringify({
      prompts: [
        { identifier: 'shared', name: 'Dialogue', content: 'Keep dialogue concise.' },
        { identifier: 'pacing', name: 'Pacing', content: 'Advance one scene at a time.' },
        { identifier: 'shared', name: 'Humor', content: 'Use dry humor.' },
      ],
      prompt_order: [{ order: [
        { identifier: 'pacing', enabled: false },
        { identifier: 'shared', enabled: true },
      ] }],
    }))
    const prompts = orderedImportPrompts(preset, 0)

    expect(prompts.map(({ name, content, enabled }) => ({ name, content, enabled }))).toEqual([
      { name: 'Pacing', content: 'Advance one scene at a time.', enabled: false },
      { name: 'Dialogue', content: 'Keep dialogue concise.', enabled: true },
      { name: 'Humor', content: 'Use dry humor.', enabled: false },
    ])
    expect(new Set(prompts.map(prompt => prompt.identifier)).size).toBe(3)
    expect(directNarrativeSlots(prompts).map(({ name, content, enabled }) => ({ name, content, enabled })))
      .toEqual(prompts.map(({ name, content, enabled }) => ({ name, content, enabled })))
    const selected = new Set([prompts[2].identifier])
    expect(prompts.filter(prompt => selected.has(prompt.identifier)).map(prompt => prompt.name)).toEqual(['Humor'])
  })

  it('keeps every duplicate separately without colliding with existing source identifiers', () => {
    const contents = ['First', 'Second', 'Existing suffix', 'Third']
    const preset = parseNarrativePreset(JSON.stringify({
      prompts: ['shared', 'shared', 'shared-2', 'shared'].map((identifier, index) => ({ identifier, content: contents[index] })),
      prompt_order: [{ order: [
        { identifier: 'shared', enabled: true },
        { identifier: 'shared-2', enabled: true },
      ] }],
    }))
    const prompts = orderedImportPrompts(preset, 0)

    expect(new Set(prompts.map(prompt => prompt.identifier)).size).toBe(4)
    expect(prompts.map(({ content, enabled }) => ({ content, enabled }))).toEqual([
      { content: 'First', enabled: true },
      { content: 'Existing suffix', enabled: true },
      { content: 'Second', enabled: false },
      { content: 'Third', enabled: false },
    ])
  })

  it('retains each configuration’s activation state while leaving extra definitions disabled', () => {
    const preset = parseNarrativePreset(JSON.stringify({
      prompts: [{ identifier: 'shared', content: 'First' }, { identifier: 'shared', content: 'Second' }],
      prompt_order: [false, true].map(enabled => ({ order: [{ identifier: 'shared', enabled }] })),
    }))

    expect(orderedImportPrompts(preset, 0).map(({ content, enabled }) => ({ content, enabled }))).toEqual([
      { content: 'First', enabled: false }, { content: 'Second', enabled: false },
    ])
    expect(orderedImportPrompts(preset, 1).map(({ content, enabled }) => ({ content, enabled }))).toEqual([
      { content: 'First', enabled: true }, { content: 'Second', enabled: false },
    ])
  })

  it.each([
    [{ identifier: 'shared', enabled: true }, { identifier: 'shared', enabled: false }],
    [{ identifier: 'shared-2', enabled: true }],
  ])('still rejects ambiguous or missing order references: %j', (...order) => {
    expect(() => parseNarrativePreset(JSON.stringify({
      prompts: [{ identifier: 'shared', content: 'First' }, { identifier: 'shared', content: 'Second' }],
      prompt_order: [{ order }],
    }))).toThrow('Narrative preset import failed: invalidOrder')
  })
})
