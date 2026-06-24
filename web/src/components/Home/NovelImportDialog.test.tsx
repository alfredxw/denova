import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { NovelImportDialog } from './NovelImportDialog'

describe('NovelImportDialog', () => {
  afterEach(() => {
    vi.restoreAllMocks()
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
