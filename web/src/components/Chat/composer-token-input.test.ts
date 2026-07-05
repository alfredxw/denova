import { describe, expect, it } from 'vitest'
import { serializeComposerJSON, textToComposerJSON } from './composer-token-input'

describe('composer token parsing', () => {
  const options = {
    skills: ['novel-lite', 'rewrite'],
    files: ['chapters/ch01.md', 'world/长路径.md'],
    lore: [{ id: 'lore-hero', label: '主角' }],
    styleScenes: ['激烈打斗'],
  }

  it('round trips mixed text and inline tokens', () => {
    const text = 'hello /novel-lite, world @chapters/ch01.md\n@资料:主角 #激烈打斗'
    const doc = textToComposerJSON(text, options)

    expect(serializeComposerJSON(doc)).toBe(text)
    expect(JSON.stringify(doc)).toContain('"kind":"skill"')
    expect(JSON.stringify(doc)).toContain('"kind":"file"')
    expect(JSON.stringify(doc)).toContain('"kind":"lore"')
    expect(JSON.stringify(doc)).toContain('"kind":"style"')
  })

  it('leaves unknown slash and at text editable as normal text', () => {
    const text = 'email a@b.com and /unknown remain plain'
    const doc = textToComposerJSON(text, options)

    expect(serializeComposerJSON(doc)).toBe(text)
    expect(JSON.stringify(doc)).not.toContain('"composerToken"')
  })

  it('handles Chinese file and lore labels without losing the backing id', () => {
    const doc = textToComposerJSON('@world/长路径.md @资料:主角', options)
    const serialized = JSON.stringify(doc)

    expect(serializeComposerJSON(doc)).toBe('@world/长路径.md @资料:主角')
    expect(serialized).toContain('"value":"lore-hero"')
    expect(serialized).toContain('"label":"主角"')
  })
})
