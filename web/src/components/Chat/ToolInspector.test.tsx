import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { MessageItem } from './MessageItem'
import type { ToolCallChatMessage } from '@/lib/api'

vi.mock('@/components/monaco/DenovaMonaco', () => ({
  DenovaMonacoEditor: ({ value, options }: { value: string; options: { ariaLabel: string; readOnly: boolean } }) => <textarea aria-label={options.ariaLabel} value={value} readOnly={options.readOnly} />,
}))

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })

describe('ToolInspector', () => {
  it('offers enlargement only after expansion, copies untouched input, and follows live updates', async () => {
    const copy = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText: copy } })
    const raw = ' {"id":9223372036854775807,"command":"pwd"} '
    const message: ToolCallChatMessage = { id: 'call-1', role: 'tool_call', name: 'bash', args: raw, status: 'running' }
    const { container, rerender } = render(<MessageItem message={message} />)
    const header = container.querySelector('[data-nova-tool-header]')!
    expect(screen.queryByRole('button', { name: '查看工具详情' })).not.toBeInTheDocument()
    fireEvent.click(header)
    const inspectButton = within(header.parentElement!).getByRole('button', { name: '查看工具详情' })
    expect(inspectButton.textContent).toBe('')
    expect(header).not.toContainElement(inspectButton)
    fireEvent.click(inspectButton)
    const dialog = await screen.findByRole('dialog')
    expect(header).toHaveAttribute('aria-expanded', 'true')
    await userEvent.click(within(dialog).getByRole('tab', { name: '原始数据' }))
    expect((await within(dialog).findByRole('textbox', { name: '输入' }) as HTMLTextAreaElement).value).toContain('9223372036854775807')
    expect(within(dialog).getByText('等待工具返回结果…')).toBeVisible()
    fireEvent.click(within(dialog).getAllByRole('button', { name: '复制原文' })[0])
    expect(copy).toHaveBeenCalledWith(raw)

    rerender(<MessageItem message={{ ...message, status: 'success', result: 'finished\nraw output' }} />)
    expect(screen.getByRole('dialog')).toBe(dialog)
    expect(within(dialog).getByRole('textbox', { name: '输出' })).toHaveValue('finished\nraw output')
    fireEvent.click(within(dialog).getByRole('button', { name: '关闭' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(header).toHaveAttribute('aria-expanded', 'true')
    fireEvent.click(header)
    expect(screen.queryByRole('button', { name: '查看工具详情' })).not.toBeInTheDocument()
    vi.unstubAllGlobals()
  })

  it('keeps the inspector open when presentation changes to a specialized Todo card', async () => {
    const message: ToolCallChatMessage = { id: 'todo-1', role: 'tool_call', name: 'todo', args: '{"items":[]}', status: 'running' }
    const { container, rerender } = render(<MessageItem message={message} />)
    fireEvent.click(container.querySelector('[data-nova-tool-header]')!)
    fireEvent.click(screen.getByRole('button', { name: '查看工具详情' }))
    const dialog = await screen.findByRole('dialog')
    rerender(<MessageItem message={{ ...message, args: '{"items":[{"text":"Write chapter"}]}', status: 'success', tool_presentation: { call: 'todo', result: 'todo' } }} />)
    expect(screen.getByRole('dialog')).toBe(dialog)
    expect(within(dialog).getByText('Write chapter')).toBeVisible()
  })

  it('inspects pending questions without duplicating answer controls', async () => {
    render(<MessageItem message={{ role: 'ask', ask: { schema: 'ask.pending.v1', id: 'ask-1', tool_call_id: 'call-ask', agent_kind: 'ide', status: 'pending', questions: [{ id: 'q1', question: 'Which chapter?' }] } }} />)
    fireEvent.click(screen.getByRole('button', { name: '查看工具详情' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('Which chapter?')).toBeVisible()
    expect(within(dialog).queryByRole('textbox')).not.toBeInTheDocument()
    await userEvent.click(within(dialog).getByRole('tab', { name: '原始数据' }))
    expect((within(dialog).getByRole('textbox', { name: '交互记录' }) as HTMLTextAreaElement).value).toContain('call-ask')
    expect(within(dialog).queryByRole('textbox', { name: '输入' })).not.toBeInTheDocument()
  })

})
