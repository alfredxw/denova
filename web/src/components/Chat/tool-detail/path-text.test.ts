import { describe, expect, it } from 'vitest'
import { resolveWorkspaceFilePath } from './path-text'

describe('resolveWorkspaceFilePath', () => {
  const workspace = '/workspace/book'

  it.each([
    ['src/chapter.ts', '', 'src/chapter.ts'],
    ['/workspace/book/src/chapter.ts', '', 'src/chapter.ts'],
    ['src/chapter.ts', 'packages/app', 'packages/app/src/chapter.ts'],
    ['src/chapter.ts', '/workspace/book/packages/app', 'packages/app/src/chapter.ts'],
    ['src/chapter.ts:12:4', '', 'src/chapter.ts'],
    ['"src/chapter name.ts:12"', '', 'src/chapter name.ts'],
    ['.\\src\\chapter.ts', '', 'src/chapter.ts'],
  ])('resolves %s with cwd %s', (candidate, cwd, expected) => {
    expect(resolveWorkspaceFilePath(candidate, workspace, cwd)).toBe(expected)
  })

  it.each([
    'https://example.com/chapter.ts',
    'resource://skills/chapter.ts',
    'src/**/*.ts',
    'src/components',
    '/outside/book/chapter.ts',
  ])('does not link %s', (candidate) => {
    expect(resolveWorkspaceFilePath(candidate, workspace)).toBeNull()
  })
})
