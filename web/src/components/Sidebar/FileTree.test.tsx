import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { FileTree } from './FileTree'
import type { FileNode } from '@/hooks/useWorkspace'

describe('FileTree', () => {
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
      '第一章 开局.md',
      '第十章 交锋.md',
      '第十一章 潮声.md',
      '第一百一十一章 归途.md',
      '第一千一百一十一章 终局.md',
    ]
    const elements = order.map((name) => screen.getByText(name))
    for (let i = 0; i < elements.length - 1; i += 1) {
      expect(elements[i].compareDocumentPosition(elements[i + 1]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    }
  })

  it('sorts by hidden prefixes and hides them in labels', () => {
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

    expect(screen.queryByText('v00001-第一卷-风起')).not.toBeInTheDocument()
    expect(screen.getByText('第一卷 风起')).toBeInTheDocument()

    const order = ['序章.md', '第一章 开局.md', '第十章 交锋.md', '第一百一十一章 归途.md']
    const elements = order.map((name) => screen.getByText(name))
    for (let i = 0; i < elements.length - 1; i += 1) {
      expect(elements[i].compareDocumentPosition(elements[i + 1]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    }
  })
})
