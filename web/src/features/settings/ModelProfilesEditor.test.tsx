import { useState } from 'react'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, it, vi } from 'vitest'

import { discoverModels, fetchModelCatalog, pingModelProfile } from './api'
import { ModelProfilesEditor } from './ModelProfilesEditor'
import {
  MODEL_PROTOCOL_ANTHROPIC_MESSAGES,
  MODEL_PROTOCOL_CHAT_COMPLETIONS,
  MODEL_PROTOCOL_RESPONSES,
  MODEL_PROVIDER_DEEPSEEK,
  MODEL_PROVIDER_OPENAI,
  MODEL_PROVIDER_OPENAI_COMPATIBLE,
} from './model-profiles'
import type { ModelCatalog, ModelProfileSettings } from './types'

vi.mock('./api', () => ({
  discoverModels: vi.fn(),
  fetchModelCatalog: vi.fn(),
  pingModelProfile: vi.fn(),
}))

const catalog: ModelCatalog = {
  protocols: [
    MODEL_PROTOCOL_ANTHROPIC_MESSAGES,
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
    {
      id: MODEL_PROVIDER_OPENAI_COMPATIBLE,
      name: 'Compatible / Custom Endpoint',
      default_protocol: MODEL_PROTOCOL_CHAT_COMPLETIONS,
      endpoints: {
        [MODEL_PROTOCOL_CHAT_COMPLETIONS]: {},
        [MODEL_PROTOCOL_RESPONSES]: {},
      },
    },
    ...Array.from({ length: 10 }, (_, index) => ({
      id: `provider-${index}`,
      name: `Provider ${index}`,
      default_protocol: MODEL_PROTOCOL_CHAT_COMPLETIONS,
      endpoints: {
        [MODEL_PROTOCOL_CHAT_COMPLETIONS]: { base_url: `https://provider-${index}.example.test/v1` },
      },
    })),
  ],
}

beforeEach(() => {
  vi.mocked(discoverModels).mockReset().mockResolvedValue({
    models: [
      { id: 'gpt-listed', owned_by: 'openai' },
      { id: 'gpt-other' },
    ],
    provider: MODEL_PROVIDER_OPENAI,
    protocol: MODEL_PROTOCOL_RESPONSES,
    base_url: 'https://api.openai.com/v1',
  })
  vi.mocked(fetchModelCatalog).mockReset().mockResolvedValue(catalog)
  vi.mocked(pingModelProfile).mockReset()
})

it('offers discovered models without restricting custom model input', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  render(<EditorHarness onChange={onChange} />)

  await user.click(await screen.findByRole('button', { name: '获取可用模型' }))
  await waitFor(() => expect(discoverModels).toHaveBeenCalledWith(expect.objectContaining({
    provider: MODEL_PROVIDER_OPENAI,
    protocol: MODEL_PROTOCOL_RESPONSES,
    model: 'gpt-5',
  }), expect.any(AbortSignal)))
  await user.click(await screen.findByRole('option', { name: /gpt-listed/ }))
  expect(screen.getByDisplayValue('gpt-listed')).toBeInTheDocument()

  const modelInput = screen.getByDisplayValue('gpt-listed')
  await user.clear(modelInput)
  await user.type(modelInput, 'private-model-id')
  expect(screen.getByDisplayValue('private-model-id')).toBeInTheDocument()
  expect(onChange).toHaveBeenLastCalledWith([
    expect.objectContaining({ model: 'private-model-id' }),
  ])
})

it('keeps provider and protocol independent while applying catalog endpoint defaults', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  render(<EditorHarness onChange={onChange} />)

  const providerPicker = await screen.findByRole('combobox', { name: '服务商' })
  expect(providerPicker).toHaveTextContent('OpenAI')
  expect(screen.getByText('Responses API')).toBeInTheDocument()

  await user.click(providerPicker)
  const providerList = screen.getByRole('listbox', { name: '服务商' })
  expect(providerList).toHaveClass('max-h-64', 'overflow-y-auto')
  expect(within(providerList).getAllByRole('option')).toHaveLength(catalog.providers.length)
  fireEvent.scroll(providerList, { target: { scrollTop: 96 } })
  expect(within(providerList).getAllByRole('option')).toHaveLength(catalog.providers.length)
  await user.click(screen.getByRole('option', { name: /DeepSeek/ }))

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

  await user.click(screen.getByRole('combobox', { name: '服务商' }))
  await user.click(screen.getByRole('option', { name: /^OpenAI/ }))
  expect(screen.getByRole('combobox', { name: '服务商' })).toHaveTextContent('OpenAI')
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

it('configures a custom SessionKey mapping for compatible endpoints', async () => {
  const user = userEvent.setup()
  const onChange = vi.fn()
  render(<EditorHarness onChange={onChange} />)

  await user.click(await screen.findByRole('combobox', { name: '服务商' }))
  await user.click(screen.getByRole('option', { name: /Compatible/ }))

  const mappingPicker = screen.getByRole('combobox', { name: 'SessionKey 映射' })
  await user.click(mappingPicker)
  await user.click(screen.getByRole('option', { name: '请求 Header' }))
  expect(screen.getByDisplayValue('X-Session-Id')).toBeInTheDocument()
  expect(onChange).toHaveBeenLastCalledWith([
    expect.objectContaining({
      provider: MODEL_PROVIDER_OPENAI_COMPATIBLE,
      session_key_mapping: { location: 'header', name: 'X-Session-Id' },
    }),
  ])

  const fieldName = screen.getByDisplayValue('X-Session-Id')
  await user.clear(fieldName)
  await user.type(fieldName, 'X-Custom-Session')
  expect(onChange).toHaveBeenLastCalledWith([
    expect.objectContaining({
      session_key_mapping: { location: 'header', name: 'X-Custom-Session' },
    }),
  ])
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
