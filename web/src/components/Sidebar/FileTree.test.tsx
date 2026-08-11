import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { FileTree } from './FileTree'
import type { FileNode } from '@/hooks/useWorkspace'

describe('FileTree', () => {
  it('shows original file names without localized aliases', () => {
    const nodes: FileNode[] = [
      { name: 'ideas.md', type: 'file' },
      { name: 'CREATOR.md', type: 'file' },
    ]

    render(
      <FileTree
        nodes={nodes}
        selectedFile={null}
        onSelectFile={vi.fn()}
      />,
    )

    expect(screen.getByText('ideas.md')).toBeInTheDocument()
    expect(screen.queryByText('灵感')).not.toBeInTheDocument()
    expect(screen.getByText('CREATOR.md')).toBeInTheDocument()
  })

  it('keeps action menu triggers measurable while visually hidden', () => {
    const nodes: FileNode[] = [
      { name: 'interactive-openings.json', type: 'file' },
    ]

    render(
      <FileTree
        nodes={nodes}
        selectedFile={null}
        onSelectFile={vi.fn()}
        onRenameItem={vi.fn()}
      />,
    )

    const trigger = screen.getByLabelText('更多操作')

    expect(trigger).not.toHaveClass('hidden')
    expect(trigger).toHaveClass('opacity-0')
    expect(trigger).toHaveClass('data-[state=open]:opacity-100')
  })

  it('does not render an action trigger when no file action is available', () => {
    render(
      <FileTree
        nodes={[{ name: 'read-only.md', type: 'file' }]}
        selectedFile={null}
        onSelectFile={vi.fn()}
      />,
    )

    expect(screen.queryByLabelText('更多操作')).not.toBeInTheDocument()
  })

  it('sorts Chinese chapter ordinals in reading order', () => {
    const nodes: FileNode[] = [{
      name: 'chapters',
      type: 'dir',
      children: [
        { name: '第十一章-潮声.md', type: 'file' },
        { name: '第一百一十一章-归途.md', type: 'file' },
        { name: '第一章-开局.md', type: 'file' },
        { name: '第一千一百一十一章-终局.md', type: 'file' },
        { name: '序章.md', type: 'file' },
        { name: '第十章-交锋.md', type: 'file' },
      ],
    }]

    render(
      <FileTree
        nodes={nodes}
        selectedFile={null}
        onSelectFile={vi.fn()}
      />,
    )

    const order = [
      '序章.md',
      '第一章-开局.md',
      '第十章-交锋.md',
      '第十一章-潮声.md',
      '第一百一十一章-归途.md',
      '第一千一百一十一章-终局.md',
    ]
    const elements = order.map((name) => screen.getByText(name))
    for (let i = 0; i < elements.length - 1; i += 1) {
      expect(elements[i].compareDocumentPosition(elements[i + 1]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    }
  })

  it('sorts by hidden prefixes while keeping original labels', () => {
    const nodes: FileNode[] = [{
      name: 'chapters',
      type: 'dir',
      children: [
        { name: 'ch00011-第十章-交锋.md', type: 'file' },
        { name: 'ch00002-第一章-开局.md', type: 'file' },
        { name: 'ch00111-第一百一十一章-归途.md', type: 'file' },
        { name: 'ch00001-序章.md', type: 'file' },
      ],
    }, {
      name: 'v00001-第一卷-风起',
      type: 'dir',
    }]

    render(
      <FileTree
        nodes={nodes}
        selectedFile={null}
        onSelectFile={vi.fn()}
      />,
    )

    expect(screen.getByText('v00001-第一卷-风起')).toBeInTheDocument()

    const order = ['ch00001-序章.md', 'ch00002-第一章-开局.md', 'ch00011-第十章-交锋.md', 'ch00111-第一百一十一章-归途.md']
    const elements = order.map((name) => screen.getByText(name))
    for (let i = 0; i < elements.length - 1; i += 1) {
      expect(elements[i].compareDocumentPosition(elements[i + 1]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    }
  })

  it('reports expanded paths when directories are toggled', () => {
    const nodes: FileNode[] = [
      { name: 'manuscripts', type: 'dir', children: [{ name: 'ch01.md', type: 'file' }] },
    ]
    const onExpandedPathsChange = vi.fn()

    render(
      <FileTree
        nodes={nodes}
        selectedFile={null}
        onSelectFile={vi.fn()}
        onExpandedPathsChange={onExpandedPathsChange}
      />,
    )

    expect(screen.queryByText('ch01.md')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'manuscripts' }))
    expect(screen.getByText('ch01.md')).toBeInTheDocument()
    expect(onExpandedPathsChange).toHaveBeenLastCalledWith(expect.arrayContaining(['manuscripts']))

    fireEvent.click(screen.getByRole('button', { name: 'manuscripts' }))
    expect(screen.queryByText('ch01.md')).not.toBeInTheDocument()
    expect(onExpandedPathsChange).toHaveBeenLastCalledWith(expect.not.arrayContaining(['manuscripts']))
  })

  it('starts expanded from defaultExpandedPaths and reports collapse', () => {
    const nodes: FileNode[] = [
      { name: 'manuscripts', type: 'dir', children: [{ name: 'ch01.md', type: 'file' }] },
    ]
    const onExpandedPathsChange = vi.fn()

    render(
      <FileTree
        nodes={nodes}
        selectedFile={null}
        onSelectFile={vi.fn()}
        defaultExpandedPaths={['manuscripts']}
        onExpandedPathsChange={onExpandedPathsChange}
      />,
    )

    expect(screen.getByText('ch01.md')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'manuscripts' }))
    expect(screen.queryByText('ch01.md')).not.toBeInTheDocument()
    expect(onExpandedPathsChange).toHaveBeenLastCalledWith(expect.not.arrayContaining(['manuscripts']))
  })

  it('exposes the full path as a title on truncated directory and file names', () => {
    const nodes: FileNode[] = [
      { name: 'manuscripts', type: 'dir', children: [{ name: 'chapters', type: 'dir', children: [{ name: 'ch01.md', type: 'file' }] }] },
    ]

    render(
      <FileTree
        nodes={nodes}
        selectedFile={null}
        onSelectFile={vi.fn()}
        defaultExpandedPaths={['manuscripts', 'manuscripts/chapters']}
      />,
    )

    expect(screen.getByText('manuscripts')).toHaveAttribute('title', 'manuscripts')
    expect(screen.getByText('chapters')).toHaveAttribute('title', 'manuscripts/chapters')
    expect(screen.getByText('ch01.md')).toHaveAttribute('title', 'manuscripts/chapters/ch01.md')
  })
})
