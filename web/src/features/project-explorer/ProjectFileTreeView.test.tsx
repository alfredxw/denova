import type { NodeRendererProps, RowRendererProps, TreeApi } from 'react-arborist'
import { createRef } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ContextMenu, ContextMenuTrigger } from '@/components/ui/context-menu'
import { ProjectFileTreeView } from './ProjectFileTreeView'
import type { ProjectFileExplorerNode } from './model'

describe('ProjectFileTreeView', () => {
  afterEach(() => vi.restoreAllMocks())

  it('fills its host when a context-menu trigger also supplies a ref', async () => {
    const externalRef = createRef<HTMLDivElement>()
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      if (this.getAttribute('data-testid') === 'tree-host') {
        return domRect(420, 640)
      }
      return domRect(0, 0)
    })

    render(
      <ContextMenu>
        <ContextMenuTrigger asChild>
          <ProjectFileTreeView
            ref={externalRef}
            data-testid="tree-host"
            nodes={[]}
            treeRef={createRef<TreeApi<ProjectFileExplorerNode>>()}
            ariaLabel="Project files"
            renderNode={EmptyNode}
            renderRow={PlainRow}
          />
        </ContextMenuTrigger>
      </ContextMenu>,
    )

    expect(externalRef.current).toBe(screen.getByTestId('tree-host'))
    await waitFor(() => {
      expect(screen.getByRole('tree', { name: 'Project files' })).toHaveStyle({
        width: '420px',
        height: '640px',
      })
    })
  })
})

function EmptyNode(_: NodeRendererProps<ProjectFileExplorerNode>) {
  return null
}

function PlainRow({ attrs, innerRef, children }: RowRendererProps<ProjectFileExplorerNode>) {
  return <div {...attrs} ref={innerRef}>{children}</div>
}

function domRect(width: number, height: number): DOMRect {
  return {
    width,
    height,
    top: 0,
    left: 0,
    right: width,
    bottom: height,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  }
}
