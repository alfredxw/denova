import { describe, expect, it } from 'vitest'
import { isPreviewableMarkdown, projectFileLanguage } from './file-language'

describe('projectFileLanguage', () => {
  it.each([
    ['src/main.ts', 'typescript'],
    ['src/view.tsx', 'typescript'],
    ['README.md', 'markdown'],
    ['docs/guide.markdown', 'markdown'],
    ['config/settings.yaml', 'yaml'],
    ['Dockerfile', 'dockerfile'],
    ['LICENSE', 'plaintext'],
  ])('maps %s to %s', (path, language) => {
    expect(projectFileLanguage(path)).toBe(language)
  })

  it('previews plain Markdown without executing MDX', () => {
    expect(isPreviewableMarkdown('README.md')).toBe(true)
    expect(isPreviewableMarkdown('docs/guide.markdown')).toBe(true)
    expect(isPreviewableMarkdown('Component.mdx')).toBe(false)
  })
})
