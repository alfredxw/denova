import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { FilePreview } from './FilePreview'

describe('FilePreview', () => {
  it('renders workspace images through the asset endpoint and opens a large preview', async () => {
    const user = userEvent.setup()
    render(<FilePreview path="covers/cover.png" content="" />)

    expect(screen.getByText('只读预览')).toBeInTheDocument()
    const image = screen.getByRole('img', { name: 'cover.png 预览' })
    expect(image).toHaveAttribute('src', '/api/workspace/asset?path=covers%2Fcover.png')

    await user.click(screen.getByRole('button', { name: '放大查看图像' }))

    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByRole('img', { name: 'cover.png 预览' })).toHaveAttribute('src', '/api/workspace/asset?path=covers%2Fcover.png')
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

  it('pretty prints JSON files', () => {
    render(<FilePreview path="setting/book.json" content='{"title":"Nova","nested":{"ok":true}}' />)

    expect(screen.getByText('JSON')).toBeInTheDocument()
    const preview = screen.getByText((_, element) => element?.tagName === 'PRE')
    expect(preview).toHaveTextContent('"title": "Nova"')
    expect(preview).toHaveTextContent('"ok": true')
  })

  it('pretty prints valid JSONL rows and keeps invalid rows visible', () => {
    render(<FilePreview path="interactive/story/story-main.jsonl" content={`{"turn":1}\nnot-json\n{"turn":2}`} />)

    expect(screen.getByText('JSONL')).toBeInTheDocument()
    expect(screen.getByText('1 行 JSONL 解析失败，已保留原始行')).toBeInTheDocument()
    const preview = screen.getByText((_, element) => element?.tagName === 'PRE')
    expect(preview).toHaveTextContent('"turn": 1')
    expect(preview).toHaveTextContent('not-json')
    expect(preview).toHaveTextContent('"turn": 2')
  })

  it('cache-busts an image when its watcher revision changes', () => {
    const { rerender } = render(<FilePreview path="covers/cover.png" content="" revision="watch:1" />)
    expect(screen.getByRole('img', { name: 'cover.png 预览' })).toHaveAttribute(
      'src',
      '/api/workspace/asset?path=covers%2Fcover.png&revision=watch%3A1',
    )

    rerender(<FilePreview path="covers/cover.png" content="" revision="watch:2" />)
    expect(screen.getByRole('img', { name: 'cover.png 预览' })).toHaveAttribute(
      'src',
      '/api/workspace/asset?path=covers%2Fcover.png&revision=watch%3A2',
    )
  })

  it('shows the retained snapshot when a previewed file was deleted', () => {
    render(<FilePreview path="setting/book.json" content='{"title":"retained"}' revision="missing" />)

    expect(screen.getByRole('alert')).toHaveTextContent('文件已从磁盘删除')
    expect(screen.getByText((_, element) => element?.tagName === 'PRE')).toHaveTextContent('"title": "retained"')
  })
})
