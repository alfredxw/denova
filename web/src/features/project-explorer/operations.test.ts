import { describe, expect, it } from 'vitest'
import type { ProjectFileExplorerNode } from './model'
import {
  buildProjectFileDuplicatePlan,
  buildProjectFilePastePlan,
} from './operations'

describe('project file tree operations', () => {
  it('uses VS Code-style copy names for same-folder collisions', () => {
    const nodes = [file('notes.md'), file('notes copy.md')]

    expect(buildProjectFilePastePlan(nodes, { mode: 'copy', paths: ['notes.md'] }, '')).toEqual([
      { source: 'notes.md', destination: 'notes copy 2.md' },
    ])
  })

  it('does not move a directory into itself or one of its descendants', () => {
    const nodes = [directory('src', [directory('src/nested')])]

    expect(buildProjectFilePastePlan(nodes, { mode: 'cut', paths: ['src'] }, 'src/nested')).toEqual([])
  })

  it('duplicates each selected item beside its source with collision-free names', () => {
    const nodes = [
      directory('docs', [file('docs/guide.md'), file('docs/guide copy.md')]),
      directory('src'),
    ]

    expect(buildProjectFileDuplicatePlan(nodes, ['docs/guide.md', 'src'])).toEqual([
      { source: 'src', destination: 'src copy' },
      { source: 'docs/guide.md', destination: 'docs/guide copy 2.md' },
    ])
  })
})

function file(path: string): ProjectFileExplorerNode {
  return {
    id: path,
    path,
    name: path.slice(path.lastIndexOf('/') + 1),
    type: 'file',
    ignored: false,
    symlink: false,
    loaded: false,
    loading: false,
  }
}

function directory(path: string, children: ProjectFileExplorerNode[] = []): ProjectFileExplorerNode {
  return {
    id: path,
    path,
    name: path.slice(path.lastIndexOf('/') + 1),
    type: 'dir',
    ignored: false,
    symlink: false,
    loaded: true,
    loading: false,
    children,
  }
}
