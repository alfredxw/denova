import { render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { TokenUsageDialog } from './TokenUsagePanel'

it('shows reported engine totals without inventing individual model calls', () => {
  render(<TokenUsageDialog open onOpenChange={vi.fn()} messages={[{ id: 'external', role: 'token_usage', run_id: 'external-op', prompt_tokens: 300, completion_tokens: 150, total_tokens: 450 }]} />)
  expect(screen.getByText('引擎仅提供累计用量，未提供逐次调用明细。')).toBeInTheDocument()
  expect(screen.getAllByText('450').length).toBeGreaterThan(0)
  expect(screen.getByText(/1 次 Agent 请求 \/ 不可用 次模型调用/)).toBeInTheDocument()
  expect(screen.queryByText('调用 #1')).not.toBeInTheDocument()
  expect(screen.queryByText('缓存写入')).not.toBeInTheDocument()
})

it('shows cache write tokens in summary and call breakdown when reported', () => {
  render(
    <TokenUsageDialog
      open
      onOpenChange={vi.fn()}
      messages={[{
        id: 'anthropic-run',
        role: 'token_usage',
        run_id: 'run-anthropic',
        prompt_tokens: 10100,
        cached_prompt_tokens: 8000,
        cache_write_prompt_tokens: 2000,
        uncached_prompt_tokens: 2100,
        cache_hit_rate: 0.792,
        completion_tokens: 50,
        total_tokens: 10150,
        model_calls: 1,
        usage_calls: [{
          index: 1,
          prompt_tokens: 10100,
          cached_prompt_tokens: 8000,
          cache_write_prompt_tokens: 2000,
          uncached_prompt_tokens: 2100,
          cache_hit_rate: 0.792,
          completion_tokens: 50,
          total_tokens: 10150,
        }],
      }]}
    />,
  )
  expect(screen.getByText('缓存写入')).toBeInTheDocument()
  expect(screen.getAllByText('2,000').length).toBeGreaterThanOrEqual(2)
})
