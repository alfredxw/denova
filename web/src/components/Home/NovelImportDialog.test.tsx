import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { NovelImportDialog } from './NovelImportDialog'

describe('NovelImportDialog', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('previews with sample settings and imports with confirmed split regex', async () => {
    const user = userEvent.setup()
    const requests: Array<{ path: string; body: FormData }> = []
    globalThis.fetch = vi.fn(async (input, init) => {
      const path = typeof input === 'string' ? input : input.url
      requests.push({ path, body: init?.body as FormData })
      if (path.endsWith('/preview/stream')) {
        const preview = {
          title: '长夜',
          split_strategy: requests.length === 1 ? 'tool_agent_regex' : 'custom_regex',
          split_regex: requests.length === 1 ? '^==\\s*(.+?)\\s*==$' : '^@@\\s*(.+)$',
          sample_chars: 20000,
          chapter_count: 2,
          total_chars: 120,
          chapters: [
            { index: 1, title: '开端', chars: 60 },
            { index: 2, title: '转折', chars: 60 },
          ],
          warnings: [],
        }
        return new Response(sse([
          ['progress', { step: 'uploaded' }],
          ['progress', { step: 'agent_start' }],
          ['preview', preview],
          ['done', { status: 'ok' }],
        ]), { status: 200, headers: { 'Content-Type': 'text/event-stream' } })
      }
      return new Response(JSON.stringify({
        workspace: '/nova/长夜',
        title: '长夜',
        chapter_count: 2,
        total_chars: 120,
        chapter_paths: ['chapters/ch0001-开端.md', 'chapters/ch0002-转折.md'],
        message: 'ok',
      }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    }) as typeof fetch

    const onImported = vi.fn()
    const { container } = render(
      <NovelImportDialog
        open
        novaDir="/nova"
        onOpenChange={vi.fn()}
        onImported={onImported}
      />,
    )

    const input = container.querySelector('input[type="file"]') as HTMLInputElement
    fireEvent.change(input, { target: { files: [new File(['== 开端 ==\n内容\n== 转折 ==\n内容'], '长夜.txt', { type: 'text/plain' })] } })

    await screen.findByText('工具 Agent 识别')
    expect(requests[0].path).toBe('/api/books/import-novel/preview/stream')
    expect(requests[0].body.get('sample_chars')).toBe('20000')
    expect(requests[0].body.get('split_regex')).toBeNull()

    await user.clear(screen.getByLabelText('章节/分卷标题正则（Go regexp）'))
    await user.type(screen.getByLabelText('章节/分卷标题正则（Go regexp）'), '^@@\\s*(.+)$')
    expect(screen.getByText('分割参数已变更，请重新预览后再导入。')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '导入' })).toBeDisabled()

    await user.click(screen.getByRole('button', { name: '重新预览' }))
    await screen.findByText('自定义正则')
    expect(requests[1].body.get('split_regex')).toBe('^@@\\s*(.+)$')

    await user.click(screen.getByRole('button', { name: '导入' }))
    await waitFor(() => expect(onImported).toHaveBeenCalled())
    const importBody = requests[2].body
    expect(importBody.get('split_regex')).toBe('^@@\\s*(.+)$')
    expect(importBody.get('sample_chars')).toBe('20000')
    expect(importBody.get('split_strategy')).toBe('custom_regex')
  })

  it('can force tool agent detection without sending the current regex', async () => {
    const user = userEvent.setup()
    const requests: Array<{ path: string; body: FormData }> = []
    globalThis.fetch = vi.fn(async (input, init) => {
      const path = typeof input === 'string' ? input : input.url
      const body = init?.body as FormData
      requests.push({ path, body })
      const forcedAgent = body.get('split_strategy') === 'tool_agent_regex'
      const preview = {
        title: '蓝天',
        split_strategy: forcedAgent ? 'tool_agent_regex' : 'local_regex',
        split_regex: forcedAgent ? '^AI\\s*(.+)$' : '^LOCAL\\s*(.+)$',
        sample_chars: 20000,
        chapter_count: 2,
        total_chars: 120,
        chapters: [
          { index: 1, title: '序章', chars: 60 },
          { index: 2, title: '第一章 起飞', chars: 60 },
        ],
        warnings: [],
      }
      return new Response(sse([
        ['progress', { step: forcedAgent ? 'agent_start' : 'split_start' }],
        ['preview', preview],
        ['done', { status: 'ok' }],
      ]), { status: 200, headers: { 'Content-Type': 'text/event-stream' } })
    }) as typeof fetch

    const { container } = render(
      <NovelImportDialog
        open
        novaDir="/nova"
        onOpenChange={vi.fn()}
        onImported={vi.fn()}
      />,
    )

    const input = container.querySelector('input[type="file"]') as HTMLInputElement
    fireEvent.change(input, { target: { files: [new File(['序章\n内容\n第一章 起飞\n内容'], '蓝天.txt', { type: 'text/plain' })] } })

    await screen.findByText('本地规则识别')
    expect(screen.getByLabelText('章节/分卷标题正则（Go regexp）')).toHaveValue('^LOCAL\\s*(.+)$')

    await user.click(screen.getByRole('button', { name: 'AI 识别' }))
    await screen.findByText('工具 Agent 识别')
    expect(requests[1].body.get('split_strategy')).toBe('tool_agent_regex')
    expect(requests[1].body.get('split_regex')).toBeNull()
    expect(screen.getByLabelText('章节/分卷标题正则（Go regexp）')).toHaveValue('^AI\\s*(.+)$')
  })
})

function sse(events: Array<[string, unknown]>) {
  return events.map(([event, data]) => `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`).join('')
}
