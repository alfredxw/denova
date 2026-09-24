import { render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { TokenUsageDialog } from './TokenUsagePanel'

it('shows reported engine totals without inventing individual model calls', () => {
  render(<TokenUsageDialog open onOpenChange={vi.fn()} messages={[{ id: 'external', role: 'token_usage', run_id: 'external-op', prompt_tokens: 300, completion_tokens: 150, total_tokens: 450 }]} />)
  expect(screen.getByText('引擎仅提供累计用量，未提供逐次调用明细。')).toBeInTheDocument()
  expect(screen.getAllByText('450').length).toBeGreaterThan(0)
  expect(screen.getByText(/1 次 Agent 请求 \/ 不可用 次模型调用/)).toBeInTheDocument()
  expect(screen.queryByText('调用 #1')).not.toBeInTheDocument()
})
