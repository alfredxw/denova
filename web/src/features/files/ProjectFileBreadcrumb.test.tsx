import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { buildProjectFileTreeFromNodes } from '@/features/project-explorer/model'
import { ProjectFileSnapshotBreadcrumb } from './ProjectFileBreadcrumb'

describe('ProjectFileSnapshotBreadcrumb', () => {
  it('opens any path level and selects a file from that directory', async () => {
    const user = userEvent.setup()
    const onSelectFile = vi.fn()
    const nodes = buildProjectFileTreeFromNodes([{
      name: 'assets',
      type: 'dir',
      children: [{
        name: 'runs',
        type: 'dir',
        children: [
          { name: 'image.png', type: 'file' },
          { name: 'meta.json', type: 'file' },
        ],
      }],
    }])

    render(
      <ProjectFileSnapshotBreadcrumb
        workspace="D:/Books/Background"
        nodes={nodes}
        selectedPath="assets/runs/meta.json"
        onSelectFile={onSelectFile}
      />,
    )

    expect(screen.getByRole('button', { name: '浏览 Background' })).toBeVisible()
    expect(screen.getByRole('button', { name: '浏览 assets' })).toBeVisible()
    await user.click(screen.getByRole('button', { name: '浏览 meta.json' }))
    let imageRow: HTMLElement | null = null
    await waitFor(() => {
      imageRow = document.querySelector<HTMLElement>('.nova-file-tree')
        ?.shadowRoot
        ?.querySelector<HTMLElement>('[data-item-path="assets/runs/image.png"]') ?? null
      expect(imageRow).not.toBeNull()
    })
    await user.click(imageRow!)

    await waitFor(() => expect(onSelectFile).toHaveBeenCalledWith('assets/runs/image.png'))
  })
})
