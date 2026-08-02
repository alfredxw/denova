import { describe, expect, it } from 'vitest'
import { projectFileLanguage } from './file-language'

describe('projectFileLanguage', () => {
  it.each([
    ['src/main.ts', 'typescript'],
    ['src/view.tsx', 'typescript'],
    ['README.md', 'markdown'],
    ['config/settings.yaml', 'yaml'],
    ['Dockerfile', 'dockerfile'],
    ['LICENSE', 'plaintext'],
  ])('maps %s to %s', (path, language) => {
    expect(projectFileLanguage(path)).toBe(language)
  })
})
