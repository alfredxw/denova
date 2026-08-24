import { render, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { fileTreeRow, fileTreeShadow } from '@/test/file-tree'
import { NovaFileTree } from './NovaFileTree'

describe('NovaFileTree', () => {
  it('clears a display name when a virtualized row is reused for another path', async () => {
    const volumePath = 'chapters/v00001-volume/'
    const chapterPath = `${volumePath}ch00001-first.md`
    const ordinaryPath = `${volumePath}notes.md`
    const paths = (filePath: string) => ['chapters/', volumePath, filePath]
    const getDisplayName = (path: string) => {
      if (path === volumePath) return 'volume'
      if (path === chapterPath) return 'first'
      return null
    }
    const { rerender } = render(
      <NovaFileTree
        paths={paths(chapterPath)}
        ariaLabel="Project files"
        searchLabel="Filter by name"
        initialExpansion="open"
        getDisplayName={getDisplayName}
      />,
    )

    await waitFor(() => expect(fileTreeRow(chapterPath)).toHaveAttribute('aria-label', 'first'))

    rerender(
      <NovaFileTree
        paths={paths(ordinaryPath)}
        ariaLabel="Project files"
        searchLabel="Filter by name"
        initialExpansion="open"
        getDisplayName={getDisplayName}
      />,
    )

    await waitFor(() => expect(fileTreeRow(ordinaryPath)).toHaveAttribute('aria-label', 'notes.md'))
    expect(fileTreeRow(ordinaryPath).querySelector('[data-nova-file-tree-display-name-value]')).not.toBeInTheDocument()
  })

  it('only reserves Git and action lanes when they are needed', async () => {
    render(
      <NovaFileTree
        paths={['changed.ts', 'clean.ts']}
        ariaLabel="Project files"
        searchLabel="Filter by name"
        gitStatus={[{ path: 'changed.ts', status: 'modified' }]}
        renderContextMenu={() => null}
      />,
    )

    await waitFor(() => expect(fileTreeRow('changed.ts')).toHaveAttribute('data-item-git-status', 'modified'))
    expect(fileTreeRow('clean.ts').querySelector('[data-item-section="git"]')).toBeInTheDocument()
    expect(fileTreeRow('clean.ts').querySelector('[data-item-section="action"]')).toBeInTheDocument()

    const unsafeCSS = fileTreeShadow().querySelector('[data-file-tree-unsafe-css]')
    expect(unsafeCSS).toHaveTextContent(
      '[data-type="item"]:not([data-item-git-status]):not([data-item-contains-git-change="true"]) > [data-item-section="git"]',
    )
    expect(unsafeCSS).toHaveTextContent(
      '[data-type="item"][data-item-context-menu-button-visibility="when-needed"]:not(:hover):not(:focus-visible):not([data-item-context-hover="true"]) > [data-item-section="action"]',
    )
  })
})
