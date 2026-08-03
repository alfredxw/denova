import { describe, expect, it } from 'vitest'
import type { ProjectFileTreeResolveResult } from '@/lib/api-client/project-files'
import {
  buildProjectFileExplorerNodes,
  mergeProjectDirectories,
  PROJECT_FILE_LOAD_MORE_PREFIX,
} from './model'

describe('project file explorer model', () => {
  it('appends stable cursor pages and removes the load-more row when complete', () => {
    const first = mergeProjectDirectories(new Map(), [result({
      path: '',
      revision: 'r1',
      entries: [
        { name: 'a.ts', path: 'a.ts', type: 'file' },
        { name: 'b.ts', path: 'b.ts', type: 'file' },
      ],
      children_state: 'partial',
      continuation: 'cursor-2',
    })])
    expect(buildProjectFileExplorerNodes('', first, new Set()).map((node) => node.id)).toEqual([
      'a.ts',
      'b.ts',
      PROJECT_FILE_LOAD_MORE_PREFIX,
    ])

    const complete = mergeProjectDirectories(first, [result({
      path: '',
      revision: 'r1',
      entries: [{ name: 'c.ts', path: 'c.ts', type: 'file' }],
      children_state: 'complete',
    })], '')
    expect(buildProjectFileExplorerNodes('', complete, new Set()).map((node) => node.id)).toEqual([
      'a.ts',
      'b.ts',
      'c.ts',
    ])
  })

  it('keeps unresolved directories expandable without traversing symlinks', () => {
    const directories = mergeProjectDirectories(new Map(), [result({
      path: '',
      revision: 'r1',
      entries: [
        { name: 'src', path: 'src', type: 'dir' },
        { name: 'linked', path: 'linked', type: 'dir', symlink: true },
      ],
      children_state: 'complete',
    })])
    const nodes = buildProjectFileExplorerNodes('', directories, new Set())
    expect(nodes[0]).toMatchObject({ id: 'src', loaded: false, children: [] })
    expect(nodes[1]).toMatchObject({ id: 'linked', symlink: true })
    expect(nodes[1].children).toBeUndefined()
  })
})

function result(directory: ProjectFileTreeResolveResult['directories'][number]): ProjectFileTreeResolveResult {
  return { path: directory.path, ok: true, directories: [directory] }
}
