import { useState } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, it, vi } from 'vitest'

import { fetchModelCatalog, pingModelProfile } from './api'
import { ModelProfilesEditor } from './ModelProfilesEditor'
import {
  MODEL_PROTOCOL_ANTHROPIC_MESSAGES,
  MODEL_PROTOCOL_CHAT_COMPLETIONS,
  MODEL_PROTOCOL_GOOGLE_GENERATIVE_AI,
  MODEL_PROTOCOL_RESPONSES,
  MODEL_PROVIDER_DEEPSEEK,
  MODEL_PROVIDER_OPENAI,
} from './model-profiles'
import type { ModelCatalog, ModelProfileSettings } from './types'

vi.mock('./api', () => ({
  fetchModelCatalog: vi.fn(),
  pingModelProfile: vi.fn(),
}))

const catalog: ModelCatalog = {
  protocols: [
    MODEL_PROTOCOL_ANTHROPIC_MESSAGES,
    MODEL_PROTOCOL_GOOGLE_GENERATIVE_AI,
    MODEL_PROTOCOL_CHAT_COMPLETIONS,
    MODEL_PROTOCOL_RESPONSES,
  ],
  providers: [
    {
      id: MODEL_PROVIDER_OPENAI,
      name: 'OpenAI',
      default_protocol: MODEL_PROTOCOL_RESPONSES,
      endpoints: {
        [MODEL_PROTOCOL_CHAT_COMPLETIONS]: { base_url: 'https://api.openai.com/v1' },
        [MODEL_PROTOCOL_RESPONSES]: { base_url: 'https://api.openai.com/v1' },
      },
    },
    {
      id: MODEL_PROVIDER_DEEPSEEK,
      name: 'DeepSeek',
      default_protocol: MODEL_PROTOCOL_CHAT_COMPLETIONS,
      endpoints: {
        [MODEL_PROTOCOL_CHAT_COMPLETIONS]: { base_url: 'https://api.deepseek.com' },
        [MODEL_PROTOCOL_RESPONSES]: { base_url: 'https://api.deepseek.com' },
        [MODEL_PROTOCOL_ANTHROPIC_MESSAGES]: { base_url: 'https://api.deepseek.com/anthropic' },
      },
    },
  ],
}

beforeEach(() => {
  vi.mocked(fetchModelCatalog).mockReset().mockResolvedValue(catalog)
  vi.mocked(pingModelProfile).mockReset()
})

it('keeps provider and protocol independent while applying catalog endpoint defaults', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  render(<EditorHarness onChange={onChange} />)

  await waitFor(() => expect(document.querySelector('option[value="deepseek"]')).toBeInTheDocument())
  expect(screen.getByDisplayValue(MODEL_PROVIDER_OPENAI)).toBeInTheDocument()
  expect(screen.getByText('Responses API')).toBeInTheDocument()

  const providerInput = screen.getByDisplayValue(MODEL_PROVIDER_OPENAI)
  await user.clear(providerInput)
  await user.type(providerInput, MODEL_PROVIDER_DEEPSEEK)

  expect(screen.getByText('Responses API')).toBeInTheDocument()
  expect(screen.getByDisplayValue('https://api.deepseek.com')).toBeInTheDocument()
  expect(onChange).toHaveBeenLastCalledWith([
    expect.objectContaining({
      provider: MODEL_PROVIDER_DEEPSEEK,
      protocol: MODEL_PROTOCOL_RESPONSES,
      base_url: 'https://api.deepseek.com',
    }),
  ])

  await user.click(screen.getByText('Responses API').closest('button')!)
  await user.click(screen.getByRole('option', { name: 'Anthropic Messages' }))

  expect(screen.getByDisplayValue('https://api.deepseek.com/anthropic')).toBeInTheDocument()
  expect(onChange).toHaveBeenLastCalledWith([
    expect.objectContaining({
      provider: MODEL_PROVIDER_DEEPSEEK,
      protocol: MODEL_PROTOCOL_ANTHROPIC_MESSAGES,
      base_url: 'https://api.deepseek.com/anthropic',
    }),
  ])
})

it('pings the current unsaved profile and reports the resolved route', async () => {
  const user = userEvent.setup()
  vi.mocked(pingModelProfile).mockResolvedValue({
    ok: true,
    latency_ms: 42,
    provider: MODEL_PROVIDER_OPENAI,
    protocol: MODEL_PROTOCOL_RESPONSES,
    base_url: 'https://api.openai.com/v1',
    model: 'gpt-5',
  })
  render(<EditorHarness />)

  await user.click(screen.getByRole('button', { name: '测试连接' }))

  await waitFor(() => expect(pingModelProfile).toHaveBeenCalledWith(expect.objectContaining({
    id: 'default',
    provider: MODEL_PROVIDER_OPENAI,
    protocol: MODEL_PROTOCOL_RESPONSES,
    api_key: 'test-key',
    model: 'gpt-5',
  }), expect.any(AbortSignal)))
  expect(await screen.findByText(/连接成功，耗时 42 ms/)).toBeInTheDocument()
})

function EditorHarness({ onChange = () => undefined }: { onChange?: (profiles: ModelProfileSettings[]) => void }) {
  const [profiles, setProfiles] = useState<ModelProfileSettings[]>([{
    id: 'default',
    provider: MODEL_PROVIDER_OPENAI,
    protocol: MODEL_PROTOCOL_RESPONSES,
    api_key: 'test-key',
    base_url: 'https://api.openai.com/v1',
    model: 'gpt-5',
  }])
  return (
    <ModelProfilesEditor
      profiles={profiles}
      effectiveProfiles={[]}
      onChange={(nextProfiles) => {
        setProfiles(nextProfiles)
        onChange(nextProfiles)
      }}
    />
  )
}
