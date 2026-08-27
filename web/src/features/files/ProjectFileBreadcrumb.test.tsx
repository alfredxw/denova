import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { buildProjectFileTreeFromNodes } from '@/features/project-explorer/model'
import { fileTreeRow, fileTreeShadow } from '@/test/file-tree'
import { ProjectFileSnapshotBreadcrumb } from './ProjectFileBreadcrumb'

describe('ProjectFileSnapshotBreadcrumb', () => {
  it('expands only the selected directory and navigates to a sibling file', async () => {
    const user = userEvent.setup()
    const onSelectFile = vi.fn()
    const nodes = buildProjectFileTreeFromNodes([{
      name: 'assets',
      type: 'dir',
      children: [
        {
          name: 'archive',
          type: 'dir',
          children: [{ name: 'old.json', type: 'file' }],
        },
        {
          name: 'runs',
          type: 'dir',
          children: [
            { name: 'image.png', type: 'file' },
            { name: 'meta.json', type: 'file' },
          ],
        },
      ],
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
    await user.click(screen.getByRole('button', { name: '浏览 runs' }))
    await waitFor(() => expect(fileTreeRow('assets/runs/')).toHaveAttribute('aria-expanded', 'true'))
    expect(fileTreeRow('assets/archive/')).toHaveAttribute('aria-expanded', 'false')
    expect(fileTreeShadow().querySelector('[data-item-path="assets/archive/old.json"]')).toBeNull()
    expect(fileTreeRow('assets/runs/image.png')).toBeVisible()

    await user.click(screen.getByRole('button', { name: '浏览 runs' }))
    await user.click(screen.getByRole('button', { name: '浏览 meta.json' }))
    await user.click(fileTreeRow('assets/runs/image.png'))

    await waitFor(() => expect(onSelectFile).toHaveBeenCalledWith('assets/runs/image.png'))
  })
})
