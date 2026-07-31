import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'

import { ModelProfilesEditor } from './ModelProfilesEditor'
import {
  MODEL_PROTOCOL_CHAT_COMPLETIONS,
  MODEL_PROTOCOL_RESPONSES,
  MODEL_PROVIDER_DEEPSEEK,
  MODEL_PROVIDER_OPENAI,
} from './model-profiles'

it('shows provider and protocol independently and applies the provider defaults together', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  render(
    <ModelProfilesEditor
      profiles={[{
        id: 'default',
        provider: MODEL_PROVIDER_OPENAI,
        protocol: MODEL_PROTOCOL_RESPONSES,
        openai_base_url: 'https://api.openai.com/v1',
        openai_model: 'gpt-5',
      }]}
      effectiveProfiles={[]}
      onChange={onChange}
    />,
  )

  expect(screen.getByText('OpenAI')).toBeInTheDocument()
  expect(screen.getByText('Responses API')).toBeInTheDocument()

  await user.click(screen.getByText('OpenAI').closest('button')!)
  await user.click(screen.getByRole('option', { name: 'DeepSeek' }))

  expect(onChange).toHaveBeenCalledWith([
    expect.objectContaining({
      provider: MODEL_PROVIDER_DEEPSEEK,
      protocol: MODEL_PROTOCOL_CHAT_COMPLETIONS,
      openai_base_url: 'https://api.deepseek.com',
    }),
  ])
})
