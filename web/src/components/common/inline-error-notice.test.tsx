import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { InlineErrorNotice } from './inline-error-notice'

it('shows and copies a useful diagnostic without expanding hidden details', async () => {
  const writeText = vi.fn().mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
  render(<InlineErrorNotice message={{ error: 'Approval failed', code: 'agent_runtime.definition_mismatch', request_id: 'request-1', details: { detail: 'definition changed' } }} />)
  expect(screen.getByRole('alert')).toHaveTextContent('definition changed')
  expect(screen.getByRole('alert')).toHaveTextContent('agent_runtime.definition_mismatch')
  fireEvent.click(screen.getByRole('button', { name: '复制诊断信息' }))
  await waitFor(() => expect(writeText).toHaveBeenCalledOnce())
  expect(writeText.mock.calls[0][0]).toContain('request-1')
  expect(writeText.mock.calls[0][0]).toContain('界面版本')
})
