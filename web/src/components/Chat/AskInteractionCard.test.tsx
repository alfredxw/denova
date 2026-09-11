import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { setConfiguredLocale } from '@/i18n'
import { AskInteractionCard } from './AskInteractionCard'

afterEach(() => act(() => setConfiguredLocale('zh-CN')))

describe('tool effect verification', () => {
  it.each([
    { locale: 'zh-CN' as const, question: '这项操作是否已经生效？', unknown: '暂时无法确定', submit: '提交', waiting: '尚未确认操作结果，任务将继续等待。' },
    { locale: 'en-US' as const, question: 'Did this operation take effect?', unknown: 'I cannot determine this yet', submit: 'Submit', waiting: 'Verification is still pending. The task will keep waiting.' },
  ])('localizes $locale and keeps an uncertain answer pending', async copy => {
    setConfiguredLocale(copy.locale)
    const resolve = vi.fn().mockResolvedValue({ schema: 'ask.result.v1', id: 'verify-call', status: 'pending' })
    render(<AskInteractionCard message={{ id: 'ask', role: 'ask', ask: {
      schema: 'ask.pending.v1', id: 'verify-call', tool_call_id: 'execution-1', agent_kind: 'ide', status: 'pending', allow_other: false,
      verification: { execution_id: 'execution-1', tool: 'publish_document', arguments: { title: 'Chapter 1' } },
      questions: [{ id: 'effect', question: 'Host diagnostic question', options: [
        { id: 'executed', label: 'Executed' }, { id: 'not_executed', label: 'Not executed' }, { id: 'unknown', label: 'Unknown' },
      ] }],
    } }} onResolve={resolve} />)
    expect(screen.getByText(copy.question)).toBeInTheDocument()
    expect(screen.queryByText('Host diagnostic question')).not.toBeInTheDocument()
    expect(screen.getByText('publish_document')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('radio', { name: copy.unknown }))
    fireEvent.click(screen.getByRole('button', { name: copy.submit }))
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent(copy.waiting))
    expect(resolve).toHaveBeenCalledWith(expect.anything(), { status: 'answered', answers: [{ question_id: 'effect', selected_option_ids: ['unknown'] }] })
    expect(screen.getByRole('radio', { name: copy.unknown })).toBeEnabled()
    resolve.mockResolvedValue({ schema: 'ask.result.v1', id: 'verify-call', status: 'cancelled', cancel_reason: 'task_aborted' })
    fireEvent.click(screen.getByRole('button', { name: copy.locale === 'zh-CN' ? '取消任务' : 'Cancel task' }))
    await waitFor(() => expect(resolve).toHaveBeenLastCalledWith(expect.anything(), { status: 'cancelled', reason: 'task_aborted' }))
  })
})
