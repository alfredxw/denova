import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ImageFilePreview } from './ImageFilePreview'

describe('ImageFilePreview', () => {
  const projectId = 'project-preview'

  it('renders Project images through the asset endpoint and opens a large preview', async () => {
    const user = userEvent.setup()
    render(<ImageFilePreview projectId={projectId} path="covers/cover.png" />)

    expect(screen.getByText('只读预览')).toBeInTheDocument()
    const image = screen.getByRole('img', { name: 'cover.png 预览' })
    expect(image).toHaveAttribute('src', '/api/projects/project-preview/files/asset?path=covers%2Fcover.png')

    await user.click(screen.getByRole('button', { name: '放大查看图像' }))

    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByRole('img', { name: 'cover.png 预览' })).toHaveAttribute('src', '/api/projects/project-preview/files/asset?path=covers%2Fcover.png')
    expect(within(dialog).queryByTitle('covers/cover.png')).not.toBeInTheDocument()
    expect(within(dialog).getByText('100%')).toBeInTheDocument()

    await user.click(within(dialog).getByRole('button', { name: '放大' }))

    expect(within(dialog).getByText('125%')).toBeInTheDocument()

    await user.click(within(dialog).getByRole('button', { name: '缩小' }))
    await user.click(within(dialog).getByRole('button', { name: '缩小' }))
    expect(within(dialog).getByText('80%')).toBeInTheDocument()

    const viewport = within(dialog).getByTestId('image-preview-viewport')
    vi.spyOn(viewport, 'getBoundingClientRect').mockReturnValue({
      bottom: 480,
      height: 480,
      left: 0,
      right: 640,
      top: 0,
      width: 640,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    } as DOMRect)

    fireEvent.wheel(viewport, { deltaY: -120 })
    expect(within(dialog).getByText('80%')).toBeInTheDocument()

    fireEvent.wheel(viewport, { deltaY: -120, ctrlKey: true, clientX: 320, clientY: 240 })
    expect(within(dialog).getByText('129%')).toBeInTheDocument()
  })

  it('cache-busts an image when its watcher revision changes', () => {
    const { rerender } = render(<ImageFilePreview projectId={projectId} path="covers/cover.png" revision="watch:1" />)
    expect(screen.getByRole('img', { name: 'cover.png 预览' })).toHaveAttribute(
      'src',
      '/api/projects/project-preview/files/asset?path=covers%2Fcover.png&revision=watch%3A1',
    )

    rerender(<ImageFilePreview projectId={projectId} path="covers/cover.png" revision="watch:2" />)
    expect(screen.getByRole('img', { name: 'cover.png 预览' })).toHaveAttribute(
      'src',
      '/api/projects/project-preview/files/asset?path=covers%2Fcover.png&revision=watch%3A2',
    )
  })

  it('shows the missing-file warning when a previewed image was deleted', () => {
    render(<ImageFilePreview projectId={projectId} path="covers/cover.png" revision="missing" />)

    expect(screen.getByRole('alert')).toHaveTextContent('文件已从磁盘删除')
  })
})
