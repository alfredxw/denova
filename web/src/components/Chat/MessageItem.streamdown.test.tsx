import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { MessageItem } from './MessageItem'

describe('MessageItem streaming Markdown', () => {
  it('renders animated Markdown, math, CJK emphasis, links, and project images', () => {
    const content = [
      '# 实时标题',
      '',
      '**该强调应正确包含句号。**后文继续。',
      '',
      '$$',
      'x^2 + y^2 = z^2',
      '$$',
    ].join('\n')
    const { container, rerender } = render(
      <>
        <MessageItem key="streaming" message={{ role: 'assistant', content, streaming: true }} />
      </>,
    )

    expect(screen.getByRole('heading', { name: '实时标题' })).toBeInTheDocument()
    expect(container.querySelector('[data-streamdown="strong"]')).toHaveTextContent('该强调应正确包含句号。')
    expect(container.querySelector('.katex-display')).toBeInTheDocument()
    expect(container.querySelector('[data-sd-animate]')).toBeInTheDocument()

    rerender(
      <>
        <MessageItem key="streaming" message={{ role: 'assistant', content, streaming: false }} />
      </>,
    )

    expect(container.querySelector('[data-sd-animate]')).not.toBeInTheDocument()
    expect(container.querySelector('.katex-display')).toBeInTheDocument()
    rerender(
      <>
        <MessageItem key="streaming" message={{ role: 'assistant', content, streaming: false }} />
        <MessageItem
          key="project-image"
          projectId="project-message"
          message={{
            role: 'assistant',
            content: '[参考资料](https://example.com/docs)\n\n[危险链接](javascript:alert(1))\n\n![封面](assets/image/generated/cover.png)',
          }}
        />
      </>,
    )

    expect(container.querySelector('a[href="https://example.com/docs"]')).toHaveAttribute('target', '_blank')
    expect(container.querySelector('a[href^="javascript:"]')).not.toBeInTheDocument()
    expect(container.querySelector('img[alt="封面"]')).toHaveAttribute(
      'src',
      '/api/projects/project-message/files/asset?path=assets%2Fimage%2Fgenerated%2Fcover.png',
    )
  })

  it('uses Streamdown code and table controls without download actions', () => {
    const content = [
      '```ts',
      'const greeting = "hello"',
      '```',
      '',
      '| 项目 | 状态 |',
      '| --- | --- |',
      '| 代码块 | 已验证 |',
    ].join('\n')
    const { container } = render(
      <MessageItem message={{ role: 'assistant', content, streaming: false }} />,
    )

    expect(screen.getByRole('button', { name: '复制代码' })).toBeInTheDocument()
    expect(container.querySelector('[data-streamdown="table"]')).toBeInTheDocument()
    fireEvent.click(screen.getByTitle('复制表格'))
    expect(screen.getByRole('button', { name: 'Markdown' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'CSV' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'TSV' })).toBeInTheDocument()

    fireEvent.click(screen.getByTitle('全屏查看'))
    expect(screen.getByRole('dialog', { name: '全屏查看' })).toBeInTheDocument()
    expect(screen.getByTitle('退出全屏')).toBeInTheDocument()
    expect(screen.queryByTitle(/下载|download/i)).not.toBeInTheDocument()
  })

  it('renders streaming thinking as Markdown', () => {
    const content = [
      '**正在分析。**',
      '',
      '$$',
      'x^2 + y^2 = z^2',
      '$$',
      '',
      '![分析图](assets/image/generated/analysis.png)',
    ].join('\n')
    const { container, rerender } = render(
      <MessageItem projectId="project-thinking" message={{ role: 'thinking', content, streaming: true }} />,
    )

    expect(container.querySelector('[data-streamdown="strong"]')).toHaveTextContent('正在分析。')
    expect(container.querySelector('.katex-display')).toBeInTheDocument()
    expect(container.querySelector('[data-sd-animate]')).toBeInTheDocument()
    expect(container.querySelector('img[alt="分析图"]')).toHaveAttribute(
      'src',
      '/api/projects/project-thinking/files/asset?path=assets%2Fimage%2Fgenerated%2Fanalysis.png',
    )

    rerender(
      <MessageItem projectId="project-thinking" message={{ role: 'thinking', content, streaming: false }} />,
    )
    fireEvent.click(screen.getByRole('button', { name: '展开思考' }))

    expect(container.querySelector('[data-streamdown="strong"]')).toHaveTextContent('正在分析。')
    expect(container.querySelector('[data-sd-animate]')).not.toBeInTheDocument()
  })
})
