import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { InputArea } from './InputArea'

describe('InputArea', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows only skills in skills command mode and keeps keyboard selection visible', () => {
    const scrollSpy = vi.spyOn(HTMLElement.prototype, 'scrollIntoView')
    render(
      <InputArea
        onSend={vi.fn()}
        disabled={false}
        commandScope="skills"
        skills={[
          { name: 'outline', description: 'Outline the next arc' },
          { name: 'rewrite', description: 'Rewrite selected prose' },
          { name: 'worldbuild', description: 'Expand setting details' },
        ]}
      />,
    )

    const input = screen.getByRole('textbox')
    fireEvent.change(input, { target: { value: '/' } })

    expect(screen.getByText('/outline')).toBeInTheDocument()
    expect(screen.getByText('/rewrite')).toBeInTheDocument()
    expect(screen.queryByText('/plan')).not.toBeInTheDocument()

    scrollSpy.mockClear()
    fireEvent.keyDown(input, { key: 'ArrowDown' })

    expect(scrollSpy).toHaveBeenCalledWith({ block: 'nearest' })
  })

  it('opens real token usage details as a request list', async () => {
    const user = userEvent.setup()
    render(
      <InputArea
        onSend={vi.fn()}
        disabled={false}
        tokenUsageMessages={[
          {
            role: 'token_usage',
            id: 'usage-1',
            run_id: 'run-1',
            agent_kind: 'ide',
            prompt_tokens: 1200,
            cached_prompt_tokens: 600,
            cache_hit_rate: 0.5,
            completion_tokens: 240,
            reasoning_tokens: 32,
            total_tokens: 1440,
            model_calls: 1,
            generated_bytes: 88,
            created_at: '2026-06-22T09:00:00Z',
          },
          {
            role: 'token_usage',
            id: 'usage-2',
            run_id: 'run-2',
            agent_kind: 'interactive',
            prompt_tokens: 2000,
            cached_prompt_tokens: 1500,
            cache_hit_rate: 0.75,
            completion_tokens: 500,
            reasoning_tokens: 64,
            total_tokens: 2500,
            model_calls: 2,
            generated_bytes: 188,
            created_at: '2026-06-22T09:01:00Z',
            usage_calls: [
              {
                index: 1,
                created_at: '2026-06-22T09:00:30Z',
                finish_reason: 'tool_calls',
                requested_tools: ['read_workspace_file', 'search_workspace'],
                prompt_tokens: 800,
                cached_prompt_tokens: 500,
                cache_hit_rate: 0.625,
                completion_tokens: 200,
                reasoning_tokens: 24,
                total_tokens: 1000,
              },
              {
                index: 2,
                created_at: '2026-06-22T09:00:45Z',
                finish_reason: 'stop',
                after_tools: ['read_workspace_file'],
                prompt_tokens: 1200,
                cached_prompt_tokens: 1000,
                cache_hit_rate: 0.8333,
                completion_tokens: 300,
                reasoning_tokens: 40,
                total_tokens: 1500,
              },
            ],
          },
        ]}
      />,
    )

    await user.click(screen.getByRole('button', { name: '输入动作' }))
    await user.click(await screen.findByText('真实用量明细'))

    expect(screen.getByRole('heading', { name: '真实模型用量' })).toBeInTheDocument()
    expect(screen.getByText('请求明细')).toBeInTheDocument()
    expect(screen.getByText('数据来源')).toBeInTheDocument()
    expect(screen.getByText('2 次 Agent 请求 / 3 次模型调用')).toBeInTheDocument()
    expect(screen.getByText('请求 1')).toBeInTheDocument()
    expect(screen.getAllByText('run-2').length).toBeGreaterThan(0)
    expect(screen.getAllByText('interactive').length).toBeGreaterThan(0)
    expect(screen.getByText('Call 2')).toBeInTheDocument()
    expect(screen.getAllByText('本次 Token 消耗').length).toBeGreaterThan(0)
    expect(screen.getAllByText('本次请求工具').length).toBeGreaterThan(0)
    expect(screen.getByText('请求工具 read_workspace_file, search_workspace')).toBeInTheDocument()
    expect(screen.getByText('search_workspace')).toBeInTheDocument()
    expect(screen.getByText('工具结果后 read_workspace_file')).toBeInTheDocument()
    expect(screen.getAllByText('83.3%').length).toBeGreaterThan(0)
    expect(screen.getAllByText('3,940').length).toBeGreaterThan(0)
    expect(screen.getAllByText('未命中 Token').length).toBeGreaterThan(0)
    expect(screen.getAllByText('1,100').length).toBeGreaterThan(0)
    expect(screen.getAllByText('300').length).toBeGreaterThan(0)
    expect(screen.getAllByText('1,500').length).toBeGreaterThan(0)
  })
})
