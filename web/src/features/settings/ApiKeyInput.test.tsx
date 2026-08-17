import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it } from 'vitest'

import { ApiKeyInput } from './ApiKeyInput'

it('keeps an API key masked until the user explicitly reveals it', async () => {
  const user = userEvent.setup()
  render(<ApiKeyInputHarness />)

  const input = screen.getByLabelText('API Key')
  expect(input).toHaveAttribute('type', 'password')
  expect(input).toHaveValue('sk-secret')

  await user.click(screen.getByRole('button', { name: '显示 API Key' }))
  expect(input).toHaveAttribute('type', 'text')
  expect(screen.getByRole('button', { name: '隐藏 API Key' })).toHaveAttribute('aria-pressed', 'true')

  await user.clear(input)
  await user.type(input, 'sk-updated')
  expect(input).toHaveValue('sk-updated')

  await user.click(screen.getByRole('button', { name: '隐藏 API Key' }))
  expect(input).toHaveAttribute('type', 'password')
})

function ApiKeyInputHarness() {
  const [value, setValue] = useState('sk-secret')
  return (
    <ApiKeyInput
      label="API Key"
      value={value}
      placeholder="API Key，不填则继承默认"
      onChange={setValue}
    />
  )
}
